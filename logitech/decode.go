package logitech

import (
	"fmt"

	"github.com/ushineko/sanshoku/battery"
)

/*
DecodeUnifiedBattery reads the parameters of a feature 0x1004 (Unified
Battery) function 1 reply: the bytes after the report ID, device index,
feature index and function.

p[0] is the state of charge, used when it is a percentage (at most 100);
p[2] is the charging state, 1 or 2 charging, 3 full, anything else
discharging. The device reports a level band as well, in p[1], and the band is
what a device with no fuel gauge actually knows; it is not turned into a
number, because a device saying "good" does not mean 75 %. A reply too short
to hold a reading is an error, never a zero. The name and kind are left for
the caller.
*/
func DecodeUnifiedBattery(p []byte) (battery.Battery, error) {
	if len(p) < 3 {
		return battery.Battery{}, fmt.Errorf("a unified battery reply of %d bytes", len(p))
	}

	var b battery.Battery
	if soc := int(p[0]); soc <= 100 {
		b.Level, b.HasLevel = soc, true
	}

	switch p[2] {
	case 0x01, 0x02: // charging, charging slowly
		b.State = battery.Charging
	case 0x03: // charging complete
		b.State = battery.Full
	default:
		b.State = battery.Discharging
	}
	return b, nil
}

/*
DecodeBatteryStatus reads the parameters of a feature 0x1000 (Battery Status)
function 0 reply, which is what a device without a fuel gauge has instead of
0x1004.

p[0] is the level, used when it is a percentage (at most 100); p[2] is the
charging state, 1 or 4 charging, 2 or 3 full, anything else discharging. A
reply too short to hold a reading is an error, never a zero.
*/
func DecodeBatteryStatus(p []byte) (battery.Battery, error) {
	if len(p) < 3 {
		return battery.Battery{}, fmt.Errorf("a battery status reply of %d bytes", len(p))
	}

	var b battery.Battery
	if level := int(p[0]); level <= 100 {
		b.Level, b.HasLevel = level, true
	}

	switch p[2] {
	case 0x01, 0x04: // recharging, slow recharge
		b.State = battery.Charging
	case 0x02, 0x03: // almost full, full
		b.State = battery.Full
	default:
		// 0x05 invalid battery, 0x06 thermal error and 0x07 charging error all
		// land here. They are faults of the charger, not readings, and the
		// level beside them is still the level.
		b.State = battery.Discharging
	}
	return b, nil
}
