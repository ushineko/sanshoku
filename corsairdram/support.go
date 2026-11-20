package corsairdram

import (
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/support"
)

/*
Support is the corsairdram driver's support table: the Dominator Platinum
RGB DDR5, Tested on the bench run spec 042 records, and the other module families
OpenRGB knows, Expected.
*/
func Support() []support.Entry {
	return []support.Entry{{
		Driver:       driverName,
		Device:       "Corsair Dominator Platinum RGB DDR5",
		Match:        "the chipset SMBus, addresses 0x18 to 0x1F; registers 0x43 and 0x44; product 0600 or 0601",
		Vendor:       0x1B1C,
		Products:     []uint16{0x0600, 0x0601},
		Capabilities: []string{"lighting"},
		Tier:         support.Tested,
		Hardware:     "four Corsair Dominator Platinum RGB DDR5 modules on an Intel Raptor Lake SMBus (8086:7a23)",
		Tested:       time.Date(2026, time.November, 20, 0, 0, 0, 0, time.UTC),
		Spec:         42,
		Notes: "each module's 12 LEDs as a canvas, by direct frames; found by reads only; writes go only to the " +
			"device-information and frame registers",
		Windows: windowsPort,
	}, {
		Driver:       driverName,
		Device:       "Corsair RGB memory, other families",
		Match:        "the chipset SMBus, addresses 0x18 to 0x1F (DDR5) and 0x58 to 0x5F (DDR4); the product ID",
		Vendor:       0x1B1C,
		Products:     []uint16{0x0100, 0x0200, 0x0300, 0x0400, 0x0700, 0x0800, 0x0900, 0x0A00, 0x0B00},
		Capabilities: []string{"lighting"},
		Tier:         support.Expected,
		Spec:         42,
		Notes: "Vengeance RGB Pro, Dominator Platinum RGB, Vengeance RGB Pro SL and Vengeance RGB RS DDR4; Dominator " +
			"Titanium RGB, Vengeance RGB, Vengeance Shugo Series and Vengeance RGB RS DDR5; protocol 4 and later only",
		Windows: windowsPort,
	}, {
		Driver:       driverName,
		Device:       "Corsair Vengeance RGB (DDR4, the original)",
		Hardware:     "two Corsair Vengeance RGB DDR4 modules on an AMD B450's SMBus (1022:790b)",
		Match:        "the chipset SMBus, addresses 0x58 to 0x5F and 0x18 to 0x1F; registers 0xA0 to 0xAF read 0xBA",
		Vendor:       0x1B1C,
		Capabilities: []string{"lighting"},
		Tier:         support.Expected,
		Spec:         46,
		Notes:        "one light per module, a colour written register by register; found by reads only",
		Windows: &support.Port{
			Tier:   support.Tested,
			Tested: time.Date(2026, time.November, 20, 0, 0, 0, 0, time.UTC),
			Notes:  "through PawnIO (spec 044), as Administrator; a colour takes about 157 ms",
		},
	}}
}

// windowsPort is where the newer families stand on Windows: reached through
// PawnIO (spec 044), and not benched there.
var windowsPort = &support.Port{Tier: support.Expected, Notes: "through PawnIO's SMBus modules (spec 044), as Administrator"}

// Describe is what the driver reads: Corsair memory modules' lights.
func (Driver) Describe() sanshoku.Description {
	return sanshoku.Description{
		Name:         "Corsair memory",
		Finds:        "memory module",
		Capabilities: []string{"lighting"},
		Platforms:    []string{"linux", "windows"},
		Quiet:        false,
	}
}
