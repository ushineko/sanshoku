package sony

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku/battery"
)

// longReport is a long report for the connection with status as its battery
// byte, the rest zero.
func longReport(overBluetooth bool, status byte) []byte {
	if overBluetooth {
		r := make([]byte, 78)
		r[0], r[btStatusAt] = btReport, status
		return r
	}
	r := make([]byte, 64)
	r[0], r[usbStatusAt] = usbReport, status
	return r
}

// The status byte, read as hid-playstation reads it. 0x07 discharging is the
// reading measured on the desk in spec 017.
func TestDecodeReadsTheStatusByte(t *testing.T) {
	for _, c := range []struct {
		name   string
		status byte
		level  int
		state  battery.State
	}{
		{"measured: level 7, discharging", 0x07, 75, battery.Discharging},
		{"empty", 0x00, 5, battery.Discharging},
		{"the top step is 100, not 105", 0x0a, 100, battery.Discharging},
		{"charging", 0x14, 45, battery.Charging},
		{"full is 100 whatever the level", 0x28, 100, battery.Full},
	} {
		for _, bt := range []bool{false, true} {
			b, err := Decode(longReport(bt, c.status), bt)
			require.NoError(t, err, c.name)
			assert.Equal(t, c.level, b.Level, c.name)
			assert.True(t, b.HasLevel, c.name)
			assert.Equal(t, c.state, b.State, c.name)
			assert.Equal(t, battery.KindGamepad, b.Kind, c.name)
		}
	}
}

// A fault in the charge state is no reading, not a level.
func TestAFaultIsNoReading(t *testing.T) {
	for _, status := range []byte{0xa5, 0xb5, 0xf5} {
		_, err := Decode(longReport(false, status), false)
		require.ErrorIs(t, err, errNoReading, "%#x", status)
	}
}

/*
Report 0x01 is the long report over USB and the short one over Bluetooth, and
on Windows both arrive padded to the same length, so the connection decides.
The short report measured over Bluetooth had sensor bytes where the status
byte would be; read as a long report it would have been a level.
*/
func TestTheConnectionDecidesWhichReportIsLong(t *testing.T) {
	short := longReport(false, 0x07)
	short = append(short, make([]byte, 78-len(short))...)
	_, err := Decode(short, true)
	require.ErrorIs(t, err, errNoReading, "a Bluetooth 0x01 was read as a reading")

	_, err = Decode(longReport(true, 0x07), false)
	require.ErrorIs(t, err, errNoReading, "a 0x31 was read as a USB report")

	_, err = Decode([]byte{usbReport, 0x80, 0x80}, false)
	require.ErrorIs(t, err, errNoReading, "a truncated report was read")
}
