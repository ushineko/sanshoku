package asus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/hidraw"
)

// driverName is the driver's name, as Name returns it and the support table keys it.
const driverName = "asus"

// vendor is ASUS's USB vendor ID.
const vendor = 0x0B05

// controlPage is the vendor page the controller's one collection is on.
const controlPage = 0xFF72

/*
products is the allow-list: the Aura USB mainboard controllers that share one
protocol, as OpenRGB's Aura mainboard detector lists them (spec 033). Found by
vendor and page; written to only by product. A name is the product's own
where the mainboard's DMI name is not it.
*/
var products = map[uint16]string{
	0x18F3: "",                      // AURA LED Controller, on the ROG Maximus Z790 Hero (tested)
	0x1939: "",                      // expected
	0x19AF: "",                      // expected; on Windows, its collection with usage A1
	0x1AA6: "",                      // expected
	0x1BED: "",                      // expected
	0x1B29: "ASUS ROG Strix LC III", // the pump of a liquid cooler, on the same controller; expected
}

// pickedUsage is the usage a product's control collection has where it has
// more than one on the page: OpenRGB matches 19af on usage A1 only.
var pickedUsage = map[uint16]uint16{0x19AF: 0xA1}

const (
	// reportID is the controller's only report, in both directions.
	reportID = 0xEC
	// reportLen is a report's bytes after the report ID.
	reportLen = 64

	askVersion = 0x82
	gotVersion = 0x02
	askConfig  = 0xB0
	gotConfig  = 0x30

	// replyWait bounds one question. The board answered in about 11 ms.
	replyWait = 500 * time.Millisecond
)

// dmiBoard is where Linux keeps the mainboard's name. A variable so that a
// test can give its own.
var dmiBoard = "/sys/class/dmi/id/board_name"

/*
Driver finds and opens ASUS Aura USB mainboard controllers.

The zero value is ready to use.
*/
type Driver struct{}

// Name is "asus".
func (Driver) Name() string { return driverName }

/*
Find returns one candidate per ASUS node on usage page 0xFF72. ErrAbsent when
there is none. A product not on the allow-list is a candidate, so it can be
seen, and opening it is refused without its node being opened. Find opens
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
		if !theCollection(n) {
			continue
		}
		out = append(out, candidate(n, openNode, boardName()))
	}
	return out, nil
}

/*
theCollection is whether a node is the product's control collection. On
Windows each collection is a node of its own, and a product with more than one
on the page is read only on the usage OpenRGB reads it on. On Linux a node is
the whole interface, and this is always true.
*/
func theCollection(n hidraw.Node) bool {
	want, ok := pickedUsage[n.Product]
	if !ok || len(n.Descriptor) > 0 {
		return true
	}
	for _, r := range n.Reports {
		if r.Page == controlPage && r.Usage == want {
			return true
		}
	}
	return false
}

/*
boardName is the mainboard as OpenRGB names it: "ASUS " and the board's DMI
name ("ASUS ROG MAXIMUS Z790 HERO"), or "" where there is none to read.
*/
func boardName() string {
	b, err := os.ReadFile(dmiBoard)
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(string(b))
	if name == "" {
		return ""
	}
	return "ASUS " + name
}

// node is an open controller: a *hidraw.Handle, or a test's stand-in.
type node interface {
	hidraw.ReportDevice
	io.Closer
	Drain(depth int)
}

func openNode(path string) (node, error) { return hidraw.Open(path) }

// candidate is one node as a candidate, refusing a product the allow-list
// does not name before anything is opened.
func candidate(n hidraw.Node, open func(string) (node, error), board string) sanshoku.Candidate {
	id := sanshoku.Identity{
		Vendor: n.Vendor, Product: n.Product, Bus: sanshoku.BusUSB,
		Name: n.Name, Phys: n.Phys, Path: n.Path,
	}
	switch {
	case products[n.Product] != "":
		id.Name = products[n.Product]
	case board != "":
		id.Name = board
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
			if err := d.start(ctx); err != nil {
				_ = nd.Close()
				return nil, err
			}
			return d, nil
		},
	}
}

// device is one open controller.
type device struct {
	id sanshoku.Identity

	mu sync.Mutex
	// rd is the open node. Nil once closed.
	rd node
	// version is the firmware's own string ("AULA3-6K75-0309").
	version string
	// ch are the channels: the board's own first, then each header.
	ch []channel
}

// Identity is the node's, named after the board.
func (d *device) Identity() sanshoku.Identity { return d.id }

// Close closes the node and sends nothing: the lights keep their last frame.
func (d *device) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rd == nil {
		return nil
	}
	err := d.rd.Close()
	d.rd = nil
	return err //nolint:wrapcheck // the handle names the node
}

// errNoAnswer is a question the controller did not answer as expected.
var errNoAnswer = errors.New("the controller did not answer")

/*
start reads the firmware's version and the configuration table, lays out the
channels from it, and sends the packet OpenRGB sends next (`52 53 00 01`),
whose meaning nobody has written down. The caller has not shared the device.
*/
func (d *device) start(ctx context.Context) error {
	v, err := d.ask(ctx, askVersion, gotVersion)
	if err != nil {
		return fmt.Errorf("%s: its version: %w", d.id, err)
	}
	d.version = strings.TrimRight(string(v[1:17]), "\x00")
	t, err := d.ask(ctx, askConfig, gotConfig)
	if err != nil {
		return fmt.Errorf("%s: its configuration: %w", d.id, err)
	}
	d.ch = channelsFrom(t[3:63], d.version)
	return d.write(0x52, 0x53, 0x00, 0x01)
}

/*
ask writes a question and returns the reply after the report ID: the report
whose first byte is want. The caller holds mu, or has not shared the device.
*/
func (d *device) ask(ctx context.Context, q, want byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, replyWait)
	defer cancel()
	d.rd.Drain(hidraw.QueueDepth)
	req := make([]byte, 1+reportLen)
	req[0], req[1] = reportID, q
	matches := func(r []byte) bool { return len(r) == 1+reportLen && r[0] == reportID && r[1] == want }
	reply, err := hidraw.Exchange(ctx, d.rd, req, matches, 1+reportLen)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errNoAnswer, err)
	}
	return reply[1:], nil
}

// write sends one report the controller does not answer. The caller holds
// mu, or has not shared the device.
func (d *device) write(data ...byte) error {
	if d.rd == nil {
		return fmt.Errorf("%s: %w", d.id.Path, os.ErrClosed)
	}
	r := make([]byte, 1+reportLen)
	r[0] = reportID
	copy(r[1:], data)
	return d.rd.Write(r) //nolint:wrapcheck // the handle names the node
}
