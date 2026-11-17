package nzxt

import (
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/support"
)

/*
Support is the nzxt driver's support table: each product in Known.

The Kraken Elite is Tested, on the bench run spec 005 records. The Kraken 2024
Plus is Expected (spec 033): OpenRGB drives its lighting with the commands
this driver sends, and liquidctl its status and screen as the Elite's; nothing
here has run on one.
*/
func Support() []support.Entry {
	return []support.Entry{{
		Driver:       driverName,
		Device:       Known[0x3012].Name,
		Match:        "1e71:3012, allow-list; vendor-defined usage page tried first",
		Vendor:       vendor,
		Products:     []uint16{0x3012},
		Capabilities: []string{"cooling", "screen", "lighting", "onboard"},
		Tier:         support.Tested,
		Hardware:     "NZXT Kraken Elite (1e71:3012)",
		Firmware:     "1.2.12 (the `11 02` reply; USB bcdDevice reads 01.02 and does not follow it)",
		Tested:       time.Date(2026, time.September, 29, 0, 0, 0, 0, time.UTC),
		Spec:         5,
		Notes:        "coolant, pump and fan over hidraw; the 640x640 LCD over usbfs, GIF only; the ring and fans as a canvas over hidraw, and the firmware's own effects (specs 024, 025)",
		Windows: &support.Port{
			Tier:  support.Expected,
			Notes: "coolant, pump, fan and lighting over HID; the screen needs usbfs, which Windows does not have",
		},
	}, {
		Driver:       driverName,
		Device:       Known[0x3014].Name,
		Match:        "1e71:3014, allow-list; vendor-defined usage page tried first",
		Vendor:       vendor,
		Products:     []uint16{0x3014},
		Capabilities: []string{"cooling", "screen", "lighting", "onboard"},
		Tier:         support.Expected,
		Spec:         33,
		Notes: "one lighting channel, its fans (accessory 2A, three of eight) taken as attached where it " +
			"lists none, as OpenRGB takes them; the 240x240 LCD is liquidctl's figure, not measured",
		Windows: &support.Port{Tier: support.Expected, Notes: "as the Kraken Elite: the screen needs usbfs"},
	}}
}

// Describe is what the driver reads: the Kraken's status and lighting on Linux
// and Windows, its screen on Linux only, which the support table says (spec
// 014, spec 024).
func (Driver) Describe() sanshoku.Description {
	return sanshoku.Description{
		Name:         "NZXT",
		Finds:        "cooler",
		Capabilities: []string{"cooling", "screen", "lighting", "onboard"},
		Platforms:    []string{"linux", "windows"},
		Quiet:        false,
	}
}
