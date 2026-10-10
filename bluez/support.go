package bluez

import (
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/support"
)

// benched is the date of spec 004's bench run, and windowsBenched of spec
// 016's on Windows.
var (
	benched        = time.Date(2026, time.September, 29, 0, 0, 0, 0, time.UTC)
	windowsBenched = time.Date(2026, time.October, 9, 0, 0, 0, 0, time.UTC)
)

/*
Support is the bluez driver's support table: one driver-wide entry with no
vendor, which support.Lookup matches for any candidate of this driver.

hayami never verified the Battery1 path on hardware (hayami issue #24). Spec
004's bench read it from a Sony WH-1000XM6 (054c:0f8a), at the level BlueZ's
own property held, so the entry is Tested on that headset.

On Windows (spec 016) the level is the Bluetooth stack's own battery property,
read from the device tree; the bench read a Bose QC35 there at the level
Windows' Bluetooth settings showed.
*/
func Support() []support.Entry {
	return []support.Entry{
		{
			Driver:       driverName,
			Device:       "Any connected device with Battery1",
			Match:        "connected org.bluez.Device1 with org.bluez.Battery1, not Apple audio",
			Capabilities: []string{"battery"},
			Tier:         support.Tested,
			Hardware:     "Sony WH-1000XM6",
			Tested:       benched,
			Spec:         4,
			Notes:        "BlueZ's own level, read over the system bus; nothing is sent to the device",
			Windows: &support.Port{
				Tier:   support.Tested,
				Tested: windowsBenched,
				Notes: "the Bluetooth stack's battery property, read from the device tree; nothing is sent " +
					"to the device. Tested on a Bose QC35; a DualSense has no such property",
			},
		},
	}
}

// Describe is what the driver reads: connected Bluetooth devices with a battery,
// through BlueZ on Linux and the Bluetooth stack's device properties on Windows
// (spec 016). Not Quiet: only connected devices are listed (spec 014).
func (Driver) Describe() sanshoku.Description {
	return sanshoku.Description{
		Name:         "Bluetooth",
		Finds:        "device with a battery",
		Capabilities: []string{"battery"},
		Platforms:    []string{"linux", "windows"},
		Quiet:        false,
	}
}
