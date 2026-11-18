package corsair

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/hidraw"
)

// driverName is the driver's name, as Name returns it and the support table keys it.
const driverName = "corsair"

// vendor is Corsair's USB vendor ID.
const vendor = 0x1B1C

// controlPage is the vendor page the lighting interface is on.
const controlPage = 0xFF42

/*
products is the allow-list: what each product this driver will write to is
called. Found by vendor and page; written to only by product.
*/
var products = map[uint16]string{
	0x1B9B: "Corsair MM700",
	0x1BC9: "Corsair MM700 3XL", // the MM700's entry in OpenRGB's V2 table (spec 033); expected
}

const (
	// reportLen is every report's length. There is no report ID; a write is
	// a 0 and then these bytes.
	reportLen = 64

	// wired is byte 0 of every command to a device on its cable.
	wired = 0x08

	cmdSet   = 0x01
	cmdGet   = 0x02
	cmdClose = 0x05
	cmdBlock = 0x06
	cmdOpen  = 0x0D

	propMode    = 0x03
	propProduct = 0x12

	modeHardware = 0x01
	modeSoftware = 0x02

	// lightingEndpoint is the endpoint a frame is written to. OpenRGB tries
	// 0x22 first; the MM700 refuses it (status 09) and opens 0x01.
	lightingEndpoint = 0x01

	// replyWait bounds one exchange. Replies came in 0.7 to 2 ms.
	replyWait = 200 * time.Millisecond

	// productTries and productGap are OpenRGB's: a freshly enumerated device
	// does not answer its first questions.
	productTries = 8
	productGap   = 120 * time.Millisecond
)

/*
Driver finds and opens Corsair V2 devices.

The zero value is ready to use.
*/
type Driver struct{}

// Name is "corsair".
func (Driver) Name() string { return driverName }

/*
Find returns one candidate per Corsair node on usage page 0xFF42. ErrAbsent
when there is none. A product not on the allow-list is a candidate, so it can
be seen, and opening it is refused without its node being opened. Find opens
nothing.
*/
func (Driver) Find(context.Context) ([]sanshoku.Candidate, error) {
	nodes, err := hidraw.Nodes(vendor, hidraw.UsagePage(controlPage))
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, sanshoku.ErrAbsent
	}
	out := make([]sanshoku.Candidate, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, candidate(n, openNode, productGap))
	}
	return out, nil
}

// node is an open interface: a *hidraw.Handle, or a test's stand-in.
type node interface {
	hidraw.ReportDevice
	io.Closer
	Drain(depth int)
}

func openNode(path string) (node, error) { return hidraw.Open(path) }

// candidate is one node as a candidate, refusing a product the allow-list
// does not name before anything is opened.
func candidate(n hidraw.Node, open func(string) (node, error), gap time.Duration) sanshoku.Candidate {
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
		Key:      n.Key,
		Open: func(ctx context.Context) (sanshoku.Device, error) {
			if _, ok := products[n.Product]; !ok {
				return nil, fmt.Errorf("%s: not a product this driver speaks to: %w", id, sanshoku.ErrUnsupported)
			}
			nd, err := open(n.Path)
			if err != nil {
				return nil, err
			}
			d := &device{id: id, rd: nd}
			if err := d.checkProduct(ctx, gap); err != nil {
				_ = nd.Close()
				return nil, err
			}
			return d, nil
		},
	}
}

// device is one open, allow-listed Corsair V2 device.
type device struct {
	id sanshoku.Identity

	mu sync.Mutex
	// rd is the open node. Nil once closed.
	rd node
	// held is the frame the pad shows, or was last sent; sent is whether it
	// has been since it changed.
	held [lights]colour
	sent bool
	// open is whether this handle opened the lighting endpoint and has not
	// closed it (spec 036).
	open bool
}

// Identity is the node's, named after the product.
func (d *device) Identity() sanshoku.Identity { return d.id }

/*
Close closes the node and sends nothing: the pad keeps its last frame. An
endpoint this handle left open is closed first, best effort, so the next to
open the pad is not refused (spec 036 R3); a frame closes its own, so that is
only after a frame that could not.
*/
func (d *device) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rd == nil {
		return nil
	}
	if d.open {
		_ = d.shut(context.Background())
	}
	err := d.rd.Close()
	d.rd = nil
	return err //nolint:wrapcheck // the handle names the node
}

/*
checkProduct asks the device its product, up to productTries times gap apart,
and refuses one that is not the product the node said it was. The caller has
not shared the device yet.
*/
func (d *device) checkProduct(ctx context.Context, gap time.Duration) error {
	var last error
	for try := range productTries {
		if try > 0 {
			select {
			case <-ctx.Done():
				return fmt.Errorf("%s: %w", d.id, ctx.Err())
			case <-time.After(gap):
			}
		}
		value, err := d.get(ctx, propProduct)
		if err != nil {
			last = err
			continue
		}
		if uint16(value) != d.id.Product { //nolint:gosec // a product id is 16 bits
			return fmt.Errorf("%s answered as product %04x: %w", d.id, value, sanshoku.ErrUnsupported)
		}
		return nil
	}
	return fmt.Errorf("%s did not say what it is: %w: %w", d.id, sanshoku.ErrAbsent, last)
}

// errStatus is a reply whose status is not 00.
var errStatus = errors.New("the device refused the command")

// refused is a reply's status, as an error that is errStatus.
type refused struct{ status byte }

func (e *refused) Error() string { return fmt.Sprintf("status %02x: %v", e.status, errStatus) }
func (e *refused) Unwrap() error { return errStatus }

/*
exchange writes one command and returns the reply to it: the report carrying
the command at byte 1. A status other than 00 at byte 2 is an error naming
it. The caller holds mu, or has not shared the device yet.
*/
func (d *device) exchange(ctx context.Context, data ...byte) ([]byte, error) {
	if d.rd == nil {
		return nil, fmt.Errorf("%s: %w", d.id.Path, os.ErrClosed)
	}
	req := make([]byte, 1+reportLen)
	copy(req[1:], data)
	ctx, cancel := context.WithTimeout(ctx, replyWait)
	defer cancel()
	d.rd.Drain(hidraw.QueueDepth)
	cmd := data[1]
	matches := func(r []byte) bool { return len(r) >= 3 && r[1] == cmd }
	reply, err := hidraw.Exchange(ctx, d.rd, req, matches, reportLen)
	if err != nil {
		return nil, fmt.Errorf("%s, command %02x: %w", d.id, cmd, err)
	}
	if reply[2] != 0 {
		return nil, fmt.Errorf("%s, command %02x: %w", d.id, cmd, &refused{status: reply[2]})
	}
	return reply, nil
}

// get reads a property's 32-bit value.
func (d *device) get(ctx context.Context, prop byte) (uint32, error) {
	reply, err := d.exchange(ctx, wired, cmdGet, prop)
	if err != nil {
		return 0, err
	}
	if len(reply) < 7 {
		return 0, fmt.Errorf("%s, property %02x: a short reply: %w", d.id, prop, errStatus)
	}
	return binary.LittleEndian.Uint32(reply[3:7]), nil
}

// set writes a property.
func (d *device) set(ctx context.Context, prop, value byte) error {
	_, err := d.exchange(ctx, wired, cmdSet, prop, 0x00, value)
	return err
}
