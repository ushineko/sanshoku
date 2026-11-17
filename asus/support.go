package asus

import (
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/support"
)

/*
Support is the asus driver's support table: the Aura USB controller on the
ROG Maximus Z790 Hero, Tested on the bench run spec 027 records, and the
products that share its protocol, Expected (spec 033).
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
	}, {
		Driver:       driverName,
		Device:       "Aura USB mainboard controller, other products",
		Match:        "vendor 0b05, usage page 0xFF72; allow-list 0b05:1939, 0b05:19af (usage 0xA1), 0b05:1aa6, 0b05:1bed",
		Vendor:       vendor,
		Products:     []uint16{0x1939, 0x19AF, 0x1AA6, 0x1BED},
		Capabilities: []string{"lighting"},
		Tier:         support.Expected,
		Spec:         33,
		Notes: "the controllers OpenRGB's Aura mainboard detector lists with 18f3; the layout is read from " +
			"the controller's table, and firmware AULA3-AR32 numbers its channels headers first",
		Windows: &support.Port{Tier: support.Expected, Notes: "19af is read on its usage 0xA1 collection"},
	}, {
		Driver:       driverName,
		Device:       "ASUS ROG Strix LC III",
		Match:        "vendor 0b05, usage page 0xFF72; allow-list 0b05:1b29",
		Vendor:       vendor,
		Products:     []uint16{0x1B29},
		Capabilities: []string{"lighting"},
		Tier:         support.Expected,
		Spec:         33,
		Notes:        "a liquid cooler's pump on the same controller and protocol, named for itself",
		Windows:      &support.Port{Tier: support.Expected},
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
