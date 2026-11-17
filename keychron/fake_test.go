package keychron

import (
	"context"
	"fmt"
	"sync"

	"github.com/ushineko/sanshoku/hidraw"
)

/*
fake is a K4 HE's raw interface in memory, answering as the bench showed
(spec 028): every command is echoed; VIA values read and set; the LED count
is a status byte and the count; features carry the Keychron RGB bit.
*/
type fake struct {
	mu         sync.Mutex
	Effect     uint8
	Brightness uint8
	Features   uint8
	LEDs       int
	Silent     bool
	Told       [][]byte
	pending    [][]byte
	closed     bool
}

func newFake() *fake { return &fake{Effect: 20, Brightness: 200, Features: 0x8F, LEDs: 100} }

func (f *fake) Write(r []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(r) != 1+reportLen || r[0] != 0 {
		return fmt.Errorf("a report of %d bytes with id %02x", len(r), r[0])
	}
	cmd := append([]byte(nil), r[1:]...)
	f.Told = append(f.Told, cmd)
	if f.Silent {
		return nil
	}
	reply := append([]byte(nil), cmd...)
	switch {
	case cmd[0] == cmdFeatures:
		reply[1], reply[2] = 0, f.Features
	case cmd[0] == cmdRGB && cmd[1] == rgbLEDCount:
		reply[2], reply[3] = byte(f.LEDs>>8), byte(f.LEDs)
	case cmd[0] == cmdGetValue && cmd[2] == valueEffect:
		reply[3] = f.Effect
	case cmd[0] == cmdGetValue && cmd[2] == valueBrightness:
		reply[3] = f.Brightness
	case cmd[0] == cmdSetValue && cmd[2] == valueEffect:
		f.Effect = cmd[3]
	case cmd[0] == cmdSetValue && cmd[2] == valueBrightness:
		f.Brightness = cmd[3]
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

var k4Node = hidraw.Node{Vendor: vendor, Product: 0x0E40, Name: "Keychron Keychron K4 HE", Path: "/dev/hidraw-test37"}

func opened(f *fake) (*device, error) {
	dev, err := candidate(k4Node, func(string) (node, error) { return f, nil }).Open(context.Background())
	if err != nil {
		return nil, err
	}
	return dev.(*device), nil
}
