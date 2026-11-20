package gigabyte

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"

	"github.com/ushineko/sanshoku/hidraw"
)

/*
fake is a Fusion 2 USB controller in memory. It answers the report question
(60) with the report a test sets, in the layout the Z790 AORUS MASTER X's
controllers returned (spec 039 E0), and takes every other command without
answering, as the controller does. Its chip ID is made up.
*/
type fake struct {
	mu      sync.Mutex
	Info    [reportLen]byte
	Silent  bool
	Told    [][]byte
	pending []byte
	closed  bool
}

/*
infoReport is a controller's report: its number, flags and chip ID, the
product string and the colour orders as controller 0 on the bench reported
them (GRB on both headers).
*/
func infoReport(controller, flags byte, chip uint32) [reportLen]byte {
	var r [reportLen]byte
	r[0] = reportID
	r[1] = 0x01
	r[infoController] = controller
	r[infoFlags] = flags
	copy(r[infoProduct:], "IT5701-GIGABYTE V7.0.18.1")
	binary.LittleEndian.PutUint32(r[infoHeader1:], 0x00010002) // GRB
	binary.LittleEndian.PutUint32(r[infoHeader2:], 0x00010002) // GRB
	binary.LittleEndian.PutUint32(r[infoChip:], chip)
	return r
}

func newFake(controller byte) *fake {
	return &fake{Info: infoReport(controller, 0, 0x5701ABCD)}
}

func (f *fake) SetFeature(_ context.Context, r []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(r) != reportLen || r[0] != reportID {
		return fmt.Errorf("a report of %d bytes with id %02x", len(r), r[0])
	}
	f.Told = append(f.Told, append([]byte(nil), r[1:]...))
	if r[1] == cmdInfo && !f.Silent {
		f.pending = append([]byte(nil), f.Info[:]...)
	}
	return nil
}

func (f *fake) GetFeature(_ context.Context, r []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pending == nil {
		return errors.New("no report")
	}
	copy(r, f.pending)
	f.pending = nil
	return nil
}

func (f *fake) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

// told is every command written after the report ID, as its first n bytes.
func (f *fake) told(n int) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.Told))
	for i, c := range f.Told {
		out[i] = fmt.Sprintf("% x", c[:n])
	}
	return out
}

// sent is every command written after the report ID, whole.
func (f *fake) sent() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]byte(nil), f.Told...)
}

func (f *fake) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Told = nil
}

var masterXNode = hidraw.Node{Vendor: vendor, Product: 0x5702, Name: "ITE Tech. Inc. ITE Device", Path: "/dev/hidraw-test3"}

const masterX = "Z790 AORUS MASTER X"

// opened is a controller of the Z790 AORUS MASTER X opened over f, as Open does.
func opened(f *fake) (*device, error) {
	c := candidate(masterXNode, func(string) (node, error) { return f, nil }, masterX)
	dev, err := c.Open(context.Background())
	if err != nil {
		return nil, err
	}
	return dev.(*device), nil
}
