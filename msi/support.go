package msi

import (
	"slices"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/support"
)

// Support is the msi driver's support table: the RTX 4090 Suprim Liquid X,
// Tested on the bench run spec 029 records, and the cards that share its
// controller, Expected (spec 033). Linux only: it needs i2c-dev.
func Support() []support.Entry {
	return []support.Entry{{
		Driver:       driverName,
		Device:       "GeForce RTX 4090 Suprim Liquid X",
		Match:        "PCI 10de:2684, subsystem 1462:5104; NVIDIA i2c adapter 1, address 0x68",
		Vendor:       0x1462,
		Products:     []uint16{0x5104},
		Capabilities: []string{"lighting", "onboard"},
		Tier:         support.Tested,
		Hardware:     "MSI GeForce RTX 4090 Suprim Liquid X",
		Firmware:     "–",
		Tested:       time.Date(2026, time.November, 17, 0, 0, 0, 0, time.UTC),
		Spec:         29,
		Notes: "one light: a static colour as a canvas (six I2C writes with 20 ms after each, " +
			"about four colours a second at most), and the card's own effects; nothing is read back",
	}, {
		Driver:       driverName,
		Device:       "MSI GeForce RTX 40 and 50 cards on the same controller",
		Match:        "the 46 other cards OpenRGB pairs with this controller, each on all four PCI IDs; NVIDIA i2c adapter 1, address 0x68",
		Vendor:       0x1462,
		Products:     expectedSubsystems(),
		Capabilities: []string{"lighting", "onboard"},
		Tier:         support.Expected,
		Spec:         33,
		Notes: "the static colour as on the tested card; six cards lack Stack and Rhythm, and the RTX 50 " +
			"cards take the coloured Rhythm as mode 0B, as OpenRGB has them; the RTX 50 cards' four further " +
			"effects are not offered",
	}}
}

// expectedSubsystems are the subsystem IDs of every card but the tested one,
// each once, in order.
func expectedSubsystems() []uint16 {
	seen := map[uint16]bool{0x5104: true}
	var out []uint16
	for c := range cards {
		if !seen[c.subDevice] {
			seen[c.subDevice] = true
			out = append(out, c.subDevice)
		}
	}
	slices.Sort(out)
	return out
}

// Describe is what the driver reads: an MSI graphics card's light, on Linux.
func (Driver) Describe() sanshoku.Description {
	return sanshoku.Description{
		Name:         "MSI",
		Finds:        "graphics card",
		Capabilities: []string{"lighting", "onboard"},
		Platforms:    []string{"linux"},
		Quiet:        false,
	}
}
