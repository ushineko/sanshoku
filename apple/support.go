package apple

import (
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/support"
)

/*
Support is the apple driver's support table.

AirPods Pro (Modalias product 2027) are Tested: the bench read left and right
on the desk on 2026-09-30. Every other Apple audio accessory is one Expected
entry, because every one goes through the same exchange; a bench run on
another generation promotes an entry for it, named as the product reports it.
*/
func Support() []support.Entry {
	return []support.Entry{
		{
			Driver:       driverName,
			Device:       "AirPods Pro",
			Match:        "connected BlueZ device, Modalias vendor 004c product 2027, audio; L2CAP PSM 0x1001",
			Vendor:       appleVendor,
			Products:     []uint16{0x2027},
			Capabilities: []string{"battery"},
			Tier:         support.Tested,
			Hardware:     "Apple AirPods Pro (product 2027)",
			Tested:       benched,
			Spec:         4,
			Notes:        "left and right read 98% over the accessory protocol; the case is reported only when the buds are in it",
		},
		{
			Driver:       driverName,
			Device:       "Other AirPods and Apple audio accessories",
			Match:        "connected BlueZ device, Modalias vendor 004c, audio icon or audio sink profile; L2CAP PSM 0x1001",
			Vendor:       appleVendor,
			Capabilities: []string{"battery"},
			Tier:         support.Expected,
			Spec:         4,
			Notes:        "per-ear and case levels over the accessory protocol, falling back to Battery1; one generation measured",
		},
	}
}

// benched is the date of the bench run with AirPods Pro connected.
var benched = time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC)

// Describe is what the driver reads: Apple accessories among the Bluetooth
// devices, over L2CAP, on Linux only. Named Bluetooth, as BlueZ's driver is:
// to a person they are the Bluetooth devices on the desk (spec 014).
func (Driver) Describe() sanshoku.Description {
	return sanshoku.Description{
		Name:         "Bluetooth",
		Finds:        "device with a battery",
		Capabilities: []string{"battery"},
		Platforms:    []string{"linux"},
		Quiet:        false,
	}
}
