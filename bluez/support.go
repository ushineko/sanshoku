package bluez

import (
	"time"

	"github.com/ushineko/sanshoku/support"
)

// benched is the date of spec 004's bench run.
var benched = time.Date(2026, time.September, 29, 0, 0, 0, 0, time.UTC)

/*
Support is the bluez driver's support table: one driver-wide entry with no
vendor, which support.Lookup matches for any candidate of this driver.

hayami never verified the Battery1 path on hardware (hayami issue #24). Spec
004's bench read it from a Sony WH-1000XM6 (054c:0f8a), at the level BlueZ's
own property held, so the entry is Tested on that headset.
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
		},
	}
}
