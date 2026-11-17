package nzxt

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/ushineko/sanshoku"
)

/*
fake is a Kraken's control channel that exists only in memory, ported from
hotaru's cooler.Fake and carrying only the behaviours hotaru measured:

  - Chatter: it streams `75 02` status reports nobody asked for, ahead of every
    reply. Eleven of twelve reports after one request on hotaru's machine were
    broadcasts, and a fake that answered politely would let the bug this driver
    was written around straight back in.
  - Queued: a backlog of broadcasts already waiting, as after a handle sits
    idle; Drain clears it.
  - Silent: every question goes unanswered, as a wedged device or the wrong
    node does.
  - Faulty: the temperature bytes read `FF FF`, liquidctl#172.
  - LoseFirst: this many replies go to another reader, as OpenRGB takes them.
  - Told: every report written, in order.
  - Accessories: each lighting channel's six accessory slots, as `21 03`
    reports them. newFake has this desk's: the Kraken Elite ring (`1E`) on
    channel 1 and an F360 Core (`1D`) on channel 2.

A lighting command (`26 xx`) is acknowledged as the cooler does, with `FF 01`
and the command at bytes 14 and 15 (spec 024). Every other command is answered
with its prefix incremented and a result byte of 0x01, which is how the device
answers a command it accepted. A read with
nothing to read reports the deadline at once rather than waiting it out.
*/
type fake struct {
	mu sync.Mutex

	Coolant  float64
	PumpRPM  int
	PumpDuty int
	FanRPM   int
	FanDuty  int

	Chatter   int
	Queued    int
	Silent    bool
	Faulty    bool
	LoseFirst int
	Told      [][]byte

	Accessories [channels][slotsPer]byte

	// buffered is the channels whose last `26` command was a `26 04`, for
	// the guard on `26 06`.
	buffered map[byte]bool

	/*
		Memory makes the fake keep its screen slots: a reserve (`32 01`)
		records the slot's address and size, a delete (`32 02`) empties it,
		and a question about a slot (`30 04`) is answered from them, as the
		cooler does. Off, every slot reads vacant. Spec 021.
	*/
	Memory bool
	memory map[byte][2]int

	pending [][]byte
	closed  bool
}

// newFake is a cooler reporting plausible numbers, chattering as the real one
// does.
func newFake() *fake {
	return &fake{
		Coolant: 37.5, PumpRPM: 2608, PumpDuty: 81, FanRPM: 1190, FanDuty: 51, Chatter: 3,
		Accessories: [channels][slotsPer]byte{{0x1E}, {0x1D}},
	}
}

func (f *fake) Write(report []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return errors.New("closed")
	}
	f.Told = append(f.Told, append([]byte(nil), report...))
	f.guard(report)
	for range f.Chatter {
		f.pending = append(f.pending, f.broadcast())
	}
	if f.LoseFirst > 0 {
		f.LoseFirst--
		return nil
	}
	if f.Silent {
		return nil
	}
	reply := make([]byte, reportLen)
	reply[0], reply[1] = report[0]+1, report[1]
	switch {
	case report[0] == 0x26, report[0] == 0x2A:
		reply[0], reply[1], reply[14], reply[15] = 0xFF, 0x01, report[0], report[1]
	case reply[0] == 0x75:
		f.fill(reply)
	case report[0] == 0x20 && report[1] == 0x03:
		for i, slots := range f.Accessories {
			copy(reply[slotsAt+slotsPer*i:], slots[:])
		}
	default:
		reply[resultByte] = resultOK
	}
	if f.Memory {
		f.remember(report, reply)
	}
	f.pending = append(f.pending, reply)
	return nil
}

func (f *fake) Read(ctx context.Context, buf []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if f.Queued > 0 {
		f.Queued--
		return copy(buf, f.broadcast()), nil
	}
	if len(f.pending) == 0 {
		return 0, fmt.Errorf("no report from the fake: %w", context.DeadlineExceeded)
	}
	next := f.pending[0]
	f.pending = f.pending[1:]
	return copy(buf, next), nil
}

// remember keeps the slot map for Memory, and answers a question about a slot
// from it.
func (f *fake) remember(report, reply []byte) {
	if f.memory == nil {
		f.memory = map[byte][2]int{}
	}
	switch {
	case report[0] == 0x32 && report[1] == 0x01:
		f.memory[report[2]] = [2]int{int(report[4]) | int(report[5])<<8, int(report[6]) | int(report[7])<<8}
	case report[0] == 0x32 && report[1] == 0x02:
		delete(f.memory, report[2])
	case report[0] == 0x30 && report[1] == 0x04:
		held, ok := f.memory[report[2]]
		if !ok {
			return
		}
		reply[14], reply[15], reply[16] = report[2], report[2]+1, 0x02
		reply[17], reply[18] = low(held[0]), low(held[0]>>8)
		reply[19], reply[20] = low(held[1]), low(held[1]>>8)
		reply[21], reply[22] = 0x01, 0x01
	}
}

// Drain clears what was waiting before the question, as the real handle does.
func (f *fake) Drain(int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Queued = 0
	f.pending = nil
}

func (f *fake) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

// told is the commands written so far, each cut to its first n bytes.
func (f *fake) told(n int) [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]byte, len(f.Told))
	for i, r := range f.Told {
		out[i] = r[:n]
	}
	return out
}

// broadcast is the `75 02` report the device streams unasked.
func (f *fake) broadcast() []byte {
	r := make([]byte, reportLen)
	r[0], r[1] = 0x75, 0x02
	f.fill(r)
	return r
}

func (f *fake) fill(r []byte) {
	if f.Faulty {
		r[15], r[16] = 0xFF, 0xFF
		return
	}
	whole := int(f.Coolant)
	r[15] = low(whole)
	r[16] = low(int(f.Coolant*10) - whole*10)
	r[17], r[18] = low(f.PumpRPM), low(f.PumpRPM>>8)
	r[19] = low(f.PumpDuty)
	r[23], r[24] = low(f.FanRPM), low(f.FanRPM>>8)
	r[25] = low(f.FanDuty)
}

/*
misuse is every lighting command any fake in the package was sent outside
spec 024 R1.11 and spec 025 R1.7, which TestMain fails the run on: the `22`
family, a `2A` command other than `04`, a `26` command other than `14`, `04`
and `06`, and a
`26 06` whose channel's last `26` command was not a `26 04`.
*/
var misuse struct {
	sync.Mutex
	seen []string
}

func (f *fake) guard(report []byte) {
	bad := ""
	switch {
	case report[0] == 0x22:
		bad = "a command the driver never sends"
	case report[0] == 0x2A:
		if report[1] != 0x04 {
			bad = "a 2A command the driver never sends"
		}
	case report[0] != 0x26:
		return
	case report[1] == 0x04:
		if f.buffered == nil {
			f.buffered = map[byte]bool{}
		}
		f.buffered[report[2]] = true
		return
	case report[1] == 0x06:
		if !f.buffered[report[2]] {
			bad = "26 06 without a 26 04 before it"
		}
		delete(f.buffered, report[2])
	case report[1] == 0x14:
		return
	default:
		bad = "a 26 command the driver never sends"
	}
	if bad != "" {
		misuse.Lock()
		misuse.seen = append(misuse.seen, fmt.Sprintf("% x: %s", report[:4], bad))
		misuse.Unlock()
	}
}

// low is the bottom byte of a value, which is what a report field holds.
func low(v int) byte { return byte(v & 0xFF) }

// withFake is an open device over a fake, with the default freshness.
func withFake(f *fake) *device {
	return &device{id: sanshoku.Identity{Vendor: vendor, Product: 0x3012, Name: krakenNode.Name, Path: krakenNode.Path}, model: Known[0x3012], fresh: defaultFreshness, hid: f, showing: -1, fx: firmwareState{running: -1}}
}
