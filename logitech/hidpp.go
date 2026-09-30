package logitech

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/ushineko/sanshoku/hidraw"
)

// The two HID++ report kinds. A request is sent as a short report; the reply
// may come back as either, which is the trap in request below.
const (
	reportShort = 0x10
	reportLong  = 0x11
)

// replySize is the buffer a reply is read into. A long report is twenty bytes;
// hidraw hands over one whole report per read and discards what does not fit,
// so this is room to spare rather than a limit on what is recognised.
const replySize = 64

/*
softwareID marks a reply as an answer to *this process's* request.

Devices echo it back in the low nibble of the function byte, and HID++ reserves
four bits of every request for it so that concurrent clients can tell their
answers apart. hayami used the constant 0x08, which separated it from solaar
and did nothing at all between two hayamis -- and there are routinely several:
the panel polls the receiver every fifteen seconds while another process asks
the same node. Two requests agreeing on device index, feature index and
function would then accept each other's replies.

That is not theoretical. A phantom peripheral called "Q" appeared beside a real
mouse on hayami's panel, at the same percentage: 81 is `chr('Q')`, so a battery
level had been decoded as a device name (hayami issue #58).

Taken from the process ID because it has to differ between processes and
nothing else about it matters.

**Solaar's own ID is excluded.** It uses 0x0B -- `SOLAAR_SOFTWARE_ID` in
logitech_receiver/base.py -- and a value picked freely from 1..15 lands on it
one run in fifteen, at which point this process and solaar accept each other's
replies. The live test caught it in hayami: 71 % read where solaar read 79 % in
the same second, which is a reply belonging to somebody else rather than a
battery moving.

Two processes of this module can still collide -- one chance in fourteen -- so
this narrows the window rather than closing it, and the whole-name check in
name is the other half.
*/
var softwareID = softwareIDs[os.Getpid()%len(softwareIDs)]

// softwareIDs are the values this process will use: every ID a request may
// carry except zero, which marks a request as nobody's, and solaar's.
var softwareIDs = []byte{
	0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A,
	// 0x0B is solaar's.
	0x0C, 0x0D, 0x0E, 0x0F,
}

// rootFeature is feature 0x0000, the one every HID++ 2.0 device has at index
// zero. Its function 0 maps a feature ID to that device's index for it.
const rootFeature = 0x00

// The battery features, newest first.
//
// 0x1004 is what a device with a fuel gauge reports. 0x1000 is the older one,
// still all an early wireless mouse has, and reading it costs one extra
// lookup on a device that has neither.
const (
	featureUnifiedBattery = 0x1004
	featureBatteryStatus  = 0x1000
)

// errUnknownFeature is a device answering that it has never heard of the
// feature asked about. Not a failure: it is how a device says it has no fuel
// gauge, and the caller tries the older feature next.
var errUnknownFeature = errors.New("the device does not have that feature")

// errNoDevice is an index with nothing paired to it. The receiver answers for
// it -- immediately -- so this ends a discovery rather than delaying one.
var errNoDevice = errors.New("no device at that index")

// errSilent is a device that did not answer within the deadline.
//
// It is a sentinel and not a plain timeout because it is the ordinary case: a
// wireless mouse that has gone to sleep says nothing at all, every poll, for
// as long as nobody touches it. Reported as a failure it would put a line in a
// consumer's log every poll for a mouse that is merely idle, and the real
// failures would be lost among them. The reading is withheld; the caller does
// not hear about it.
var errSilent = errors.New("the device did not answer")

// errNotReachable is an index that is paired and not answering: asleep,
// switched off, or a slot left behind by hardware that has gone. The protocol
// gives nothing to tell those apart, so neither does this.
var errNotReachable = errors.New("a paired device is not reachable")

// errOldProtocol is a device that answered and does not speak HID++ 2.0.
//
// A keyboard from before that generation refuses the root request outright
// rather than reporting no features, which is a different thing and reads as a
// different sentence to a person. Its battery is a 1.0 register (hidpp10.go).
var errOldProtocol = errors.New("the device speaks HID++ 1.0")

// hidppError is the device refusing a request, with the code it refused with.
type hidppError struct {
	code byte
}

func (e hidppError) Error() string { return fmt.Sprintf("hid++ error 0x%02x", e.code) }

/*
HID++ 1.0 error codes.

**A different code space from 2.0's, and they overlap.** Both arrive through
the same reply, distinguished only by where in it they sit -- sub-id 0x8F for
1.0, feature index 0xFF for 2.0 -- and 0x01 means "unsupported feature" in one
and "invalid sub-id" in the other. Reading them with one table made a
ten-year-old keyboard that does not speak 2.0 at all look like a device with no
fuel gauge, and a receiver slot with nothing behind it look like a connection
that had failed (hayami issue #66).
*/
const (
	err10InvalidSubID   = 0x01
	err10InvalidAddress = 0x02
	err10ConnectFail    = 0x04
	err10Busy           = 0x07
	err10UnknownDevice  = 0x08
	err10ResourceError  = 0x09
)

// HID++ 2.0 error codes.
const err20UnsupportedFeature = 0x01

// The sub-ids that mark an error reply: 0x8F is the 1.0 form, and 0xFF sits
// where a 2.0 reply carries its feature index.
const (
	errorSub10 = 0x8F
	errorSub20 = 0xFF
)

// requestAttempts is how many times a request is sent before the device is
// taken to be silent.
//
// **Five, because one is not nearly enough.** Measured by hayami on a Lightspeed
// receiver with a G502 X PLUS, and the two measurements say different things:
//
//   - A mouse in continuous use answers a single request fourteen times in
//     twenty; a second attempt carries it to twenty.
//   - A mouse left alone for six seconds takes up to *four* attempts. The first
//     requests wake it and are lost, which is the ordinary state of a wireless
//     peripheral and not a fault.
//
// None of this is visible from the protocol, and every test that drives a fake
// transport passes at one attempt. At one attempt a consumer would show a
// mouse flapping between a reading and "not answering" on most polls, on a
// desk where nothing was wrong.
//
// Only silence is retried, so this costs nothing on an index that is empty:
// the receiver refuses those, and a refusal is an answer.
const requestAttempts = 5

// request sends one HID++ 2.0 request and returns the reply's parameters.
//
// Three things about the reply are not obvious and each of them cost a probe
// to find out:
//
//   - **A short request may be answered with a long report.** The 0x1004 reply
//     on a Lightspeed receiver comes back as 0x11. Matching on the report ID
//     drops the answer that was asked for, and the only symptom is a timeout,
//     which reads like absent hardware.
//   - **There are two error forms.** HID++ 2.0 answers with feature index
//     0xFF; HID++ 1.0 answers with sub-id 0x8F, and the receiver uses the 1.0
//     form to say that an index is empty. A reader that knows only the 2.0
//     form waits out its timeout on every unpaired index -- six of them, and a
//     discovery that should take milliseconds takes seconds.
//   - **Other traffic shares the node.** The mouse's own notifications arrive
//     on it unasked, and solaar's replies too, so a reply has to be recognised
//     rather than merely received.
func request(ctx context.Context, rd hidraw.ReportDevice, timeout time.Duration, device, feature, function byte, params ...byte) ([]byte, error) {
	req := make([]byte, 7)
	req[0] = reportShort
	req[1] = device
	req[2] = feature
	req[3] = function<<4 | softwareID
	copy(req[4:], params)

	matches := func(r []byte) bool {
		_, matched, _ := reply(r, device, feature, req[3])
		return matched
	}
	var err error
	for range requestAttempts {
		var r []byte
		r, err = attempt(ctx, rd, timeout, req, matches)
		if errors.Is(err, errSilent) {
			// Silence is the one answer worth asking again for. An error reply
			// is an answer, and asking an empty index twice would double the
			// cost of a discovery to learn nothing.
			continue
		}
		if err != nil {
			return nil, err
		}
		out, _, err := reply(r, device, feature, req[3])
		return out, err
	}
	return nil, fmt.Errorf("device %d: %w after %d attempts", device, err, requestAttempts)
}

/*
attempt sends one request and waits once, for at most timeout, for its reply.

The per-attempt bound is this driver's (timeout, 300 ms by default, hayami's
measurement), and running out of it is hidraw.ErrSilent, which becomes
errSilent here. The caller's own context expiring or being cancelled is not
silence: it is the caller giving up, and is returned as the caller's error so
that no retry and no swallowing happens on its behalf. That is why the parent
context is checked before the error is read.
*/
func attempt(ctx context.Context, rd hidraw.ReportDevice, timeout time.Duration, req []byte, matches func([]byte) bool) ([]byte, error) {
	actx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	r, err := hidraw.Exchange(actx, rd, req, matches, replySize)
	if err == nil {
		return r, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, fmt.Errorf("hid++ request: %w", ctxErr)
	}
	if errors.Is(err, hidraw.ErrSilent) {
		return nil, errSilent
	}
	return nil, err
}

// reply reads one report, saying whether it answers the request at all.
//
// The middle return is what makes this readable: a report that is not an
// answer is neither a result nor an error, it is somebody else's traffic, and
// the exchange goes back to waiting.
func reply(r []byte, device, feature, function byte) (params []byte, matched bool, err error) {
	if len(r) < 4 || r[1] != device {
		return nil, false, nil
	}
	if r[0] != reportShort && r[0] != reportLong {
		return nil, false, nil
	}

	// HID++ 1.0 error: sub-id 0x8F, then the sub-id and address that failed
	// and the code. The receiver answers this way for an empty index.
	if r[2] == errorSub10 {
		if len(r) < 6 {
			return nil, false, nil
		}
		return nil, true, translate10(r[5])
	}

	// HID++ 2.0 error: feature index 0xFF, the failing function, then the code.
	if r[2] == errorSub20 {
		if len(r) < 5 {
			return nil, false, nil
		}
		return nil, true, translate20(r[4])
	}

	if r[2] != feature || r[3] != function {
		return nil, false, nil
	}
	return r[4:], true, nil
}

// translate10 names the HID++ 1.0 error codes this package treats as answers
// rather than as failures.
//
// The three that are not failures say three different things, and a panel that
// ran them together said "no Logitech receiver" about a receiver with a
// keyboard on it:
//
//   - an index with nothing paired to it at all
//   - an index paired to something that is not answering -- asleep, switched
//     off, or a slot left behind by a device that has since gone. A pairing
//     table outlives the hardware in it and a receiver can carry somebody
//     else's, so a name here is not evidence a device exists.
//   - a device that answered and does not speak HID++ 2.0, which is what a
//     keyboard older than that generation does
func translate10(code byte) error {
	switch code {
	case err10UnknownDevice:
		return errNoDevice
	case err10ConnectFail, err10ResourceError, err10Busy:
		return errNotReachable
	case err10InvalidSubID:
		return errOldProtocol
	case err10InvalidAddress:
		// A register this device does not have, which is the 1.0 way of
		// saying what errUnknownFeature says in 2.0.
		return errUnknownFeature
	default:
		return hidppError{code: code}
	}
}

// translate20 names the HID++ 2.0 error codes.
func translate20(code byte) error {
	if code == err20UnsupportedFeature {
		return errUnknownFeature
	}
	return hidppError{code: code}
}

// featureIndex asks a device which of its own indices holds a feature.
//
// Index zero is the answer for a feature the device does not have, which is
// the 2.0 way of saying no and is not an error report.
func featureIndex(ctx context.Context, rd hidraw.ReportDevice, timeout time.Duration, device byte, feature uint16) (byte, error) {
	params, err := request(ctx, rd, timeout, device, rootFeature, 0x00, byte(feature>>8&0xFF), byte(feature&0xFF), 0x00)
	if err != nil {
		return 0, err
	}
	if len(params) < 1 || params[0] == 0 {
		return 0, errUnknownFeature
	}
	return params[0], nil
}
