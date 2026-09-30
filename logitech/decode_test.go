package logitech

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku/battery"
)

/*
The decoders, from the bytes in hayami's tests (hidpp_test.go, band_test.go,
logitech_test.go).

0x56 is 86, which is what solaar reported for the G502 X PLUS at the same
moment; 0x08 is the "full" level band and 0x00 is discharging. Charging and
charged are told apart, because a device on its cable that has finished is not
a device still filling. A reply too short to hold a reading is an error, not a
zero.
*/
func TestTheBatteryFeaturesDecodeToWhatTheDeviceReported(t *testing.T) {
	cases := []struct {
		name   string
		decode func([]byte) (battery.Battery, error)
		p      []byte
		level  int
		state  battery.State
	}{
		{"0x1004 the G502's own reply", DecodeUnifiedBattery, []byte{0x56, 0x08, 0x00, 0x00}, 86, battery.Discharging},
		{"0x1004 charging", DecodeUnifiedBattery, []byte{0x32, 0x04, 0x01, 0x01}, 50, battery.Charging},
		{"0x1004 charging slowly", DecodeUnifiedBattery, []byte{0x32, 0x04, 0x02, 0x01}, 50, battery.Charging},
		{"0x1004 charged", DecodeUnifiedBattery, []byte{0x64, 0x08, 0x03, 0x01}, 100, battery.Full},
		{"0x1000 recharging", DecodeBatteryStatus, []byte{0x32, 0x00, 0x01}, 50, battery.Charging},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := c.decode(c.p)
			require.NoError(t, err)
			assert.True(t, b.HasLevel)
			assert.Equal(t, c.level, b.Level)
			assert.Equal(t, c.state, b.State)
			assert.False(t, b.HasBand)
		})
	}

	_, err := DecodeUnifiedBattery([]byte{0x56})
	require.Error(t, err, "a one-byte 0x1004 reply was read as a level")
	_, err = DecodeBatteryStatus([]byte{0x56, 0x00})
	require.Error(t, err, "a two-byte 0x1000 reply was read as a level")
}

/*
The HID++ 1.0 registers, from band_test.go.

The four bands decode and nothing else does: the mapping is solaar's, checked
against a K800 that answered `05 00 00`, good and discharging. A value outside
them is no reading rather than the nearest band. Register 0x0D is a percentage
and is never also a band.
*/
func TestTheRegistersDecodeToAPercentageOrOneOfFourBands(t *testing.T) {
	for value, want := range map[byte]battery.Band{
		0x01: battery.BandCritical, 0x03: battery.BandLow, 0x05: battery.BandGood, 0x07: battery.BandFull,
	} {
		b, err := decodeStatusRegister([]byte{value, 0x00})
		require.NoError(t, err, "value %#02x", value)
		assert.Equal(t, want, b.Band)
		assert.True(t, b.HasBand)
		assert.False(t, b.HasLevel, "a band is never also a percentage")
		assert.Zero(t, b.Level)
	}
	for _, value := range []byte{0x00, 0x02, 0x04, 0x06, 0x08, 0xFF} {
		_, err := decodeStatusRegister([]byte{value, 0x00})
		require.Error(t, err, "value %#02x was decoded as a band", value)
	}

	k800, err := decodeStatusRegister([]byte{0x05, 0x00, 0x00})
	require.NoError(t, err)
	assert.Equal(t, battery.BandGood, k800.Band)
	assert.Equal(t, 3, k800.Band.Segments())
	assert.Equal(t, battery.Discharging, k800.State)

	cabled, err := decodeStatusRegister([]byte{0x05, 0x21})
	require.NoError(t, err)
	assert.Equal(t, battery.Charging, cabled.State, "a charge byte that is not zero is a device on the cable")

	gauge, err := decodeChargeRegister([]byte{72, 0x00})
	require.NoError(t, err)
	assert.Equal(t, 72, gauge.Level)
	assert.True(t, gauge.HasLevel)
	assert.False(t, gauge.HasBand, "a percentage is not also a band")
	assert.Equal(t, battery.Discharging, gauge.State)
}

/*
The two HID++ error spaces are read apart (hayami issue #66).

0x01 is the collision: unsupported feature in 2.0, invalid sub-id in 1.0. One
table read them both, so a ten-year-old keyboard that does not speak 2.0 at all
was taken for a device with no fuel gauge.
*/
func TestTheTwoErrorSpacesAreReadApart(t *testing.T) {
	cases := []struct {
		name string
		got  error
		want error
	}{
		{"2.0 unsupported feature", translate20(0x01), errUnknownFeature},
		{"1.0 invalid sub-id", translate10(0x01), errOldProtocol},
		{"1.0 invalid address", translate10(0x02), errUnknownFeature},
		{"1.0 connect fail", translate10(0x04), errNotReachable},
		{"1.0 busy", translate10(0x07), errNotReachable},
		{"1.0 unknown device", translate10(0x08), errNoDevice},
		{"1.0 resource error", translate10(0x09), errNotReachable},
	}
	for _, c := range cases {
		assert.ErrorIs(t, c.got, c.want, c.name)
	}
	var h hidppError
	assert.True(t, errors.As(translate10(0x7F), &h), "a code neither space names is a failure")
	assert.True(t, errors.As(translate20(0x7F), &h), "a code neither space names is a failure")
}

/*
A stray byte followed by the report's own padding is the exact shape the
phantom "Q" arrived in (phantom_test.go): long enough to fill the declared
length, and full of holes. A real name is whole.
*/
func TestAStrayByteAndPaddingIsNotAName(t *testing.T) {
	name, whole := printableName([]byte{0x51, 0x00, 0x00, 0x51, 0x00, 0x00})
	assert.Equal(t, "QQ", name, "the printable bytes are still recovered")
	assert.False(t, whole, "a run with holes in it was accepted as a name")

	name, whole = printableName([]byte("G502 X PLUS"))
	assert.Equal(t, "G502 X PLUS", name)
	assert.True(t, whole)
}

/*
The software ID is in the nibble, is never zero, follows the process, and is
never solaar's 0x0B (phantom_test.go). Picking freely from 1..15 landed on
solaar's one run in fifteen, and hayami read 71 % in the same second solaar
read 79 %.
*/
func TestTheSoftwareIDIsNeverZeroNorSolaars(t *testing.T) {
	assert.Equal(t, softwareIDs[os.Getpid()%len(softwareIDs)], softwareID)
	assert.NotContains(t, softwareIDs, byte(0x0B))
	assert.NotContains(t, softwareIDs, byte(0x00))
	assert.Len(t, softwareIDs, 14, "every other value a nibble can hold")
}
