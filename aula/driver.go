package aula

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/battery"
	"github.com/ushineko/sanshoku/hidraw"
)

// driverName is the driver's name, as Name returns it and the support table keys it.
const driverName = "aula"

// receiverVendor is the vendor the F75's 2.4 GHz receiver enumerates as. The
// keyboard on its cable is another vendor and another chip entirely (a Sino
// Wealth 258a:010c), which this driver neither matches nor speaks to.
const receiverVendor = 0x3554

// vendorWord is how AULA writes its own name at the front of a device's.
const vendorWord = "AULA"

// controlPage is the vendor page the receiver's battery report sits on.
const controlPage = 0xFF02

/*
products is the allow-list: what each receiver this driver will write to is
called.

Found by vendor and report, never by product; written to only by product. The
receiver calls itself "Compx 2.4G Wireless Receiver" after the maker of its
chip, which is not what anybody calls the keyboard, so the name comes from
here.
*/
var products = map[uint16]string{
	0xFA09: "AULA F75",
}

/*
defaultTimeout is how long one battery question may take.

The receiver answered in a few milliseconds with the link up (spec 013). A
keyboard that is asleep, or just switched from its cable, does not answer the
first question in time, and the second question waits longer (wakeTimeout).
*/
const defaultTimeout = 300 * time.Millisecond

/*
wakeTimeout is how long the second question may take.

An F75 that has sat idle takes about a second to answer at all. Measured by
hayami spec 035: with two 300 ms questions an idle keyboard read nothing three
times out of four, and once its link was awake it answered in 74-616 ms (#38).
A short question and then a long one keeps a keyboard that is awake as fast
as before.
*/
const wakeTimeout = 1500 * time.Millisecond

// retryWait is the pause before the question is asked a second time. Measured
// on the desk this was written on: the first question after the keyboard was
// switched to 2.4 GHz went unanswered, and the link was up moments later.
const retryWait = 200 * time.Millisecond

/*
Driver reads an AULA keyboard's battery through its 2.4 GHz receiver.

The zero value is ready to use. Timeout bounds one question; zero means
300 ms.
*/
type Driver struct {
	Timeout time.Duration
}

// Name is "aula".
func (Driver) Name() string { return driverName }

/*
Find returns one candidate per AULA receiver interface that declares report
0x13 on vendor page 0xFF02. ErrAbsent when there is none.

The receiver's other interface is the keyboard itself, and the same interface
carries a feature report on page 0xFF04 that nothing here sends to. A product
not on the allow-list is a candidate, so it can be seen, and opening it is
refused without its node being opened. Find opens nothing.
*/
func (d Driver) Find(context.Context) ([]sanshoku.Candidate, error) {
	nodes, err := hidraw.Nodes(receiverVendor, hidraw.HasReportID(controlPage, reportID))
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, sanshoku.ErrAbsent
	}
	timeout := d.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	out := make([]sanshoku.Candidate, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, candidate(n, timeout, openNode))
	}
	return out, nil
}

// node is an open receiver interface: a *hidraw.Handle, or a test's
// stand-in.
type node interface {
	hidraw.ReportDevice
	io.Closer
	Drain(depth int)
}

// openNode opens a real receiver interface.
func openNode(path string) (node, error) {
	return hidraw.Open(path)
}

// candidate is one node as a candidate, opening through open, and refusing a
// product the allow-list does not name before anything is opened.
func candidate(n hidraw.Node, timeout time.Duration, open func(string) (node, error)) sanshoku.Candidate {
	id := sanshoku.Identity{
		Vendor: n.Vendor, Product: n.Product, Bus: sanshoku.BusUSB,
		Name: n.Name, Phys: n.Phys, Path: n.Path,
	}
	if name, ok := products[n.Product]; ok {
		id.Name = name
	}
	return sanshoku.Candidate{
		Identity: id,
		Driver:   driverName,
		Open: func(context.Context) (sanshoku.Device, error) {
			if _, ok := products[n.Product]; !ok {
				return nil, fmt.Errorf("%s: not a product this driver speaks to: %w", id, sanshoku.ErrUnsupported)
			}
			nd, err := open(n.Path)
			if err != nil {
				return nil, err
			}
			return &device{id: id, timeout: timeout, wake: max(timeout, wakeTimeout), rd: nd, wait: retryWait}, nil
		},
	}
}

// device is one open receiver of an allow-listed product.
type device struct {
	id      sanshoku.Identity
	timeout time.Duration
	// wake bounds the second question: wakeTimeout, or the driver's timeout
	// when that is longer.
	wake time.Duration
	// wait is the pause before the second question: retryWait, or a test's.
	wait time.Duration

	mu sync.Mutex
	// rd is the open node. Nil once closed.
	rd node
}

// Identity is the receiver's, named after the keyboard.
func (d *device) Identity() sanshoku.Identity { return d.id }

// Close closes the node.
func (d *device) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rd == nil {
		return nil
	}
	err := d.rd.Close()
	d.rd = nil
	if err != nil {
		return fmt.Errorf("closing %s: %w", d.id, err)
	}
	return nil
}

/*
Batteries asks the receiver for the keyboard's battery, twice if the first
question goes unanswered.

A keyboard that answers neither is asleep, off, or on its cable with the
receiver on its own: no reading and no error, as every other driver reports a
device that is there and silent. A reply that is not a reading (a level out of
range, a state not seen) is no reading too, with the reason as the error.
*/
func (d *device) Batteries(ctx context.Context) ([]battery.Battery, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rd == nil {
		return nil, fmt.Errorf("reading %s: %w", d.id.Path, os.ErrClosed)
	}

	reply, err := d.ask(ctx, d.timeout)
	if errors.Is(err, hidraw.ErrSilent) && ctx.Err() == nil {
		select {
		case <-time.After(d.wait):
		case <-ctx.Done():
			return nil, fmt.Errorf("reading %s: %w", d.id, ctx.Err())
		}
		reply, err = d.ask(ctx, d.wake)
	}
	if errors.Is(err, hidraw.ErrSilent) && ctx.Err() == nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", d.id, err)
	}
	b, err := Decode(reply)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", d.id, err)
	}
	b.Name = battery.Product(vendorWord, d.id.Name)
	return []battery.Battery{b}, nil
}

// ask sends the question once and waits up to timeout for its answer, skipping
// the receiver's other frames.
func (d *device) ask(ctx context.Context, timeout time.Duration) ([]byte, error) {
	actx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	d.rd.Drain(hidraw.QueueDepth)
	return hidraw.Exchange(actx, d.rd, request(), answers, reportLen)
}
