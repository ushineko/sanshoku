package asus

import (
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/support"
)

/*
Support is the asus driver's support table: the Aura USB controller on the
ROG Maximus Z790 Hero, Tested on the bench run spec 027 records.
*/
func Support() []support.Entry {
	return []support.Entry{{
		Driver:       driverName,
		Device:       "Aura USB mainboard controller",
		Match:        "vendor 0b05, usage page 0xFF72; allow-list 0b05:18f3",
		Vendor:       vendor,
		Products:     []uint16{0x18F3},
		Capabilities: []string{"lighting"},
		Tier:         support.Tested,
		Hardware:     "ASUS ROG Maximus Z790 Hero (0b05:18f3)",
		Firmware:     "AULA3-6K75-0309",
		Tested:       time.Date(2026, time.November, 17, 0, 0, 0, 0, time.UTC),
		Spec:         27,
		Notes: "the board's own light and four addressable headers as a canvas with zones; a header's " +
			"length is the consumer's to set, as the board cannot read it; direct mode is claimed per " +
			"channel before its first frame",
		Windows: &support.Port{
			Tier:  support.Expected,
			Notes: "the same 65-byte reports over HID; the board's name is not read there",
		},
	}}
}

// Describe is what the driver reads: an Aura mainboard's lights.
func (Driver) Describe() sanshoku.Description {
	return sanshoku.Description{
		Name:         "ASUS",
		Finds:        "mainboard",
		Capabilities: []string{"lighting"},
		Platforms:    []string{"linux", "windows"},
		Quiet:        false,
	}
}
