package aula

import (
	"errors"
	"fmt"

	"github.com/ushineko/sanshoku/battery"
)

/*
The receiver's battery exchange, learned from PyFlat/Device-Battery-Info PR #6
and deepan-alve/womier-l65-linux (docs/credits.md) and measured on an F75's
receiver on Windows (spec 013).

Every report on the vendor collection is 20 bytes: report 0x13, a command, the
body, and in the last byte the sum of the nineteen before it. The battery
question is command 0x4A with an empty body. The answer echoes the command,
with the high bit sometimes set, and carries the level at byte 5 and the power
state at byte 6:

	13 4a 01 00 02 61 01 00 00 00 00 00 00 00 00 00 00 00 00 c2   97 %, on battery
	13 4a 01 00 02 64 10 00 00 00 00 00 00 00 00 00 00 00 00 d4   cable in

Only the getter is sent. The receiver speaks other commands on the same report
-- it pushes a 0x0A status frame unasked -- and none of them is sent here.
*/
const (
	reportID   = 0x13
	commandBat = 0x4A
	reportLen  = 20

	levelAt = 5
	stateAt = 6

	// stateBattery is the keyboard running on its battery.
	stateBattery = 0x01
	// stateCable is the keyboard on its cable. The level reads 100 whatever
	// the battery holds, so it is not a level at all.
	stateCable = 0x10
)

// request is the battery question, checksum included: 13 4a 00 ... 00 5d.
func request() []byte {
	r := make([]byte, reportLen)
	r[0], r[1] = reportID, commandBat
	r[reportLen-1] = checksum(r)
	return r
}

// checksum is the sum of the first nineteen bytes, wrapping at a byte.
func checksum(r []byte) byte {
	var sum byte
	for _, b := range r[:reportLen-1] {
		sum += b
	}
	return sum
}

// answers reports whether a report is the reply to the battery question: the
// report and command echoed, whole, and summing to its last byte. The
// receiver's other frames on report 0x13 are somebody else's traffic.
func answers(r []byte) bool {
	return len(r) >= reportLen && r[0] == reportID && r[1]&0x7F == commandBat && checksum(r) == r[reportLen-1]
}

// errNoReading is a reply that is a reply and not a reading.
var errNoReading = errors.New("no reading in the reply")

/*
Decode reads a battery reply into a reading.

On battery the level is the keyboard's own, 1 to 100. On its cable the
receiver pins the level at 100, so the reading is charging with no level: a
100 % that is not one would be the panel's lie. A level out of range, or a
state this driver has not seen, is no reading rather than a guess: the keyboard
is there and has not said anything this driver can stand behind.
*/
func Decode(r []byte) (battery.Battery, error) {
	if !answers(r) {
		return battery.Battery{}, fmt.Errorf("not a battery reply: % x", r)
	}
	switch state := r[stateAt]; state {
	case stateCable:
		return battery.Battery{State: battery.Charging, Kind: battery.KindKeyboard}, nil
	case stateBattery:
		level := int(r[levelAt])
		if level < 1 || level > 100 {
			return battery.Battery{}, fmt.Errorf("%w: a level of %d", errNoReading, level)
		}
		return battery.Battery{Level: level, HasLevel: true, State: battery.Discharging, Kind: battery.KindKeyboard}, nil
	default:
		return battery.Battery{}, fmt.Errorf("%w: a power state of %#02x", errNoReading, state)
	}
}
