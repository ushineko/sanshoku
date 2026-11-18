package corsair

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/ushineko/sanshoku"
)

/*
fake is an MM700's lighting interface in memory, answering as the bench
showed (spec 026): a reply is 00, the command, a status and a 32-bit value;
the mode property reads and sets; a frame is accepted in either mode.

  - Silent: this many questions go unanswered, as a freshly enumerated pad's.
  - Refuse: a command answered with this status instead of 00.
  - Product: what property 0x12 reads.
  - Held: the lighting endpoint is open. An open while it is is answered
    03, and a close ends it, as the pad did on 2026-11-17 (spec 036).
  - Busy: this many opens are answered 03 whatever Held says, as a pad
    another program keeps opening.
  - After: called with each command once it is answered, outside the lock.
*/
type fake struct {
	mu      sync.Mutex
	Mode    uint32
	Product uint32
	Silent  int
	Refuse  map[byte]byte
	Held    bool
	Busy    int
	After   func(cmd []byte)
	Told    [][]byte
	pending [][]byte
	closed  bool
}

func newFake() *fake { return &fake{Mode: modeHardware, Product: 0x1B9B} }

func (f *fake) Write(r []byte) error {
	f.mu.Lock()
	after := f.After
	cmd, err := f.write(r)
	f.mu.Unlock()
	if err == nil && after != nil {
		after(cmd)
	}
	return err
}

func (f *fake) write(r []byte) ([]byte, error) {
	if r[0] != 0 || len(r) != 1+reportLen {
		return nil, fmt.Errorf("a report of %d bytes starting %02x", len(r), r[0])
	}
	cmd := append([]byte(nil), r[1:]...)
	f.Told = append(f.Told, cmd)
	if f.Silent > 0 {
		f.Silent--
		return cmd, nil
	}
	reply := make([]byte, reportLen)
	reply[1] = cmd[1]
	if status, ok := f.Refuse[cmd[1]]; ok {
		reply[2] = status
		f.pending = append(f.pending, reply)
		return cmd, nil
	}
	switch {
	case cmd[1] == cmdOpen && (f.Held || f.Busy > 0):
		if f.Busy > 0 {
			f.Busy--
		}
		reply[2] = statusInUse
	case cmd[1] == cmdOpen:
		f.Held = true
	case cmd[1] == cmdClose:
		f.Held = false
	case cmd[1] == cmdGet && cmd[2] == propMode:
		binary.LittleEndian.PutUint32(reply[3:], f.Mode)
	case cmd[1] == cmdGet && cmd[2] == propProduct:
		binary.LittleEndian.PutUint32(reply[3:], f.Product)
	case cmd[1] == cmdSet && cmd[2] == propMode:
		f.Mode = uint32(cmd[4])
	}
	f.pending = append(f.pending, reply)
	return cmd, nil
}

func (f *fake) Read(ctx context.Context, buf []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err //nolint:wrapcheck // the caller's own context
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pending) == 0 {
		return 0, fmt.Errorf("no report: %w", context.DeadlineExceeded)
	}
	n := copy(buf, f.pending[0])
	f.pending = f.pending[1:]
	return n, nil
}

func (f *fake) Drain(int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pending = nil
}

func (f *fake) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

// told is every command written, as its first n bytes in hex.
func (f *fake) told(n int) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.Told))
	for i, c := range f.Told {
		out[i] = fmt.Sprintf("% x", c[:n])
	}
	return out
}

func (f *fake) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Told = nil
}

// opened is a pad opened over f, as Open does, with no wait between tries.
func opened(f *fake) (*device, error) {
	c := candidate(mm700Node, func(string) (node, error) { return f, nil }, 0)
	dev, err := c.Open(context.Background())
	if err != nil {
		return nil, err
	}
	return dev.(*device), nil
}

var _ sanshoku.Device = (*device)(nil)
