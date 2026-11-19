package gigabyte

import (
	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/support"
)

/*
Support is the gigabyte driver's support table: the Fusion 2 USB controllers
on the Z790 AORUS MASTER X. Expected until spec 039's bench run on the board.
*/
func Support() []support.Entry {
	return []support.Entry{{
		Driver:       driverName,
		Device:       "RGB Fusion 2 USB mainboard controller",
		Match:        "vendor 048d, usage page 0xFF89; product 048d:5702; boards in the driver's table",
		Vendor:       vendor,
		Products:     []uint16{0x5702},
		Capabilities: []string{"lighting"},
		Tier:         support.Expected,
		Spec:         39,
		Notes: "each controller's addressable headers and single-colour lights as a canvas with zones, for a " +
			"board whose layout has been measured (the Z790 AORUS MASTER X); a header's length is the consumer's " +
			"to set; nothing is written to the board's flash; not on Windows, where the board's name, and so " +
			"its layout, is not read",
	}}
}

// Describe is what the driver reads: a Gigabyte mainboard's lights.
func (Driver) Describe() sanshoku.Description {
	return sanshoku.Description{
		Name:         "Gigabyte",
		Finds:        "mainboard",
		Capabilities: []string{"lighting"},
		Platforms:    []string{"linux"},
		Quiet:        false,
	}
}
