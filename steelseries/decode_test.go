package steelseries

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku/battery"
)

/*
The decoder, from the values in hayami's steelseries_test.go.

0x95 is what the Apex answered with its cable in: bit 7 charging, and
((0x95 & 0x7f) - 1) * 5 = 100, so full. The arithmetic is the device's own
resolution and is asserted here so a future simplification to "the byte is a
percentage" cannot pass. A value that cannot be a level is not turned into
one, and a reply too short to hold a value is not a zero.
*/
func TestDecodeModernReadsTheValueByteInStepsOfFive(t *testing.T) {
	for _, c := range []struct {
		value byte
		level int
		state battery.State
	}{
		{0x95, 100, battery.Full},    // charging bit set, and already full
		{0x8d, 60, battery.Charging}, // charging bit set, part way up
		{0x14, 95, battery.Discharging},
		{0x15, 100, battery.Discharging},
		{0x02, 5, battery.Discharging},
	} {
		b, err := DecodeModern([]byte{batteryCommand, c.value})
		require.NoError(t, err, "value %#02x", c.value)
		assert.True(t, b.HasLevel)
		assert.Equal(t, c.level, b.Level, "value %#02x", c.value)
		assert.Equal(t, c.state, b.State, "value %#02x", c.value)
	}

	for _, value := range []byte{0x00, 0x80, 0x7f} {
		_, err := DecodeModern([]byte{batteryCommand, value})
		require.Error(t, err, "value %#02x was decoded as a level", value)
	}
	_, err := DecodeModern([]byte{batteryCommand})
	require.Error(t, err, "a one-byte reply was decoded")
}
