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

Every other command is answered with its prefix incremented and a result byte
of 0x01, which is how the device answers a command it accepted. A read with
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

	pending [][]byte
	closed  bool
}

// newFake is a cooler reporting plausible numbers, chattering as the real one
// does.
func newFake() *fake {
	return &fake{Coolant: 37.5, PumpRPM: 2608, PumpDuty: 81, FanRPM: 1190, FanDuty: 51, Chatter: 3}
}

func (f *fake) Write(report []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return errors.New("closed")
	}
	f.Told = append(f.Told, append([]byte(nil), report...))
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
	if reply[0] == 0x75 {
		f.fill(reply)
	} else {
		reply[resultByte] = resultOK
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

// low is the bottom byte of a value, which is what a report field holds.
func low(v int) byte { return byte(v & 0xFF) }

// withFake is an open device over a fake, with the default freshness.
func withFake(f *fake) *device {
	return &device{id: sanshoku.Identity{Vendor: vendor, Product: 0x3012, Name: krakenNode.Name, Path: krakenNode.Path}, model: Known[0x3012], fresh: defaultFreshness, hid: f, showing: -1}
}
