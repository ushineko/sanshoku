package nzxt

import (
	"errors"
	"fmt"

	"github.com/ushineko/sanshoku/cooling"
)

/*
The status exchange.

Asking is `74 01` and the answer is `75 01`. The device also streams `75 02`
reports unasked (eleven of them arrived in the twelve reads after one request,
on hotaru's development machine), so a reader that matches only the first byte
parses a broadcast and calls it a reply. It carries status too, so it even
looks right.
*/
const (
	askStatus   = 0x74
	askStatusB  = 0x01
	statusFault = 0xFF // both temperature bytes, on a firmware fault
)

// errFault is liquidctl#172: a firmware fault reports 0xFFFF rather than a
// temperature.
var errFault = errors.New("the cooler reported no temperature, which is a firmware fault")

/*
DecodeStatus reads a `75 01` status reply.

Offsets are into the report as hidraw delivers it, report number included,
matching liquidctl's own indices so the two can be compared directly, which is
how hotaru checked them: bytes 15 and 16 are the coolant temperature, whole
degrees and tenths; 17 and 18 the pump speed, little-endian; 19 the pump duty;
23 and 24 the fan speed, little-endian; 25 the fan duty.

`FF FF` at 15 and 16 is a firmware fault (liquidctl#172) and is an error: a
cooler claiming 255.5 degrees would raise an alarm about the wrong thing. So is
a report that is not a `75 01` or is too short to hold the fields. Taken is
left zero for the caller to set.
*/
func DecodeStatus(reply []byte) (cooling.Status, error) {
	if len(reply) < 26 {
		return cooling.Status{}, fmt.Errorf("a Kraken status reply of %d bytes", len(reply))
	}
	if reply[0] != askStatus+1 || reply[1] != askStatusB {
		return cooling.Status{}, fmt.Errorf("a %02x%02x report is not a status reply", reply[0], reply[1])
	}
	if reply[15] == statusFault && reply[16] == statusFault {
		return cooling.Status{}, errFault
	}
	return cooling.Status{
		Coolant:  float64(reply[15]) + float64(reply[16])/10,
		PumpRPM:  int(reply[18])<<8 | int(reply[17]),
		PumpDuty: int(reply[19]),
		FanRPM:   int(reply[24])<<8 | int(reply[23]),
		FanDuty:  int(reply[25]),
		HasPump:  true,
		HasFan:   true,
	}, nil
}
