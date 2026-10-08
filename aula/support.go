package aula

import (
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/support"
)

/*
Support is the aula driver's support table: the one receiver on the
allow-list.

The F75 on its 2.4 GHz receiver is Tested on Windows, where the bench read it
on the day spec 013 landed. On Linux it is Expected: the same exchange runs
through hidraw, and no bench there has read it yet.
*/
func Support() []support.Entry {
	return []support.Entry{{
		Driver:       driverName,
		Device:       "F75 via its 2.4 GHz receiver",
		Match:        "vendor 3554, report 0x13 on usage page 0xFF02; receiver 3554:fa09",
		Vendor:       receiverVendor,
		Products:     []uint16{0xFA09},
		Capabilities: []string{"battery"},
		Tier:         support.Expected,
		Spec:         13,
		Notes: "command 0x4A, level and power state in one 20-byte report; on its cable the level is " +
			"pinned at 100 and read as charging with no level; the keyboard on its cable (258a:010c) is not read",
		Windows: &support.Port{
			Tier:   support.Tested,
			Tested: windowsBenched,
			Notes:  "through the receiver's vendor collection, read-write, with the plain HID class driver",
		},
	}}
}

// windowsBenched is the date of spec 013's bench run on Windows.
var windowsBenched = time.Date(2026, time.October, 7, 0, 0, 0, 0, time.UTC)

// Describe is what the driver reads: the AULA F75 through its 2.4 GHz receiver,
// on Linux and Windows. Quiet: the receiver lists itself while the keyboard
// sleeps (spec 014).
func (Driver) Describe() sanshoku.Description {
	return sanshoku.Description{
		Name:         "AULA",
		Finds:        "receiver",
		Capabilities: []string{"battery"},
		Platforms:    []string{"linux", "windows"},
		Quiet:        true,
	}
}
