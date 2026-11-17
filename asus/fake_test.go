package asus

import (
	"context"
	"fmt"
	"sync"

	"github.com/ushineko/sanshoku/hidraw"
)

/*
fake is an Aura USB controller in memory. It answers the version (82) and
configuration (B0) questions, as the Z790 Hero did on the bench, and takes
every other report without answering, as the controller does.
*/
type fake struct {
	mu      sync.Mutex
	Table   [60]byte
	Silent  bool
	Told    [][]byte
	pending [][]byte
	closed  bool
}

// heroTable is the Z790 Hero's configuration table as the bench read it.
var heroTable = [60]byte{
	0x10, 0x1f, 0x04, 0x01, 0x83, 0x08, 0x0b, 0x3c, 0x00, 0x01, 0x00, 0x00,
	0x78, 0x3c, 0x00, 0x01, 0x00, 0x00, 0x78, 0x3c, 0x00, 0x01, 0x00, 0x00,
	0x78, 0x3c, 0x00, 0x01, 0x0d, 0x03, 0x01, 0xf4,
}

func newFake() *fake { return &fake{Table: heroTable} }

func (f *fake) Write(r []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(r) != 1+reportLen || r[0] != reportID {
		return fmt.Errorf("a report of %d bytes with id %02x", len(r), r[0])
	}
	f.Told = append(f.Told, append([]byte(nil), r[1:]...))
	if f.Silent {
		return nil
	}
	reply := make([]byte, 1+reportLen)
	reply[0] = reportID
	switch r[1] {
	case askVersion:
		reply[1] = gotVersion
		copy(reply[2:], "AULA3-6K75-0309")
	case askConfig:
		reply[1] = gotConfig
		copy(reply[4:], f.Table[:])
	default:
		return nil
	}
	f.pending = append(f.pending, reply)
	return nil
}

func (f *fake) Read(_ context.Context, buf []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pending) == 0 {
		return 0, fmt.Errorf("no report: %w", context.DeadlineExceeded)
	}
	n := copy(buf, f.pending[0])
	f.pending = f.pending[1:]
	return n, nil
}

func (f *fake) Drain(int) {}

func (f *fake) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

// told is every report written after the report ID, as its first n bytes.
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

var heroNode = hidraw.Node{Vendor: vendor, Product: 0x18F3, Name: "AsusTek Computer Inc. AURA LED Controller", Path: "/dev/hidraw-test5"}

// opened is the Hero's controller opened over f, as Open does.
func opened(f *fake) (*device, error) {
	c := candidate(heroNode, func(string) (node, error) { return f, nil }, "ASUS ROG MAXIMUS Z790 HERO")
	dev, err := c.Open(context.Background())
	if err != nil {
		return nil, err
	}
	return dev.(*device), nil
}
