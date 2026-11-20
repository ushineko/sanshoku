package gigabyte

import (
	"context"
	"encoding/binary"
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
const driverName = "gigabyte"

// vendor is ITE's USB vendor ID, which the controllers carry.
const vendor = 0x048D

// controlPage is the vendor page the controller's collections are on.
const controlPage = 0xFF89

// controlUsage is the usage of the collection that carries report CC. On
// Windows each collection is a node of its own, and only this one is read.
const controlUsage = 0xCC

/*
products are the Fusion 2 USB products OpenRGB 1.1's detector lists for this
vendor. Only 5702 has been measured; the others are found and refused until
one is (spec 039 R1.1).
*/
var products = map[uint16]bool{
	0x5702: true,  // IT5701 and IT5702, on the Z790 AORUS MASTER X (tested)
	0x8297: false, // IT8297
	0x5711: false, // IT5711, which has more headers and a second calibration report
	0x8950: false,
}

const (
	// reportID is the controller's one report for commands and answers.
	reportID = 0xCC
	// reportLen is a report's length with its ID, from the descriptor.
	reportLen = 64

	// cmdInfo asks for the controller's report, read back as a feature report.
	cmdInfo = 0x60

	// replyWait bounds one question.
	replyWait = 500 * time.Millisecond
)

// Where the controller's report keeps what is read from it (OpenRGB 1.1's
// IT8297Report, and spec 039 E0).
const (
	infoController = 2
	infoFlags      = 11
	infoProduct    = 12
	infoHeader1    = 44
	infoHeader2    = 48
	infoChip       = 56

	// flagLampArray is the flag that says the controller has a lamp-array
	// mode, which OpenRGB turns off at start.
	flagLampArray = 0x02
)

// dmiBoard is where Linux keeps the mainboard's name. A variable so that a
// test can give its own.
var dmiBoard = "/sys/class/dmi/id/board_name"

/*
Driver finds and opens Gigabyte RGB Fusion 2 USB controllers.

The zero value is ready to use.
*/
type Driver struct{}

// Name is "gigabyte".
func (Driver) Name() string { return driverName }

/*
Find returns one candidate per ITE node on usage page 0xFF89. ErrAbsent when
there is none. A product not measured is a candidate, so it can be seen, and
opening it is refused without its node being opened. Find opens nothing.
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
theCollection is whether a node is the collection that carries report CC. On
Linux a node is the whole interface, and this is always true.
*/
func theCollection(n hidraw.Node) bool {
	if len(n.Descriptor) > 0 {
		return true
	}
	for _, r := range n.Reports {
		if r.Page == controlPage && r.Usage == controlUsage {
			return true
		}
	}
	return false
}

// boardName is the mainboard's DMI name ("Z790 AORUS MASTER X"), or "" where
// there is none to read.
func boardName() string {
	b, err := os.ReadFile(dmiBoard)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// node is an open controller: a *hidraw.Handle, or a test's stand-in. The
// controller declares only feature reports, so that is all it is spoken to by.
type node interface {
	SetFeature(ctx context.Context, report []byte) error
	GetFeature(ctx context.Context, report []byte) error
	io.Closer
}

func openNode(path string) (node, error) { return hidraw.Open(path) }

// candidate is one node as a candidate, named after the board, refusing a
// product not measured before anything is opened.
func candidate(n hidraw.Node, open func(string) (node, error), board string) sanshoku.Candidate {
	id := sanshoku.Identity{
		Vendor: n.Vendor, Product: n.Product, Bus: sanshoku.BusUSB,
		Name: n.Name, Phys: n.Phys, Path: n.Path,
	}
	if board != "" {
		id.Name = board
	}
	return sanshoku.Candidate{
		Identity: id,
		Driver:   driverName,
		Key:      n.Key,
		Open: func(ctx context.Context) (sanshoku.Device, error) {
			if !products[n.Product] {
				return nil, fmt.Errorf("%s: a Fusion 2 product not yet measured: %w", id, sanshoku.ErrUnsupported)
			}
			if board == "" {
				return nil, fmt.Errorf("%s: the mainboard's name cannot be read, so its layout is not known: %w", id, sanshoku.ErrUnsupported)
			}
			nd, err := open(n.Path)
			if err != nil {
				return nil, err
			}
			d := &device{id: id, rd: nd}
			if err := d.start(ctx, board); err != nil {
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
	// firmware is the controller's own product string ("IT5701-GIGABYTE V7.0.18.1").
	firmware string
	// zones are the controller's lights, in the board table's order.
	zones []zone
	// lengthsSent is the length classes last sent (CC 34); claimed is whether
	// this handle has turned the headers' own effect off (CC 32).
	lengthsSent [2]byte
	lengthsKnow bool
	claimed     bool
}

// Identity is the node's, named after the board and the controller's chip.
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

// errNoAnswer is a question the controller did not answer.
var errNoAnswer = errors.New("the controller did not answer")

/*
start reads the controller's report, finds the board's layout for its
controller number, and sends what OpenRGB sends at start: lamp array off where
the controller has one, the reset of the effect registers and their commit,
and beat off. Nothing beyond the question is sent to a board not in the table.
The caller has not shared the device.
*/
func (d *device) start(ctx context.Context, board string) error {
	info, err := d.ask(ctx, cmdInfo)
	if err != nil {
		return fmt.Errorf("%s: its report: %w", d.id, err)
	}
	num := info[infoController]
	layout, ok := layoutFor(boardKey{board, num})
	if !ok {
		return fmt.Errorf("%s: controller %d of a board whose layout has not been measured: %w", d.id, num, sanshoku.ErrUnsupported)
	}
	d.firmware = strings.TrimRight(strings.TrimRight(string(info[infoProduct:infoProduct+28]), "\x00"), " ")
	d.id.Name = fmt.Sprintf("%s (0x%08X)", board, binary.LittleEndian.Uint32(info[infoChip:]))
	orders := [2]order{
		orderFrom(binary.LittleEndian.Uint32(info[infoHeader1:])),
		orderFrom(binary.LittleEndian.Uint32(info[infoHeader2:])),
	}
	d.zones = zonesFrom(layout, orders)

	if info[infoFlags]&flagLampArray != 0 {
		if err := d.write(ctx, cmdLampArray, 0x00); err != nil {
			return err
		}
	}
	for reg := byte(cmdEffectFirst); reg <= cmdEffectLast; reg++ {
		if err := d.write(ctx, reg, 0x00, 0x00); err != nil {
			return err
		}
	}
	if err := d.write(ctx, cmdCommit, 0xFF, 0x00); err != nil {
		return err
	}
	return d.write(ctx, cmdBeat, 0x00)
}

// ask writes a command and reads the controller's answer, report ID first,
// within replyWait. The caller holds mu, or has not shared the device.
func (d *device) ask(ctx context.Context, cmd byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, replyWait)
	defer cancel()
	if err := d.write(ctx, cmd); err != nil {
		return nil, err
	}
	reply := make([]byte, reportLen)
	reply[0] = reportID
	if err := d.rd.GetFeature(ctx, reply); err != nil {
		return nil, fmt.Errorf("%w: %w", errNoAnswer, err)
	}
	if reply[0] != reportID {
		return nil, fmt.Errorf("%w: a report with id %02x", errNoAnswer, reply[0])
	}
	return reply, nil
}

/*
write sends one command report: ID CC, then data, zero-filled to 64 bytes. It
refuses the commands that change what the board keeps (calibration, flash,
persistence, Gen2 scans), which this driver never sends (spec 039 R1.5). The
caller holds mu, or has not shared the device.
*/
func (d *device) write(ctx context.Context, data ...byte) error {
	if d.rd == nil {
		return fmt.Errorf("%s: %w", d.id.Path, os.ErrClosed)
	}
	if len(data) > 0 && kept(data[0]) {
		return fmt.Errorf("%s: command %02x changes what the board keeps, and is never sent", d.id, data[0])
	}
	r := make([]byte, reportLen)
	r[0] = reportID
	copy(r[1:], data)
	return d.rd.SetFeature(ctx, r) //nolint:wrapcheck // the handle names the node
}

// kept is whether a command changes what the board keeps past a power cycle,
// or scans its headers: calibration (33), Gen2 scans (38 to 3D), persistence
// (47) and the save to flash (5E), as OpenRGB 1.1 names them.
func kept(cmd byte) bool {
	return cmd == 0x33 || (cmd >= 0x38 && cmd <= 0x3D) || cmd == 0x47 || cmd == 0x5E
}
