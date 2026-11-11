package powersupply

import (
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/support"
)

// benched is the date of spec 022's bench run.
var benched = time.Date(2026, time.October, 28, 0, 0, 0, 0, time.UTC)

/*
Support is the powersupply driver's support table: the Switch Pro Controller,
and a driver-wide entry that support.Lookup matches for any other candidate of
this driver.

The Pro Controller is Tested: spec 022's bench read an 8BitDo pad in its
Switch pose over Bluetooth, through hid-nintendo, as a band. Any other device
is Expected: the same files, written by another kernel driver. Linux only.
*/
func Support() []support.Entry {
	return []support.Entry{
		{
			Driver:       driverName,
			Device:       "Nintendo Switch Pro Controller",
			Match:        "057e:2009, hid-nintendo's power_supply node",
			Vendor:       0x057E,
			Products:     []uint16{0x2009},
			Capabilities: []string{"battery"},
			Tier:         support.Tested,
			Hardware:     "8BitDo controller in its Switch (Bluetooth) mode",
			Tested:       benched,
			Spec:         22,
			Notes:        "a capacity level and no percentage, read as one of four bands",
		},
		{
			Driver:       driverName,
			Device:       "Any HID device whose battery the kernel reads",
			Match:        "/sys/class/power_supply node with scope=Device on a HID device, unless another driver found the same device",
			Capabilities: []string{"battery"},
			Tier:         support.Expected,
			Spec:         22,
			Notes: "the kernel driver's own reading (hid-nintendo, xpadneo, hid-input); a level, or one of " +
				"four bands where the kernel has only a capacity level; nothing is sent to the device",
		},
	}
}

// Describe is what the driver reads: the batteries the kernel reads for HID
// devices, on Linux. Not Quiet: a node is there only while the device is
// connected.
func (Driver) Describe() sanshoku.Description {
	return sanshoku.Description{
		Name:         "HID",
		Finds:        "device the kernel has a battery for",
		Capabilities: []string{"battery"},
		Platforms:    []string{"linux"},
		Quiet:        false,
	}
}
