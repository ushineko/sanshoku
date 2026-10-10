package sony

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
const driverName = "sony"

// vendor is Sony Interactive Entertainment's USB vendor ID, which the
// controller also carries over Bluetooth.
const vendor = 0x054C

// genericDesktop is the HID usage page the controller's gamepad report is on.
const genericDesktop = 0x01

/*
products is the allow-list: the controllers this driver will read, and what
each is called.

By product, because Sony's vendor ID is on headphones too, and this driver
sends a request. The names are the controllers' own HID names without the
vendor's, so a reading is called the same over either link.
*/
var products = map[uint16]string{
	0x0CE6: "DualSense Wireless Controller",
	0x0DF2: "DualSense Edge Wireless Controller",
}

// reportLen is the buffer an input report is read into: the longest, 0x31 over
// Bluetooth, is 78 bytes, and Windows pads report 0x01 to that.
const reportLen = 128

/*
defaultTimeout is how long one reading may take.

A DualSense in its long-report mode sends a report every few milliseconds, so
one arrives at once: measured on Windows over Bluetooth, the reports read
straight after the feature read were already 0x31.
*/
const defaultTimeout = 500 * time.Millisecond

/*
Driver reads a DualSense's battery from its input report.

Over Bluetooth it first reads feature report 0x05, once per open handle, which
is what switches the controller to the report that carries the battery. That
is a read and changes no setting, but it does change the report the controller
sends until it reconnects: a program reading it as a generic gamepad through
the short report, as a DirectInput game without Steam does on Windows, stops
seeing its input until then. Steam and the Linux kernel make the same request
on connection, so on Linux and wherever Steam runs it changes nothing.

The zero value is ready to use. Timeout bounds one reading; zero means 500 ms.
*/
type Driver struct {
	Timeout time.Duration
}

// Name is "sony".
func (Driver) Name() string { return driverName }

/*
Find returns one candidate per Sony interface that declares input report 0x01
on the Generic Desktop page, the controller's own. ErrAbsent when there is
none. A product not on the allow-list is a candidate, so it can be seen, and
opening it is refused without its node being opened. Find opens nothing.
*/
func (d Driver) Find(context.Context) ([]sanshoku.Candidate, error) {
	nodes, err := hidraw.Nodes(vendor, hidraw.HasReportID(genericDesktop, usbReport))
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

// node is an open controller interface: a *hidraw.Handle, or a test's
// stand-in.
type node interface {
	Read(ctx context.Context, buf []byte) (int, error)
	GetFeature(ctx context.Context, report []byte) error
	Drain(depth int)
	io.Closer
}

// openNode opens a real controller interface.
func openNode(path string) (node, error) {
	return hidraw.Open(path)
}

// candidate is one node as a candidate, opening through open, and refusing a
// product the allow-list does not name before anything is opened.
func candidate(n hidraw.Node, timeout time.Duration, open func(string) (node, error)) sanshoku.Candidate {
	bus := sanshoku.BusUSB
	if n.Bus == hidraw.BusBluetooth {
		bus = sanshoku.BusBluetooth
	}
	id := sanshoku.Identity{
		Vendor: n.Vendor, Product: n.Product, Bus: bus,
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
				return nil, fmt.Errorf("%s: not a product this driver reads: %w", id, sanshoku.ErrUnsupported)
			}
			nd, err := open(n.Path)
			if err != nil {
				return nil, err
			}
			return &device{id: id, timeout: timeout, rd: nd}, nil
		},
	}
}

// device is one open controller of an allow-listed product.
type device struct {
	id      sanshoku.Identity
	timeout time.Duration

	mu sync.Mutex
	// rd is the open node. Nil once closed.
	rd node
	// switched is whether the feature read that turns on the long report has
	// been made on this handle. Bluetooth only.
	switched bool
}

// Identity is the controller's.
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
Batteries reads the battery from the next long report.

A controller that sends no long report in the time given is no reading and no
error, as every driver reports a device that is there and silent. Over
Bluetooth the switch is then made again on the next reading, in case the
controller reconnected under the same handle and came back in its short mode.
*/
func (d *device) Batteries(ctx context.Context) ([]battery.Battery, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rd == nil {
		return nil, fmt.Errorf("reading %s: %w", d.id.Path, os.ErrClosed)
	}
	rctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	bt := d.id.Bus == sanshoku.BusBluetooth
	if bt && !d.switched {
		f := make([]byte, calibrationLen)
		f[0] = calibration
		if err := d.rd.GetFeature(rctx, f); err != nil {
			return nil, fmt.Errorf("reading %s: asking for the long report: %w", d.id, err)
		}
		d.switched = true
	}

	d.rd.Drain(hidraw.QueueDepth)
	buf := make([]byte, reportLen)
	for {
		n, err := d.rd.Read(rctx, buf)
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			d.switched = false
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", d.id, err)
		}
		if !isLong(buf[:n], bt) {
			continue
		}
		b, err := Decode(buf[:n], bt)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", d.id, err)
		}
		b.Name = d.id.Name
		return []battery.Battery{b}, nil
	}
}
