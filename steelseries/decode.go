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

// The headset's status, byte 15 of the Nova Pro's reply, as HeadsetControl
// names the two it acts on. 0x08 is online and is what the probe read with
// the headset on.
const (
	novaProOffline  = 0x01
	novaProCharging = 0x02
)

// novaProSteps is the top of the level's scale: HeadsetControl maps 0-8 onto
// 0-100, and the probe's 6 was the 75% it reported at the same moment.
const novaProSteps = 8

/*
DecodeNovaPro reads the Arctis Nova Pro Wireless base station's reply to
`06 b0`: the headset's level in byte 6 on a scale of 0 to 8, and its status in
byte 15.

The level is `reply[6] * 100 / 8`, so the headset goes in steps of 12.5%,
truncated, and a panel sees 75% and then 87%. A level above 8 is not a reading
and is an error; so is a reply too short to hold byte 15.

Status 0x01 is the headset switched off, or out of range, while the base
station answers for it: the reading is returned with HasLevel false, as
hayami's headset.go does, so a consumer shows the headset present with no
level rather than nothing. 0x02 is charging on the cable, and Full at a level
of 100. Anything else -- 0x08, online, is the one seen -- is Discharging, as
HeadsetControl treats it. Kind is KindHeadset; the name is the caller's to set,
from the kernel.
*/
func DecodeNovaPro(reply []byte) (battery.Battery, error) {
	if len(reply) < 16 {
		return battery.Battery{}, fmt.Errorf("an Arctis Nova Pro battery reply of %d bytes", len(reply))
	}
	b := battery.Battery{Kind: battery.KindHeadset}
	status := reply[15]
	if status == novaProOffline {
		return b, nil
	}

	step := reply[6]
	if step > novaProSteps {
		return battery.Battery{}, fmt.Errorf("an Arctis Nova Pro battery level of %d of %d", step, novaProSteps)
	}
	b.Level, b.HasLevel = int(step)*100/novaProSteps, true
	switch {
	case status != novaProCharging:
		b.State = battery.Discharging
	case b.Level == 100:
		b.State = battery.Full
	default:
		b.State = battery.Charging
	}
	return b, nil
}
