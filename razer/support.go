package razer

import (
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/support"
)

// The product IDs the entries name, as hayami's kinds table has them.
const (
	productMouseDock    = 0x007E
	productBasiliskUlt  = 0x0088
	productMouseDockPro = 0x00A4
)

// razerMatch is the rule Find matches by, in words.
const razerMatch = "vendor 1532, usage page 0xFF00 or 0xFF01"

/*
Support is the razer driver's support table.

The Mouse Dock Pro and the Basilisk Ultimate's dongle are Tested: the bench
read the mouse through each on the two machines that have them, on
2026-09-29. The older Mouse Dock is Expected: it was found and answered
nothing with the mouse off it, which is what a charger with nothing on it
does, and no run has yet caught the mouse sitting on it.
*/
func Support() []support.Entry {
	return []support.Entry{
		{
			Driver:       driverName,
			Device:       "Mouse Dock Pro, and the mouse on it",
			Match:        razerMatch + "; 1532:00a4",
			Vendor:       razerVendor,
			Products:     []uint16{productMouseDockPro},
			Capabilities: []string{"battery"},
			Tier:         support.Tested,
			Hardware:     "Razer Mouse Dock Pro with its mouse docked",
			Tested:       benched,
			Spec:         3,
			Notes:        "the mouse is read through the dock's RF relay, transaction 0x1F; read 100% full on the dock",
			Windows:      &support.Port{Tier: support.Expected},
		},
		{
			Driver:       driverName,
			Device:       "Mouse Dock",
			Match:        razerMatch + "; 1532:007e",
			Vendor:       razerVendor,
			Products:     []uint16{productMouseDock},
			Capabilities: []string{"battery"},
			Tier:         support.Expected,
			Spec:         3,
			Notes:        "OpenRazer addresses it on transaction 0x3F; found on a bench and answered nothing with the mouse off it, as a charger does",
			Windows: &support.Port{
				Tier:  support.Expected,
				Notes: "answered status 0x05, not supported, with no mouse on it, as on Linux",
			},
		},
		{
			Driver:       driverName,
			Device:       "Basilisk Ultimate via its dongle",
			Match:        razerMatch + "; 1532:0088",
			Vendor:       razerVendor,
			Products:     []uint16{productBasiliskUlt},
			Capabilities: []string{"battery"},
			Tier:         support.Tested,
			Hardware:     "Razer Basilisk Ultimate, its dongle",
			Tested:       benched,
			Spec:         3,
			Notes:        "read 63% discharging through the dongle",
			Windows: &support.Port{
				Tier:   support.Tested,
				Tested: windowsBenched,
				Notes:  "read 77% discharging, feature reports on interface 0 opened with no access",
			},
		},
	}
}

// benched is the date of the bench runs on the two machines with Razer devices.
var benched = time.Date(2026, time.September, 29, 0, 0, 0, 0, time.UTC)

// windowsBenched is the date of spec 012's bench run on Windows.
var windowsBenched = time.Date(2026, time.October, 7, 0, 0, 0, 0, time.UTC)

// Describe is what the driver reads: Razer docks, dongles and mice, on Linux
// and Windows. Quiet: a dock lists itself whether or not its mouse is awake
// (spec 014).
func (Driver) Describe() sanshoku.Description {
	return sanshoku.Description{
		Name:         "Razer",
		Finds:        "device",
		Capabilities: []string{"battery"},
		Platforms:    []string{"linux", "windows"},
		Quiet:        true,
	}
}
