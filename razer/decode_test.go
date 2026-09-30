package razer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reply builds a well-formed answer to one getter with a status and a value,
// its checksum computed the way the device computes it. The transaction is
// the dock's relay, as hayami's razer_test.go builds it.
func reply(status, command, value byte) []byte {
	out := make([]byte, reportSize+1)
	body := out[1:]
	body[0] = status
	body[1] = 0x1F
	body[5] = argsSize
	body[6] = classPower
	body[7] = command
	body[argsAt+1] = value
	body[crcAt] = crc(body)
	return out
}

/*
The decoder, from the bytes in hayami's razer_test.go.

The level is a byte over full scale, rounded rather than truncated, so 0xFF is
100 and not 99. The charge state is the value being non-zero. Busy, timeout and
not-supported are silence; fail is a fault. A reply whose checksum does not
match is not a reading: this is a radio link with a dock in the middle, and a
corrupted level is a number a panel would draw without hesitating.
*/
func TestDecodeReadsTheLevelAndChargeStateAndRejectsABadChecksum(t *testing.T) {
	for _, c := range []struct {
		raw   byte
		level int
	}{{0xFF, 100}, {0x80, 50}, {0x40, 25}, {0x00, 0}} {
		level, charging, err := Decode(reply(statusOK, commandBatteryLevel, c.raw))
		require.NoError(t, err, "raw %#02x", c.raw)
		assert.Equal(t, c.level, level, "raw %#02x", c.raw)
		assert.False(t, charging)
	}

	_, charging, err := Decode(reply(statusOK, commandCharging, 0x01))
	require.NoError(t, err)
	assert.True(t, charging)
	_, charging, err = Decode(reply(statusOK, commandCharging, 0x00))
	require.NoError(t, err)
	assert.False(t, charging)

	for _, status := range []byte{statusBusy, statusTimeout, statusNotSupported} {
		_, _, err := Decode(reply(status, commandBatteryLevel, 0x00))
		require.ErrorIs(t, err, errSilent, "status %#02x", status)
	}
	_, _, err = Decode(reply(statusFail, commandBatteryLevel, 0x00))
	require.Error(t, err)
	assert.NotErrorIs(t, err, errSilent, "a refusal was taken for silence")

	corrupt := reply(statusOK, commandBatteryLevel, 0xFF)
	corrupt[1+crcAt] ^= 0xFF // the checksum, past the report number
	_, _, err = Decode(corrupt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum")

	_, _, err = Decode(corrupt[:40])
	require.Error(t, err, "a short reply was decoded")
}

// The request carries the checksum the device expects, computed over the
// addressed bytes and not over the whole report, and the report number is not
// part of the report.
func TestTheRequestCarriesItsChecksum(t *testing.T) {
	req := request(0x1F, classPower, commandBatteryLevel)
	body := req[1:]

	var want byte
	for _, b := range body[2:crcAt] {
		want ^= b
	}

	assert.Equal(t, want, body[crcAt])
	assert.Equal(t, byte(classPower), body[6])
	assert.Equal(t, byte(commandBatteryLevel), body[7])
	assert.Len(t, req, reportSize+1)
}
