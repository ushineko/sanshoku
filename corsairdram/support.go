package corsairdram

import (
	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/support"
)

/*
Support is the corsairdram driver's support table: the Dominator Platinum
RGB DDR5, which spec 042's bench run tests, and the other module families
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
		Tier:         support.Expected,
		Spec:         42,
		Notes: "each module's 12 LEDs as a canvas, by direct frames; found by reads only; writes go only to the " +
			"device-information and frame registers",
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
	}}
}

// Describe is what the driver reads: Corsair memory modules' lights.
func (Driver) Describe() sanshoku.Description {
	return sanshoku.Description{
		Name:         "Corsair memory",
		Finds:        "memory module",
		Capabilities: []string{"lighting"},
		Platforms:    []string{"linux"},
		Quiet:        false,
	}
}
