package aula

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/hidraw"
	"github.com/ushineko/sanshoku/lighting"
)

/*
The F75 on its cable: a Sino Wealth keyboard chip, 258a:010c, that calls
itself "BY Tech Gaming Keyboard". It is another device from the receiver, on
another vendor id, and it has a canvas and no battery reading.

Its vendor collection (usage page 0xFF00) carries two feature reports. Report
6, 520 bytes, is every lighting and settings command. Report 5 is the chip's
bootloader channel, and this driver never sends it.
*/
const (
	wiredVendor  = 0x258A
	wiredProduct = 0x010C
	wiredPage    = 0xFF00
)

// wiredSettle is the pause between asking the wired keyboard a question and
// reading its answer, as womier-l65-linux leaves and as was measured to work.
const wiredSettle = 50 * time.Millisecond

// wiredNode is an open wired keyboard: a *hidraw.Handle, or a test's
// stand-in.
type wiredNode interface {
	SetFeature(ctx context.Context, report []byte) error
	GetFeature(ctx context.Context, report []byte) error
	io.Closer
}

// openWired opens a real wired keyboard.
func openWired(path string) (wiredNode, error) {
	return hidraw.Open(path)
}

// isWired is the predicate for the wired keyboard's vendor collection: the
// one product, declaring report 6 on the vendor page.
func isWired(n hidraw.Node) bool {
	return n.Product == wiredProduct && hidraw.HasReportID(wiredPage, wiredReport)(n)
}

// wiredCandidate is one wired keyboard as a candidate, opening through open.
func wiredCandidate(n hidraw.Node, open func(string) (wiredNode, error)) sanshoku.Candidate {
	id := sanshoku.Identity{
		Vendor: n.Vendor, Product: n.Product, Bus: sanshoku.BusUSB,
		Name: wiredName, Phys: n.Phys, Path: n.Path,
	}
	return sanshoku.Candidate{
		Identity: id,
		Driver:   driverName,
		Key:      n.Key,
		Open: func(context.Context) (sanshoku.Device, error) {
			nd, err := open(n.Path)
			if err != nil {
				return nil, err
			}
			return &wired{id: id, rd: nd, settle: wiredSettle}, nil
		},
	}
}

// wiredName is what the keyboard on its cable is called. Its own name is its
// chip maker's.
const wiredName = "AULA F75"

// wired is one open F75 on its cable.
type wired struct {
	id     sanshoku.Identity
	settle time.Duration

	mu sync.Mutex
	// rd is the open node. Nil once closed.
	rd    wiredNode
	model model
	held  frame
}

// Identity is the keyboard's.
func (w *wired) Identity() sanshoku.Identity { return w.id }

// Close closes the node.
func (w *wired) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.rd == nil {
		return nil
	}
	err := w.rd.Close()
	w.rd = nil
	if err != nil {
		return fmt.Errorf("closing %s: %w", w.id, err)
	}
	return nil
}

// Keys is the F75's 80 lights in slot order.
func (w *wired) Keys() []lighting.Key { return f75KeyList() }

// Floor is the shortest interval at which frames are sent: 80 ms. One frame
// takes about 66 ms to write.
func (w *wired) Floor() time.Duration { return frameFloor }

// Hold is how long a frame lasts before the firmware's effect returns. It
// makes the F75 a lighting.Fading on its cable too.
func (w *wired) Hold() time.Duration { return frameHold }

/*
Frame draws px over what the keyboard is showing and sends the whole board,
every key its own colour.

One frame is one feature report and takes about 66 ms to write. The first
frame on a handle asks the keyboard its model, and a keyboard that is not an
F75 is lighting.ErrNoCanvas.
*/
func (w *wired) Frame(ctx context.Context, px []lighting.Pixel) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.rd == nil {
		return fmt.Errorf("lighting %s: %w", w.id.Path, os.ErrClosed)
	}
	next, err := merged(w.held, px)
	if err != nil {
		return fmt.Errorf("a frame to %s: %w", w.id, err)
	}
	if err := w.askModel(ctx); err != nil {
		return fmt.Errorf("a frame to %s: %w", w.id, err)
	}
	w.held = next
	if err := w.rd.SetFeature(ctx, wiredFrame(&w.held)); err != nil {
		return fmt.Errorf("a frame to %s: %w", w.id, err)
	}
	return nil
}

/*
askModel asks the wired keyboard what it is, once per handle, and refuses one
that is not an F75.

The question is the report's header with command 0x82 and index 1, and the
answer is read back as the same feature report: the header echoed, then six
bytes whose last is the model. It is how OpenRGB's draft for this chip tells
its keyboards apart.
*/
func (w *wired) askModel(ctx context.Context) error {
	if !w.model.asked {
		if err := w.rd.SetFeature(ctx, wiredRequest(wiredModelCommand, wiredModelIndex, wiredModelLen)); err != nil {
			return fmt.Errorf("asking its model: %w", err)
		}
		select {
		case <-time.After(w.settle):
		case <-ctx.Done():
			return fmt.Errorf("asking its model: %w", ctx.Err())
		}
		answer := make([]byte, wiredLen)
		answer[0] = wiredReport
		if err := w.rd.GetFeature(ctx, answer); err != nil {
			return fmt.Errorf("reading its model: %w", err)
		}
		if answer[1] != wiredModelCommand {
			return fmt.Errorf("reading its model: %w: % x", errNoAnswer, answer[:wiredHeader])
		}
		w.model = model{asked: true, id: answer[wiredHeader+wiredModelAt]}
	}
	return w.model.refuse()
}

// Release forgets the frame and sends nothing: the firmware takes its lights
// back by itself when frames stop.
func (w *wired) Release(context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.rd == nil {
		return fmt.Errorf("releasing %s: %w", w.id.Path, os.ErrClosed)
	}
	w.held = frame{}
	return nil
}
