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

// novaProProbe is the head of the base station's reply to `06 b0` from the
// probe on 2026-09-29 (1038:12e5): level 6 of 8, status 0x08 online, and
// headsetcontrol reporting 75% at the same moment. The first sixteen bytes
// are all the decoder reads.
var novaProProbe = []byte{
	0x06, 0xb0, 0x00, 0x00, 0x01, 0x00, 0x06, 0x08,
	0x0a, 0x00, 0x00, 0x0a, 0x04, 0x00, 0x08, 0x08,
}

// novaProWith is the probe's reply with the level and status bytes replaced.
func novaProWith(level, status byte) []byte {
	r := append([]byte(nil), novaProProbe...)
	r[6], r[15] = level, status
	return r
}

/*
The Nova Pro's reply, from the probe and from the statuses HeadsetControl
names.

The probe's 6 of 8 is 75%, which is what headsetcontrol said beside it, so the
arithmetic is asserted against a measurement rather than restated. A headset
that is off is a reading with no level and no error, however stale the level
byte; charging at 8 is Full. A reply too short to hold the status, and a level
off the 0-8 scale, are errors and not readings.
*/
func TestDecodeNovaProReadsLevelInEighthsAndTheHeadsetsStatus(t *testing.T) {
	for _, c := range []struct {
		name     string
		reply    []byte
		hasLevel bool
		level    int
		state    battery.State
	}{
		{"the probe, online", novaProProbe, true, 75, battery.Discharging},
		{"the headset off", novaProWith(0x06, 0x01), false, 0, battery.Discharging},
		{"charging on the cable, full", novaProWith(0x08, 0x02), true, 100, battery.Full},
		{"charging on the cable, part way", novaProWith(0x03, 0x02), true, 37, battery.Charging},
	} {
		b, err := DecodeNovaPro(c.reply)
		require.NoError(t, err, c.name)
		assert.Equal(t, c.hasLevel, b.HasLevel, c.name)
		assert.Equal(t, c.level, b.Level, c.name)
		assert.Equal(t, c.state, b.State, c.name)
		assert.Equal(t, battery.KindHeadset, b.Kind, c.name)
	}

	_, err := DecodeNovaPro(novaProProbe[:15])
	require.Error(t, err, "a reply without byte 15 was decoded")
	_, err = DecodeNovaPro(novaProWith(9, 0x08))
	require.Error(t, err, "a level of 9 of 8 was decoded")
}
