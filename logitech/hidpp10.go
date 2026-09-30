package logitech

import (
	"context"
	"fmt"
	"time"

	"github.com/ushineko/sanshoku/battery"
	"github.com/ushineko/sanshoku/hidraw"
)

/*
HID++ 1.0 batteries, for devices older than the feature protocol.

A Logitech K800 in daily use since about 2016 answers a HID++ 2.0 root request
with "invalid sub-id" -- it has no features at all, so the 0x1004 and 0x1000
this package reads elsewhere do not exist on it. What it has instead is a
register, and what the register gives is a band rather than a percentage.

**Only getters are sent.** Sub-id 0x81 is GET_REGISTER_REQ. Its neighbour 0x80
writes, and appears nowhere in this file or this package.
*/

// The HID++ 1.0 sub-ids and registers this package reads.
const (
	// subGetRegister reads a short register. There is a long form, 0x83, which
	// nothing here needs.
	subGetRegister = 0x81

	// registerBatteryCharge carries a percentage on the devices that have a
	// fuel gauge, and answers an error on the ones that do not. The K800
	// answers the error.
	registerBatteryCharge = 0x0D

	// registerBatteryStatus carries the band, which is what a device without a
	// gauge knows about itself.
	registerBatteryStatus = 0x07
)

/*
bands maps the register's values.

solaar's, checked against one keyboard, which answered 0x05 for "good". A value
that is not one of these is not a band: an unknown reading yields nothing
rather than the nearest guess, because the whole point of a band is that it is
the device's own word and not an interpolation.
*/
var bands = map[byte]battery.Band{
	0x01: battery.BandCritical,
	0x03: battery.BandLow,
	0x05: battery.BandGood,
	0x07: battery.BandFull,
}

/*
register sends one HID++ 1.0 register read and returns the reply's parameters.

The 1.0 form is a different shape from the 2.0 one: a sub-id where a feature
index goes, and the register where a function goes, so it cannot share
request's framing. What it does share is the recognition of the 1.0 error form,
because the answer arrives on the same node amid the same unasked-for traffic.

One attempt, as hayami sends it: a register read is only made of a device that
discovery has just heard answer.
*/
func register(ctx context.Context, rd hidraw.ReportDevice, timeout time.Duration, index, reg byte) ([]byte, error) {
	req := []byte{reportShort, index, subGetRegister, reg, 0, 0, 0}
	r, err := attempt(ctx, rd, timeout, req, func(r []byte) bool {
		_, matched, _ := registerReply(r, index, reg)
		return matched
	})
	if err != nil {
		return nil, err
	}
	params, _, err := registerReply(r, index, reg)
	return params, err
}

// registerReply reads one report as the answer to a register read, or as
// somebody else's traffic.
func registerReply(r []byte, index, reg byte) (params []byte, matched bool, err error) {
	if len(r) < 5 || r[1] != index {
		return nil, false, nil
	}
	if r[2] == errorSub10 {
		if len(r) < 6 {
			return nil, false, nil
		}
		return nil, true, translate10(r[5])
	}
	if r[2] == subGetRegister && r[3] == reg {
		return r[4:], true, nil
	}
	return nil, false, nil
}

// readOldBattery reads a HID++ 1.0 battery, preferring a gauge to a band.
func readOldBattery(ctx context.Context, rd hidraw.ReportDevice, timeout time.Duration, index byte) (battery.Battery, error) {
	if params, err := register(ctx, rd, timeout, index, registerBatteryCharge); err == nil {
		if b, err := decodeChargeRegister(params); err == nil {
			return b, nil
		}
	}
	params, err := register(ctx, rd, timeout, index, registerBatteryStatus)
	if err != nil {
		return battery.Battery{}, err
	}
	return decodeStatusRegister(params)
}

// decodeChargeRegister reads register 0x0D, which is a percentage and a state.
func decodeChargeRegister(p []byte) (battery.Battery, error) {
	if len(p) < 2 {
		return battery.Battery{}, fmt.Errorf("a battery charge reply of %d bytes", len(p))
	}
	level := int(p[0])
	if level <= 0 || level > 100 {
		return battery.Battery{}, fmt.Errorf("a battery charge of %d", level)
	}
	return battery.Battery{Level: level, HasLevel: true, State: oldState(p[1])}, nil
}

/*
decodeStatusRegister reads register 0x07, which is the band.

The reply is `band, charge, 0`. The K800 this was written against answered
`05 00 00`: good, and discharging.
*/
func decodeStatusRegister(p []byte) (battery.Battery, error) {
	if len(p) < 2 {
		return battery.Battery{}, fmt.Errorf("a battery status reply of %d bytes", len(p))
	}
	band, ok := bands[p[0]]
	if !ok {
		return battery.Battery{}, fmt.Errorf("a battery band of %#02x", p[0])
	}
	return battery.Battery{Band: band, HasBand: true, State: oldState(p[1])}, nil
}

// oldState reads the charge byte, which is zero when a device is running on
// its battery and something else when it is not.
func oldState(charge byte) battery.State {
	if charge == 0 {
		return battery.Discharging
	}
	return battery.Charging
}
