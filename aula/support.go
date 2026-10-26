package aula

import (
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/support"
)

/*
Support is the aula driver's support table: the F75 by each of its two
devices.

The F75 on its 2.4 GHz receiver is Tested on Windows, where the bench read its
battery on the day spec 013 landed. On Linux it is Expected: the same exchange
runs through hidraw, and no bench there has read it yet. Its lighting, and the
keyboard on its cable, are spec 019: a prototype drew on both on Windows, and
neither entry is Tested for it until the bench has run this driver's frames
with someone watching.
*/
func Support() []support.Entry {
	return []support.Entry{{
		Driver:       driverName,
		Device:       "F75 via its 2.4 GHz receiver",
		Match:        "vendor 3554, report 0x13 on usage page 0xFF02; receiver 3554:fa09",
		Vendor:       receiverVendor,
		Products:     []uint16{0xFA09},
		Capabilities: []string{"battery", "lighting", "onboard"},
		Tier:         support.Expected,
		Spec:         13,
		Notes: "command 0x4A, level and power state in one 20-byte report; on its cable the level is " +
			"pinned at 100 and read as charging with no level. Lighting (spec 019) is frames of command 0x88, " +
			"at most 37 colours on the board at once, sent again within half a second to hold; " +
			"only a keyboard whose model reads 0xCD is drawn on. The firmware effect's colour (spec 020) " +
			"is set by writing the keyboard's flash (palette 0x09, then config block 0x04), only where it " +
			"differs; a new palette colour shows at once and does not survive a power cycle, which loads the " +
			"factory settings",
		Windows: &support.Port{
			Tier:   support.Tested,
			Tested: windowsBenched,
			Notes: "through the receiver's vendor collection, read-write, with the plain HID class driver; " +
				"the lighting was benched on 2026-10-10",
		},
	}, {
		Driver:       driverName,
		Device:       "F75 on its cable",
		Match:        "258a:010c, feature report 6 on usage page 0xFF00",
		Vendor:       wiredVendor,
		Products:     []uint16{wiredProduct},
		Capabilities: []string{"lighting"},
		Tier:         support.Expected,
		Spec:         19,
		Notes: "frames of command 0x08 on feature report 6, a colour for every key, about 66 ms a frame, " +
			"sent again within half a second to hold; no battery reading; only a keyboard whose model reads " +
			"0xCD is drawn on; report 5, the bootloader's, is never sent",
		Windows: &support.Port{
			Tier:   support.Tested,
			Tested: lightingBenched,
			Notes:  "through the keyboard's vendor collection, with the plain HID class driver",
		},
	}}
}

// windowsBenched is the date of spec 013's bench run on Windows.
var windowsBenched = time.Date(2026, time.October, 7, 0, 0, 0, 0, time.UTC)

// lightingBenched is the date of spec 019's bench runs on Windows.
var lightingBenched = time.Date(2026, time.October, 10, 0, 0, 0, 0, time.UTC)

// Describe is what the driver reads and draws on: the AULA F75 through its
// 2.4 GHz receiver or on its cable, on Linux and Windows. Quiet: the receiver lists itself while the keyboard
// sleeps (spec 014).
func (Driver) Describe() sanshoku.Description {
	return sanshoku.Description{
		Name:         "AULA",
		Finds:        "receiver",
		Capabilities: []string{"battery", "lighting", "onboard"},
		Platforms:    []string{"linux", "windows"},
		Quiet:        true,
	}
}
