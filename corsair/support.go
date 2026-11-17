package corsair

import (
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/support"
)

/*
Support is the corsair driver's support table: the MM700, Tested on the bench
run spec 026 records. Windows is Expected: the same reports over HID.
*/
func Support() []support.Entry {
	return []support.Entry{{
		Driver:       driverName,
		Device:       products[0x1B9B],
		Match:        "vendor 1b1c, usage page 0xFF42; allow-list 1b1c:1b9b",
		Vendor:       vendor,
		Products:     []uint16{0x1B9B},
		Capabilities: []string{"lighting"},
		Tier:         support.Tested,
		Hardware:     "Corsair MM700 RGB mousepad (1b1c:1b9b)",
		Firmware:     "property 0x13 reads 00050001",
		Tested:       time.Date(2026, time.November, 17, 0, 0, 0, 0, time.UTC),
		Spec:         26,
		Notes: "three lights (Left, Right, Logo) as a canvas that holds its frame; software mode is claimed " +
			"before a frame because one sent in hardware mode is accepted and not shown; release returns " +
			"the stored effect",
		Windows: &support.Port{
			Tier:  support.Expected,
			Notes: "the same 64-byte reports over HID",
		},
	}, {
		Driver:       driverName,
		Device:       products[0x1BC9],
		Match:        "vendor 1b1c, usage page 0xFF42; allow-list 1b1c:1bc9",
		Vendor:       vendor,
		Products:     []uint16{0x1BC9},
		Capabilities: []string{"lighting"},
		Tier:         support.Expected,
		Spec:         33,
		Notes:        "OpenRGB's V2 table gives it the MM700's entry: Left, Right and Logo, one light each",
		Windows:      &support.Port{Tier: support.Expected, Notes: "the same 64-byte reports over HID"},
	}}
}

// Describe is what the driver reads: a Corsair V2 device's lights.
func (Driver) Describe() sanshoku.Description {
	return sanshoku.Description{
		Name:         "Corsair",
		Finds:        "mousepad",
		Capabilities: []string{"lighting"},
		Platforms:    []string{"linux", "windows"},
		Quiet:        false,
	}
}
