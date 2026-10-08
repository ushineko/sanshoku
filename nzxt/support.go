package nzxt

import (
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/support"
)

/*
Support is the nzxt driver's support table: the one product in Known.

The Kraken Elite is Tested, on the bench run spec 005 records. There are no
Expected entries: hotaru measured one product and the allow-list is one
product. Another Kraken gets an entry, and a place in Known, when someone runs
the bench on one.
*/
func Support() []support.Entry {
	return []support.Entry{{
		Driver:       driverName,
		Device:       Known[0x3012].Name,
		Match:        "1e71:3012, allow-list; vendor-defined usage page tried first",
		Vendor:       vendor,
		Products:     []uint16{0x3012},
		Capabilities: []string{"cooling", "screen"},
		Tier:         support.Tested,
		Hardware:     "NZXT Kraken Elite (1e71:3012)",
		Firmware:     "1.2.0 (USB bcdDevice 01.02)",
		Tested:       time.Date(2026, time.September, 29, 0, 0, 0, 0, time.UTC),
		Spec:         5,
		Notes:        "coolant, pump and fan over hidraw; the 640x640 LCD over usbfs, GIF only",
		Windows: &support.Port{
			Tier:  support.Expected,
			Notes: "coolant, pump and fan over HID; the screen needs usbfs, which Windows does not have",
		},
	}}
}

// Describe is what the driver reads: the Kraken's status on Linux and Windows,
// its screen on Linux only, which the support table says (spec 014).
func (Driver) Describe() sanshoku.Description {
	return sanshoku.Description{
		Name:         "NZXT",
		Finds:        "cooler",
		Capabilities: []string{"cooling", "screen"},
		Platforms:    []string{"linux", "windows"},
		Quiet:        false,
	}
}
