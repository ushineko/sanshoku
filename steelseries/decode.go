package steelseries

import (
	"fmt"

	"github.com/ushineko/sanshoku/battery"
)

// chargingFlag is bit 7 of the value byte.
const chargingFlag = 0x80

/*
DecodeModern reads the two-byte reply to the modern battery command, 0x92 or
0xD2: the echoed command, then the value.

Bit 7 of the value is charging and the rest is a level. The level has a step of
**five**: `((v & 0x7f) - 1) * 5` is rivalcfg's arithmetic and the device's own
resolution, so a panel goes 95 % and then 100 % with nothing between. That
looks like a dropped reading and is not one.

A value of zero would decode to -5, which is how a reply that is present and
means nothing announces itself; it, and anything above 100, is an error rather
than a reading. So is a reply too short to hold a value.
*/
func DecodeModern(reply []byte) (battery.Battery, error) {
	if len(reply) < 2 {
		return battery.Battery{}, fmt.Errorf("a SteelSeries battery reply of %d bytes", len(reply))
	}

	v := reply[1]
	level := (int(v&^chargingFlag) - 1) * 5
	if level < 0 || level > 100 {
		return battery.Battery{}, fmt.Errorf("a SteelSeries battery value of %#02x", v)
	}

	b := battery.Battery{Level: level, HasLevel: true}
	switch {
	case v&chargingFlag == 0:
		b.State = battery.Discharging
	case level == 100:
		// Charging and full are the same bit here, so the level is what tells
		// them apart. A reading that said "charging" against 100 % for the
		// rest of the day would be wrong for as long as the cable stayed in.
		b.State = battery.Full
	default:
		b.State = battery.Charging
	}
	return b, nil
}
