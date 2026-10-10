package sony

import (
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/support"
)

// windowsBenched is the date of spec 017's bench run on Windows.
var windowsBenched = time.Date(2026, time.October, 9, 0, 0, 0, 0, time.UTC)

/*
Support is the sony driver's support table: the controllers on the
allow-list.

The DualSense over Bluetooth is Tested on Windows, where the bench read it on
the day spec 017 landed. Over USB, and on Linux by either link, it is
Expected: the same report is read through hidraw, and the layout is the one
hid-playstation reads, but no bench has run there yet. The Edge is Expected
everywhere: same report, never on the desk.
*/
func Support() []support.Entry {
	return []support.Entry{
		{
			Driver:       driverName,
			Device:       "DualSense",
			Match:        "054c:0ce6, input report 0x01 (USB) or 0x31 (Bluetooth, after feature report 0x05 is read)",
			Vendor:       vendor,
			Products:     []uint16{0x0CE6},
			Capabilities: []string{"battery"},
			Tier:         support.Expected,
			Spec:         17,
			Notes:        "level in tenths and charge state from the input report's status byte, as hid-playstation reads it",
			Windows: &support.Port{
				Tier:   support.Tested,
				Tested: windowsBenched,
				Notes:  "Tested over Bluetooth; USB expected. Over Bluetooth the controller stays in its long-report mode until it reconnects",
			},
		},
		{
			Driver:       driverName,
			Device:       "DualSense Edge",
			Match:        "054c:0df2, as the DualSense",
			Vendor:       vendor,
			Products:     []uint16{0x0DF2},
			Capabilities: []string{"battery"},
			Tier:         support.Expected,
			Spec:         17,
			Notes:        "the DualSense's report; hid-playstation reads both the same way",
			Windows:      &support.Port{Tier: support.Expected},
		},
	}
}

// Describe is what the driver reads: a DualSense, on Linux and Windows. Not
// Quiet: an interface is there only while the controller is connected.
func (Driver) Describe() sanshoku.Description {
	return sanshoku.Description{
		Name:         "Sony",
		Finds:        "controller",
		Capabilities: []string{"battery"},
		Platforms:    []string{"linux", "windows"},
		Quiet:        false,
	}
}
