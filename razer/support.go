package razer

import "github.com/ushineko/sanshoku/support"

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

Every entry is Expected. The Mouse Dock Pro was measured by hayami (spec 016)
and was not on the desk when spec 003's bench ran, so it waits for a bench run
to be promoted to Tested. The older Mouse Dock and the Basilisk Ultimate's
dongle are in hayami's product table and have not been read by either.
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
			Tier:         support.Expected,
			Spec:         3,
			Notes:        "the mouse is read through the dock's RF relay, transaction 0x1F; measured by hayami, not on spec 003's bench",
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
			Notes:        "OpenRazer addresses it on transaction 0x3F; a charger that may answer not-supported",
		},
		{
			Driver:       driverName,
			Device:       "Basilisk Ultimate via its dongle",
			Match:        razerMatch + "; 1532:0088",
			Vendor:       razerVendor,
			Products:     []uint16{productBasiliskUlt},
			Capabilities: []string{"battery"},
			Tier:         support.Expected,
			Spec:         3,
			Notes:        "in hayami's product table; not read by hayami or this module",
		},
	}
}
