package razer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/hidraw"
	"github.com/ushineko/sanshoku/lighting"
)

/*
Razer lighting (spec 043), beside the batteries and on the same reports.

A device draws a frame as OpenRGB's Direct mode does: each row of its matrix
as a custom-frame report, then the report that shows the custom frame. Three
matrix types say which reports: standard, extended and linear (OpenRGB 1.1,
from OpenRazer). A mouse behind a receiver, the Mouse Dock Pro first, is
reached through the receiver's node on the mouse's own transaction ID.

Only the reports on the allow-list are sent: the custom frame and its mode,
which do not store anything, and the question that lists a receiver's paired
devices.
*/

// lightsName is the lighting driver's name.
const lightsName = "razer-lights"

type matrix int

const (
	matrixStandard matrix = iota
	matrixExtended
	matrixLinear
)

type zoneKind int

const (
	zoneSingle zoneKind = iota
	zoneLinear
	zoneMatrix
)

// litZone is one of a device's zones, as OpenRGB names and sizes it.
type litZone struct {
	name       string
	kind       zoneKind
	rows, cols int
}

// litDevice is a product's lighting: its name, report, transaction ID,
// matrix, and zones, which fill the matrix row by row in order.
type litDevice struct {
	name        string
	matrix      matrix
	transaction byte
	rows, cols  int
	zones       []litZone
}

// The reports, by class and command (OpenRGB 1.1).
const (
	classGeneral       = 0x00
	commandPaired      = 0xBF // get: a receiver's pairing slots
	classStandard      = 0x03
	commandModeStd     = 0x0A // the standard and linear custom mode
	commandFrameStd    = 0x0B
	commandFrameLinear = 0x0C
	classExtended      = 0x0F
	commandModeExt     = 0x02 // the extended custom mode
	commandFrameExt    = 0x03
	effectCustomFrame  = 0x08
	modeCustom         = 0x05
	storageNone        = 0x00
	argsMax            = 80
	linearSize         = 0x32
)

// allowed are the reports the lighting driver sends (spec 043 R1.4).
var allowed = map[[2]byte]bool{
	{classGeneral, commandPaired}:       true,
	{classStandard, commandModeStd}:     true,
	{classStandard, commandFrameStd}:    true,
	{classStandard, commandFrameLinear}: true,
	{classExtended, commandModeExt}:     true,
	{classExtended, commandFrameExt}:    true,
}

// relayWait bounds the wait for a relayed report: OpenRGB polls 50 times, 2
// ms apart, for the mouse to answer through its receiver.
const (
	relayPolls = 50
	relayPause = 2 * time.Millisecond
	askWait    = 300 * time.Millisecond
)

/*
Lights finds and opens the lights of Razer devices: one candidate per control
node of a product in lit, and one per device a receiver reports paired to it.

The zero value is ready to use.
*/
type Lights struct{}

// Name is "razer-lights".
func (Lights) Name() string { return lightsName }

// Seams for tests: where the nodes are listed and how one is opened.
var (
	lightNodes = func() ([]hidraw.Node, error) { return hidraw.Nodes(razerVendor, speaksRazer) }
	openLights = func(path string) (lightNode, error) { return hidraw.Open(path) }
)

// lightNode is an open control node: a *hidraw.Handle, or a test's stand-in.
type lightNode interface {
	featureDevice
	Close() error
}

/*
pairedSeen is what each receiver listed as paired to it, per node, for the
life of the process: asked once, as a consumer may look for devices every few
seconds. A replugged receiver is a new node and is asked again.
*/
var pairedSeen = struct {
	sync.Mutex
	found map[string][]uint16
}{found: map[string][]uint16{}}

/*
Find returns a candidate per Razer control node whose product is in lit, and
one per device in lit that a receiver lists as paired to it, named after both
as OpenRGB names it: "Razer Basilisk V3 Pro (Wireless) (Razer Mouse Dock
Pro)". Asking for the pairing is a getter, sent once per node per process;
a device that is not a receiver refuses it. ErrAbsent when there is none.
*/
func (Lights) Find(ctx context.Context) ([]sanshoku.Candidate, error) {
	nodes, err := lightNodes()
	if err != nil {
		return nil, err
	}
	var out []sanshoku.Candidate
	for _, n := range nodes {
		own, ok := lit[n.Product]
		if !ok {
			continue
		}
		id := identity(n)
		id.Name = own.name
		out = append(out, lightCandidate(n, id, own, false))
		for _, pid := range paired(ctx, n, own) {
			dev, ok := lit[pid]
			if !ok || pid == n.Product {
				continue
			}
			pid2 := identity(n)
			pid2.Product = pid
			pid2.Name = dev.name + " (" + own.name + ")"
			out = append(out, lightCandidate(n, pid2, dev, true))
		}
	}
	if len(out) == 0 {
		return nil, sanshoku.ErrAbsent
	}
	return out, nil
}

// paired is the products a receiver lists, asking once per node.
func paired(ctx context.Context, n hidraw.Node, own litDevice) []uint16 {
	pairedSeen.Lock()
	defer pairedSeen.Unlock()
	if found, ok := pairedSeen.found[n.Path]; ok {
		return found
	}
	var found []uint16
	if h, err := openLights(n.Path); err == nil {
		r := &lightDevice{fd: h, transaction: own.transaction}
		if reply, err := r.query(ctx, classGeneral, commandPaired, argsMax); err == nil {
			body := reply[1:]
			slots := int(body[argsAt])
			for s := 0; s < slots && argsAt+s*3+3 < crcAt; s++ {
				pid := uint16(body[argsAt+s*3+2])<<8 | uint16(body[argsAt+s*3+3])
				if pid != 0x0000 && pid != 0xFFFF {
					found = append(found, pid)
				}
			}
		}
		_ = h.Close()
	}
	pairedSeen.found[n.Path] = found
	return found
}

func lightCandidate(n hidraw.Node, id sanshoku.Identity, dev litDevice, relayed bool) sanshoku.Candidate {
	return sanshoku.Candidate{
		Identity: id,
		Driver:   lightsName,
		Key:      n.Key,
		Open: func(context.Context) (sanshoku.Device, error) {
			h, err := openLights(n.Path)
			if err != nil {
				return nil, err
			}
			return newLightDevice(id, h, dev, relayed), nil
		},
	}
}

// lightDevice is one device's lights, open on its node.
type lightDevice struct {
	id          sanshoku.Identity
	dev         litDevice
	transaction byte
	relayed     bool

	mu sync.Mutex
	fd lightNode
	// held is the frame, row by row; sent whether the device has it.
	held []colour
	sent bool
}

type colour struct{ r, g, b uint8 }

func newLightDevice(id sanshoku.Identity, fd lightNode, dev litDevice, relayed bool) *lightDevice {
	return &lightDevice{
		id: id, dev: dev, transaction: dev.transaction, relayed: relayed,
		fd: fd, held: make([]colour, dev.rows*dev.cols),
	}
}

func (d *lightDevice) Identity() sanshoku.Identity { return d.id }

// Close closes the node and sends nothing: the device keeps its frame.
func (d *lightDevice) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fd == nil {
		return nil
	}
	err := d.fd.Close()
	d.fd = nil
	return err //nolint:wrapcheck // the handle names the node
}

var (
	_ lighting.Canvas = (*lightDevice)(nil)
	_ lighting.Zoned  = (*lightDevice)(nil)
)

/*
frameFloor is the shortest interval at which frames are sent.

Provisional until spec 043's bench run measures the dock and the mouse.
*/
const frameFloor = 50 * time.Millisecond

// Keys is every zone's lights in matrix order, named as OpenRGB names them:
// "Logo" for a single light, "LED Strip LED 4" for one of several.
func (d *lightDevice) Keys() []lighting.Key {
	var out []lighting.Key
	for _, z := range d.dev.zones {
		n := z.rows * z.cols
		for i := range n {
			name := z.name
			if n > 1 {
				name += " LED " + strconv.Itoa(i+1)
			}
			out = append(out, lighting.Key{ID: byte(len(out)), Name: name}) //nolint:gosec // at most 256, by lit
		}
	}
	return out
}

// Zones is the device's zones, as OpenRGB lays them out.
func (d *lightDevice) Zones() []lighting.Zone {
	var out []lighting.Zone
	first := 0
	for _, z := range d.dev.zones {
		lz := lighting.Zone{Name: z.name, First: first, Count: z.rows * z.cols}
		switch z.kind {
		case zoneSingle:
			lz.Shape = lighting.ShapeSingle
		case zoneLinear:
			lz.Shape = lighting.ShapeLine
		case zoneMatrix:
			lz.Shape = lighting.ShapeGrid
		}
		out = append(out, lz)
		first += lz.Count
	}
	return out
}

// Resize refuses: a Razer device's zones are its own.
func (d *lightDevice) Resize(zone string, _ int) error {
	return fmt.Errorf("%q on %s: %w", zone, d.id, lighting.ErrNoZone)
}

// Floor is the shortest interval at which frames are sent.
func (d *lightDevice) Floor() time.Duration { return frameFloor }

/*
Frame draws px over the held frame and, if it changed, sends it: each row as
a custom-frame report, then the report that shows the custom frame, neither
of which stores anything. A key not in Keys is an error and nothing is sent.
*/
func (d *lightDevice) Frame(ctx context.Context, px []lighting.Pixel) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fd == nil {
		return fmt.Errorf("lighting %s: %w", d.id.Path, os.ErrClosed)
	}
	for _, p := range px {
		if int(p.ID) >= len(d.held) {
			return fmt.Errorf("a frame to %s: key %d is not a light on it", d.id, p.ID)
		}
	}
	for _, p := range px {
		if next := (colour{p.R, p.G, p.B}); d.held[p.ID] != next {
			d.held[p.ID], d.sent = next, false
		}
	}
	if d.sent {
		return nil
	}
	for row := range d.dev.rows {
		class, command, args := frameRow(d.dev, row, d.held[row*d.dev.cols:(row+1)*d.dev.cols])
		if err := d.send(ctx, class, command, args); err != nil {
			return fmt.Errorf("a frame to %s: %w", d.id, err)
		}
	}
	class, command, args := customMode(d.dev.matrix)
	if err := d.send(ctx, class, command, args); err != nil {
		return fmt.Errorf("a frame to %s: %w", d.id, err)
	}
	d.sent = true
	return nil
}

/*
frameRow is one row's custom-frame report: its class, command and arguments,
whose length is the report's data size.
*/
func frameRow(dev litDevice, row int, lights []colour) (class, command byte, args []byte) {
	rgb := make([]byte, 0, len(lights)*3)
	for _, c := range lights {
		rgb = append(rgb, c.r, c.g, c.b)
	}
	stop := byte(dev.cols - 1) //nolint:gosec // at most 25 columns, by lit
	switch dev.matrix {
	case matrixExtended:
		return classExtended, commandFrameExt, append([]byte{0, 0, byte(row), 0, stop}, rgb...) //nolint:gosec // rows are few
	case matrixLinear:
		return classStandard, commandFrameLinear, append([]byte{0, stop}, rgb...)
	default:
		return classStandard, commandFrameStd, append([]byte{0xFF, byte(row), 0, stop}, rgb...) //nolint:gosec // rows are few
	}
}

// customMode is the report that shows the custom frame, storing nothing.
func customMode(m matrix) (class, command byte, args []byte) {
	if m == matrixExtended {
		args := make([]byte, 0x0C)
		args[2] = effectCustomFrame
		return classExtended, commandModeExt, args
	}
	return classStandard, commandModeStd, []byte{modeCustom, storageNone}
}

/*
Release forgets the frame and sends nothing: no report is known that returns
a device to the effect stored in it short of writing one, and that would
store it.
*/
func (d *lightDevice) Release(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fd == nil {
		return fmt.Errorf("releasing %s: %w", d.id.Path, os.ErrClosed)
	}
	clear(d.held)
	d.sent = false
	return nil
}

// errNotAllowed is a report the lighting driver does not send.
var errNotAllowed = errors.New("not a report the lighting driver sends")

/*
report builds a report with its leading report number: the transaction, the
data size, class, command and arguments, and the checksum.
*/
func report(transaction, class, command byte, args []byte) ([]byte, error) {
	size := len(args)
	if class == classStandard && command == commandFrameLinear {
		// OpenRGB declares a linear frame 0x32 bytes, whatever its row.
		size = linearSize
	}
	if !allowed[[2]byte{class, command}] {
		return nil, fmt.Errorf("%#02x/%#02x: %w", class, command, errNotAllowed)
	}
	if len(args) > argsMax {
		return nil, fmt.Errorf("%#02x/%#02x: %d bytes of arguments", class, command, len(args))
	}
	out := make([]byte, reportSize+1)
	body := out[1:]
	body[1] = transaction
	body[5] = byte(size) //nolint:gosec // at most 80
	body[6] = class
	body[7] = command
	copy(body[argsAt:], args)
	body[crcAt] = crc(body)
	return out, nil
}

/*
send writes one report. Behind a receiver the device answers busy while the
receiver relays it and drops a report sent meanwhile, so the answer is polled
for, as OpenRGB does. The caller holds mu.
*/
func (d *lightDevice) send(ctx context.Context, class, command byte, args []byte) error {
	r, err := report(d.transaction, class, command, args)
	if err != nil {
		return err
	}
	if err := d.fd.SetFeature(ctx, r); err != nil {
		return err //nolint:wrapcheck // the handle names the node
	}
	if !d.relayed {
		return nil
	}
	for range relayPolls {
		time.Sleep(relayPause)
		reply := make([]byte, reportSize+1)
		if err := d.fd.GetFeature(ctx, reply); err != nil {
			return nil //nolint:nilerr // the report went; a lost answer is OpenRGB's case too
		}
		if answered(reply, d.transaction, class, command) {
			return nil
		}
	}
	return nil
}

// answered is whether a reply is the final answer to a report: the same
// transaction, class and command, and a status past new and busy.
func answered(reply []byte, transaction, class, command byte) bool {
	body := reply[1:]
	return body[1] == transaction && body[6] == class && body[7] == command &&
		body[0] != 0x00 && body[0] != statusBusy
}

/*
query sends a getter and reads its final answer, polling as OpenRGB does, and
returns it with its leading report number. Only a success is an answer.
*/
func (d *lightDevice) query(ctx context.Context, class, command byte, size int) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, askWait)
	defer cancel()
	r, err := report(d.transaction, class, command, make([]byte, size))
	if err != nil {
		return nil, err
	}
	if err := d.fd.SetFeature(ctx, r); err != nil {
		return nil, err //nolint:wrapcheck // the handle names the node
	}
	for range 25 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err() //nolint:wrapcheck // the caller's deadline
		case <-time.After(relayPause):
		}
		reply := make([]byte, reportSize+1)
		if err := d.fd.GetFeature(ctx, reply); err != nil {
			return nil, err //nolint:wrapcheck // the handle names the node
		}
		if answered(reply, d.transaction, class, command) {
			if reply[1] != statusOK {
				return nil, fmt.Errorf("%#02x/%#02x: status %#02x", class, command, reply[1])
			}
			return reply, nil
		}
	}
	return nil, errSilent
}
