package keychron

import (
	"context"
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
const driverName = "keychron"

// vendor is Keychron's USB vendor ID.
const vendor = 0x3434

// rawPage is VIA's raw HID page.
const rawPage = 0xFF60

// products is the allow-list, and what each is called (OpenRGB's names).
var products = map[uint16]string{
	0x0E40: "Keychron K4 HE",
}

const (
	// reportLen is a report's bytes after its report ID of 0.
	reportLen = 32

	cmdVIAVersion = 0x01
	cmdSetValue   = 0x07
	cmdGetValue   = 0x08
	cmdFeatures   = 0xA2
	cmdRGB        = 0xA8

	rgbLEDCount  = 0x05
	rgbSetType   = 0x08
	rgbSetColour = 0x0A

	// matrix is the VIA channel of the RGB matrix, and these its values.
	matrix          = 0x03
	valueBrightness = 0x01
	valueEffect     = 0x02

	// perKeyEffect is the per-key RGB effect's number on the K4 HE's
	// firmware (v1.1.1), as on the Q1 HE's (spec 028).
	perKeyEffect = 23
	// solid is the per-key type that shows each key's colour still.
	solid = 0x00

	// featureRGB is the Keychron feature bit for its RGB commands.
	featureRGB = 1 << 7

	// replyWait bounds one exchange. Replies came in about 3 ms.
	replyWait = time.Second
)

/*
Driver finds and opens Keychron keyboards.

The zero value is ready to use.
*/
type Driver struct{}

// Name is "keychron".
func (Driver) Name() string { return driverName }

/*
Find returns one candidate per Keychron node on VIA's raw page. ErrAbsent when
there is none. A product not on the allow-list is a candidate, so it can be
seen, and opening it is refused without its node being opened.
*/
func (Driver) Find(context.Context) ([]sanshoku.Candidate, error) {
	nodes, err := hidraw.Nodes(vendor, hidraw.UsagePage(rawPage))
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, sanshoku.ErrAbsent
	}
	out := make([]sanshoku.Candidate, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, candidate(n, openNode))
	}
	return out, nil
}

// node is an open raw interface: a *hidraw.Handle, or a test's stand-in.
type node interface {
	hidraw.ReportDevice
	io.Closer
	Drain(depth int)
}

func openNode(path string) (node, error) { return hidraw.Open(path) }

func candidate(n hidraw.Node, open func(string) (node, error)) sanshoku.Candidate {
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
			if err := d.start(ctx); err != nil {
				_ = nd.Close()
				return nil, err
			}
			return d, nil
		},
	}
}

// device is one open keyboard.
type device struct {
	id sanshoku.Identity

	mu sync.Mutex
	rd node
	// held is each key's colour; sentHS each key's hue and saturation as
	// last sent, nil before the first frame.
	held   []colour
	sentHS [][2]uint8
	// bright is the board's brightness as last set, -1 before.
	bright int
	// found is what the keyboard ran before this handle took it, to give
	// back on Release; nil before the claim.
	found *setting
}

// setting is an effect and a brightness.
type setting struct{ effect, brightness uint8 }

func (d *device) Identity() sanshoku.Identity { return d.id }

// Close closes the node and sends nothing: the keyboard keeps its frame.
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

var (
	errNoAnswer = errors.New("the keyboard did not answer")
	errNoRGB    = errors.New("the keyboard has no Keychron RGB")
)

// start asks the keyboard what it is and how many lights it has. The caller
// has not shared the device.
func (d *device) start(ctx context.Context) error {
	if _, err := d.ask(ctx, cmdVIAVersion); err != nil {
		return err
	}
	f, err := d.ask(ctx, cmdFeatures)
	if err != nil {
		return err
	}
	if f[2]&featureRGB == 0 {
		return fmt.Errorf("%s: %w", d.id, errNoRGB)
	}
	c, err := d.ask(ctx, cmdRGB, rgbLEDCount)
	if err != nil {
		return err
	}
	n := int(c[2])<<8 | int(c[3])
	if n == 0 || n > 256 {
		return fmt.Errorf("%s: %d lights: %w", d.id, n, errNoAnswer)
	}
	d.held, d.bright = make([]colour, n), -1
	return nil
}

/*
ask writes a command and returns the reply that echoes it: its first byte,
and its second for a Keychron RGB command. The caller holds mu, or has not
shared the device.
*/
func (d *device) ask(ctx context.Context, data ...byte) ([]byte, error) {
	if d.rd == nil {
		return nil, fmt.Errorf("%s: %w", d.id.Path, os.ErrClosed)
	}
	ctx, cancel := context.WithTimeout(ctx, replyWait)
	defer cancel()
	d.rd.Drain(hidraw.QueueDepth)
	req := make([]byte, 1+reportLen)
	copy(req[1:], data)
	matches := func(r []byte) bool {
		return len(r) >= 4 && r[0] == data[0] && (data[0] != cmdRGB || r[1] == data[1])
	}
	reply, err := hidraw.Exchange(ctx, d.rd, req, matches, reportLen)
	if err != nil {
		return nil, fmt.Errorf("%s, command %02x: %w: %w", d.id, data[0], errNoAnswer, err)
	}
	return reply, nil
}
