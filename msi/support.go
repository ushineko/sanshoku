package msi

import (
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/support"
)

// Support is the msi driver's support table: the RTX 4090 Suprim Liquid X,
// Tested on the bench run spec 029 records. Linux only: it needs i2c-dev.
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
	}}
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
