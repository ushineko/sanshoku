package evga

import (
	"fmt"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/support"
)

/*
Support is the evga driver's support table: the RTX 3080 FTW3 Ultra LHR,
Tested on the bench run spec 041 records, and the other cards OpenRGB pairs with the
same controller, Expected.
*/
func Support() []support.Entry {
	return []support.Entry{{
		Driver:       driverName,
		Device:       "EVGA GeForce RTX 3080 FTW3 Ultra LHR",
		Match:        "NVIDIA adapter 1 on PCI 10de:2216, subsystem 3842:4897; address 0x2D",
		Vendor:       0x3842,
		Products:     []uint16{0x4897},
		Capabilities: []string{"lighting"},
		Tier:         support.Tested,
		Hardware:     "EVGA GeForce RTX 3080 FTW3 Ultra LHR (10de:2216, 3842:4897)",
		Tested:       time.Date(2026, time.November, 20, 0, 0, 0, 0, time.UTC),
		Spec:         41,
		Notes: "each lit zone (the logos and the addressable header) one colour, as a canvas; release puts " +
			"back what the card showed at open; the card's save register is never written",
	}, {
		Driver:       driverName,
		Device:       "EVGA Ampere graphics cards",
		Match:        "NVIDIA adapter 1 on the cards' PCI IDs; address 0x2D",
		Vendor:       0x3842,
		Capabilities: []string{"lighting"},
		Tier:         support.Expected,
		Spec:         41,
		Notes:        fmt.Sprintf("%d cards OpenRGB 1.1 pairs with the same controller", len(cards)-1),
	}}
}

// Describe is what the driver reads: an EVGA graphics card's lights.
func (Driver) Describe() sanshoku.Description {
	return sanshoku.Description{
		Name:         "EVGA",
		Finds:        "graphics card",
		Capabilities: []string{"lighting"},
		Platforms:    []string{"linux"},
		Quiet:        false,
	}
}
