package razer

import (
	"errors"
	"fmt"
)

// The Razer report protocol, which is 90 bytes with a checksum.
const (
	// reportSize is the report, without the leading report number.
	reportSize = 90

	// crcAt is where the checksum sits in the body: the second-to-last byte,
	// ahead of one reserved byte.
	crcAt = reportSize - 2

	// argsAt is where the arguments start in the body, after the status,
	// transaction, remaining packets (two bytes), protocol, size, class and
	// command.
	argsAt = 8

	// The classes and commands. 0x07 is power. The values are OpenRazer's,
	// read from its driver source; only getters are sent.
	classPower          = 0x07
	commandBatteryLevel = 0x80
	commandCharging     = 0x84

	// argsSize is the argument count both getters are sent with and answer
	// in: a reserved byte and the value.
	argsSize = 0x02
)

// Razer's status byte, which is the difference between "no reading" and
// "broken".
const (
	statusBusy         = 0x01
	statusOK           = 0x02
	statusFail         = 0x03
	statusTimeout      = 0x04
	statusNotSupported = 0x05
)

/*
errSilent is a reply that carries no reading and no fault.

Busy, timeout and not-supported are all ordinary on the hardware hayami
measured. The mouse sleeps on a five-minute idle timer and the relay then says
timeout; the openrazer daemon polls the same node and collides, which is busy;
and a dock with nothing paired to it answers that it does not support the
question. Reported as a failure, any of them would be a fault most of the day.
*/
var errSilent = errors.New("a Razer device answered with no reading")

/*
Decode reads a reply to one of the two battery getters, with its leading
report number: class 0x07 command 0x80, the level, or class 0x07 command
0x84, the charge state.

For the level command, level is a percentage and charging is false. For the
charging command, charging is the device's answer and level is zero. Which of
the two a reply answers is read from the reply itself, which echoes the class
and command it was sent.

The status is checked first. Busy, timeout and not-supported are an error the
driver treats as silence; fail and any unknown status are an error. The
checksum is then verified: this is a radio link with a dock in the middle, and
a corrupted battery level is a number a panel would draw without hesitating.
*/
func Decode(reply []byte) (level int, charging bool, err error) {
	if len(reply) != reportSize+1 {
		return 0, false, fmt.Errorf("a Razer reply of %d bytes", len(reply))
	}
	body := reply[1:] // drop the report number
	class, command := body[6], body[7]

	switch body[0] {
	case statusOK:
	case statusBusy, statusTimeout, statusNotSupported:
		// Asleep, contended, or a dock with nothing paired to it. No reading,
		// and nothing to report.
		return 0, false, fmt.Errorf("%#02x/%#02x, status %#02x: %w", class, command, body[0], errSilent)
	case statusFail:
		return 0, false, fmt.Errorf("a Razer device refused %#02x/%#02x", class, command)
	default:
		return 0, false, fmt.Errorf("a Razer device answered status %#02x", body[0])
	}

	if body[crcAt] != crc(body) {
		return 0, false, errors.New("a Razer reply's checksum does not match")
	}

	value := body[argsAt+1]
	switch {
	case class == classPower && command == commandBatteryLevel:
		// The level is a byte over full scale, not a percentage: 0xFF is a
		// full battery. Rounded rather than truncated, so 0xFF is 100 and not
		// 99.
		return (int(value)*100 + 127) / 255, false, nil
	case class == classPower && command == commandCharging:
		return 0, value != 0, nil
	default:
		return 0, false, fmt.Errorf("a Razer reply to %#02x/%#02x, which is not a battery getter", class, command)
	}
}

// request builds a getter, with its leading report number.
func request(transaction, class, command byte) []byte {
	out := make([]byte, reportSize+1)
	body := out[1:]
	body[1] = transaction
	body[5] = argsSize
	body[6] = class
	body[7] = command
	body[crcAt] = crc(body)
	return out
}

// crc is an XOR over the report's addressed and argument bytes, which is
// everything but the status, the transaction ID and the two trailing bytes.
func crc(body []byte) byte {
	var sum byte
	for _, b := range body[2:crcAt] {
		sum ^= b
	}
	return sum
}
