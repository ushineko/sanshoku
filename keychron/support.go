package keychron

import (
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/support"
)

// Support is the keychron driver's support table: the K4 HE, Tested on the
// bench run spec 028 records.
func Support() []support.Entry {
	return []support.Entry{{
		Driver:       driverName,
		Device:       products[0x0E40],
		Match:        "vendor 3434, usage page 0xFF60; allow-list 3434:0e40",
		Vendor:       vendor,
		Products:     []uint16{0x0E40},
		Capabilities: []string{"lighting", "onboard"},
		Tier:         support.Tested,
		Hardware:     "Keychron K4 HE (3434:0e40)",
		Firmware:     "v1.1.1 2025-06-17",
		Tested:       time.Date(2026, time.November, 17, 0, 0, 0, 0, time.UTC),
		Spec:         28,
		Notes: "100 keys as a canvas through the per-key effect (23); each key's hue and saturation, " +
			"and one brightness for the board from the brightest key, as the firmware ignores a key's own; " +
			"a black key shows white; and its 22 own effects, named as OpenRGB names them (spec 031)",
		Windows: &support.Port{
			Tier:  support.Expected,
			Notes: "the same 32-byte VIA reports over HID",
		},
	}}
}

// Describe is what the driver reads: a Keychron keyboard's lights.
func (Driver) Describe() sanshoku.Description {
	return sanshoku.Description{
		Name:         "Keychron",
		Finds:        "keyboard",
		Capabilities: []string{"lighting", "onboard"},
		Platforms:    []string{"linux", "windows"},
		Quiet:        false,
	}
}
