package logitech

import (
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/support"
)

// The receiver product IDs the entries name. Found by the node the HID++
// endpoint is on, which for a device behind a receiver is the receiver.
const (
	// productLightspeed is the Lightspeed receiver the G502 X PLUS was benched
	// through, as the bench reported it.
	productLightspeed = 0xC547
	// productUnifying is the Unifying receiver hayami's K800 was measured on
	// (hayami generations_test.go). Not on the bench machine.
	productUnifying = 0xC52B
	// productK800 is the K800's own node under hid-logitech-dj, which is where
	// a HID++ 1.0 device is read (hayami generations_test.go).
	productK800 = 0x2010
)

// hidppMatch is the rule Find matches by, in words.
const hidppMatch = "vendor 046d, report ID 0x10 on a vendor page"

/*
Support is the logitech driver's support table.

The G502 X PLUS through its Lightspeed receiver is Tested: spec 002's bench read
it and solaar agreed. The K800 through a Unifying receiver is Tested: the bench
on the machine that has it read the register 0x07 band on 2026-09-29. The two
protocol families are Expected, because every device that speaks them goes
through the same code path.
*/
func Support() []support.Entry {
	return []support.Entry{
		{
			Driver:       driverName,
			Device:       "G502 X PLUS via Lightspeed receiver",
			Match:        hidppMatch + "; receiver 046d:c547",
			Vendor:       logitechVendor,
			Products:     []uint16{productLightspeed},
			Capabilities: []string{"battery"},
			Tier:         support.Tested,
			Hardware:     "Logitech G502 X PLUS, Lightspeed receiver",
			Tested:       benched,
			Spec:         2,
			Notes:        "HID++ 2.0, feature 0x1004",
			Windows:      windowsExpected,
		},
		{
			Driver:       driverName,
			Device:       "K800 via Unifying receiver",
			Match:        hidppMatch + "; receiver 046d:c52b, read on the keyboard's own node 046d:2010",
			Vendor:       logitechVendor,
			Products:     []uint16{productUnifying, productK800},
			Capabilities: []string{"battery"},
			Tier:         support.Tested,
			Hardware:     "Logitech K800, Unifying receiver",
			Tested:       benched,
			Spec:         2,
			Notes:        "HID++ 1.0, register 0x07 band; read Good, discharging; its node is asked at its own index only (spec 007): asleep, it finishes in 4 ms, where discovery cost 9.7 s",
			Windows: &support.Port{
				Tier: support.Expected,
				Notes: "no node of its own: read on the receiver at index 1 and named from its pairing register; " +
					"register 0x07 answered there through the HID class driver (spec 012), not yet through the driver",
			},
		},
		{
			Driver:       driverName,
			Device:       "Any HID++ 2.0 device with feature 0x1004 or 0x1000",
			Match:        hidppMatch,
			Vendor:       logitechVendor,
			Capabilities: []string{"battery"},
			Tier:         support.Expected,
			Spec:         2,
			Notes:        "wired on index 0xFF or paired on 1..6",
			Windows:      windowsExpected,
		},
		{
			Driver:       driverName,
			Device:       "Any HID++ 1.0 device with register 0x0D or 0x07",
			Match:        hidppMatch + ", on a paired device's own node",
			Vendor:       logitechVendor,
			Capabilities: []string{"battery"},
			Tier:         support.Expected,
			Spec:         2,
			Notes:        "0x0D is a percentage; 0x07 is a band of four",
			Windows: &support.Port{
				Tier:  support.Expected,
				Notes: "on the receiver's node, where Windows gives a paired device none of its own",
			},
		},
	}
}

// benched is the date of spec 002's bench run.
var benched = time.Date(2026, time.September, 29, 0, 0, 0, 0, time.UTC)

// windowsExpected is an entry whose code path runs on Windows and which no
// bench there has confirmed: the HID++ exchange is the same, through the
// receiver's collections put back together (spec 012).
var windowsExpected = &support.Port{Tier: support.Expected}

// Describe is what the driver reads: Logitech receivers and their devices, on
// Linux and Windows (spec 012). Not Quiet: a receiver reports what is silent on
// it through Presence (spec 014).
func (Driver) Describe() sanshoku.Description {
	return sanshoku.Description{
		Name:         "Logitech",
		Finds:        "receiver",
		Capabilities: []string{"battery"},
		Platforms:    []string{"linux", "windows"},
		Quiet:        false,
	}
}
