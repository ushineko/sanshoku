package sony

import (
	"errors"
	"fmt"

	"github.com/ushineko/sanshoku/battery"
)

/*
The DualSense's input report, as the Linux kernel's hid-playstation reads it
(dualsense_parse_report, docs/credits.md) and as measured on a DualSense over
Bluetooth on Windows (spec 017).

Over USB the controller streams report 0x01, 64 bytes, from the moment it is
plugged in. Over Bluetooth it starts with a short report 0x01 that carries the
sticks and buttons and nothing else, and switches to report 0x31, 78 bytes,
once the host reads feature report 0x05 (the calibration): the request
hid-playstation and Steam both make. The two long reports carry the same body,
0x31 one byte further in for a sequence tag, and one byte of it is the
battery: the low nibble a level in tenths, the high nibble the charge state.

	USB  01 ... [53]=0x07   level 7, discharging: 75 %
	BT   31 ... [54]=0x07
*/
const (
	usbReport = 0x01
	btReport  = 0x31

	// usbStatusAt and btStatusAt are where the status byte is, counting the
	// report ID: the body's byte 52, after one byte of report ID over USB and
	// two (the ID and the tag) over Bluetooth.
	usbStatusAt = 53
	btStatusAt  = 54

	// calibration is the feature report whose reading switches a
	// Bluetooth-connected controller to report 0x31, and calibrationLen its
	// length over Bluetooth, as hid-playstation asks for it.
	calibration    = 0x05
	calibrationLen = 41
)

// The charge states in the status byte's high nibble, hid-playstation's
// names. 0xA, 0xB and 0xF are the controller reporting a fault.
const (
	stateDischarging = 0x0
	stateCharging    = 0x1
	stateFull        = 0x2
)

// errNoReading is a report that is a report and not a reading.
var errNoReading = errors.New("no battery reading")

/*
Decode reads the battery out of one input report.

overBluetooth says how the controller is connected, because report 0x01 means
two different things: over USB it is the long report and carries the battery,
over Bluetooth it is the short one and does not, and on Windows both arrive
padded to the same length. A report that is not the long report for the
connection is errNoReading.

The level is the kernel's: tenths plus five, so a level of 7 is 75 % and the
top step 100 %; full is 100 %. A fault state is errNoReading with the state in
the message: the controller is there and has said something this driver cannot
stand behind.
*/
func Decode(r []byte, overBluetooth bool) (battery.Battery, error) {
	at, id := usbStatusAt, byte(usbReport)
	if overBluetooth {
		at, id = btStatusAt, btReport
	}
	if len(r) <= at || r[0] != id {
		return battery.Battery{}, errNoReading
	}
	status := r[at] //nolint:gosec // len(r) > at, checked above
	b := battery.Battery{Kind: battery.KindGamepad, HasLevel: true, Level: min(int(status&0x0F)*10+5, 100)}
	switch status >> 4 {
	case stateDischarging:
		b.State = battery.Discharging
	case stateCharging:
		b.State = battery.Charging
	case stateFull:
		b.State, b.Level = battery.Full, 100
	default:
		return battery.Battery{}, fmt.Errorf("%w: charge state %#x", errNoReading, status>>4)
	}
	return b, nil
}

// isLong reports whether r is a report Decode reads: the long report for the
// connection.
func isLong(r []byte, overBluetooth bool) bool {
	if overBluetooth {
		return len(r) > btStatusAt && r[0] == btReport
	}
	return len(r) > usbStatusAt && r[0] == usbReport
}
