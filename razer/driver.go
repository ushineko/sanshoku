package razer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/battery"
	"github.com/ushineko/sanshoku/hidraw"
)

/*
Razer batteries, through whatever is in front of them.

**The mouse is not the device this talks to.** On the desk hayami's spec 016
was written for, a Razer mouse sits on a Mouse Dock Pro and never enumerates
at all: the dock is the receiver, its `input0` carries `mouse0`, and `lsusb`
shows the dock alone. OpenRazer sees the same thing and offers no battery for
it, because its accessory driver has none -- `charge_level` lives in the mouse
driver, and no mouse is bound.

So the mouse is asked through the dock's RF relay, which is a transaction ID
and nothing more exotic. What comes back is the mouse's own answer: a serial
that is not the dock's, a real DPI, and Razer's 25 % low-battery default.

Only getters are sent. Nothing here writes a setting to a device.
*/

// driverName is the driver's name, as Name returns it and the support table keys it.
const driverName = "razer"

// razerVendor is Razer's USB vendor ID.
const razerVendor = 0x1532

// The usage pages a Razer control interface declares. The Mouse Dock Pro
// declares 0xFF00; OpenRazer's devices use one or the other.
const (
	controlPage    = 0xFF00
	controlPageAlt = 0xFF01
)

// defaultTimeout is how long one exchange may take. The device answers in
// milliseconds when it answers at all (hayami, RazerTimeout).
const defaultTimeout = 300 * time.Millisecond

// defaultSettle is how long the device needs between the request and the
// reply being readable. Measured by hayami on the Mouse Dock Pro: below about
// fifty milliseconds the read returns the *previous* answer, which is worse
// than no answer.
const defaultSettle = 70 * time.Millisecond

/*
transactions are the transaction IDs to try, in order.

**Razer's commands are universal and its addressing is not.** OpenRazer drives
its whole range with one report struct and one battery getter, but picks the
transaction ID per model -- across its driver, 0xFF appears 141 times, 0x1F
126, 0x3F 114 and 0x9F 16. hayami's spec 016 hardcoded 0x1F because that is
what the Mouse Dock Pro answered, and a different Razer device would have read
nothing and looked like absent hardware (hayami issue #63).

The two docks make the point: OpenRazer uses **0x3F** for the older Mouse Dock
and **0xFF** for the Dock Pro, while 0x1F is what reaches the mouse *behind*
the Pro rather than the dock itself. Measured by hayami, 0x1F, 0x08 and 0x00
answered and 0x3F timed out, so a device accepts some and not others and there
is no single right answer to hardcode.

The order is most-likely-first for a device that relays: the relay ID, then the
two the docks themselves use, then the rest. Only getters are sent, so trying
is a few extra reads of commands whose effect is documented.
*/
var transactions = []byte{0x1F, 0x3F, 0xFF, 0x9F, 0x00}

/*
kinds is what each known product is.

hayami's spec 016 hardcoded KindMouse, which is true of a Mouse Dock and wrong
of a Razer keyboard or headset. A product not in here is KindOther -- the same
answer the SteelSeries driver settled on rather than guess, after an attempt
to infer a device's kind from its sibling interfaces read a keyboard as a
mouse.
*/
var kinds = map[uint16]battery.Kind{
	productMouseDock:    battery.KindMouse, // a charger, which answers "unsupported"
	productBasiliskUlt:  battery.KindMouse, // Basilisk Ultimate's own dongle
	productMouseDockPro: battery.KindMouse, // relays the mouse on it
}

/*
Driver reads Razer batteries through feature reports.

The zero value is ready to use. Settle is the wait between sending a request
and reading its reply; zero means 70 ms. Timeout bounds one exchange; zero
means 300 ms.
*/
type Driver struct {
	Settle  time.Duration
	Timeout time.Duration
}

// Name is "razer".
func (Driver) Name() string { return driverName }

/*
Find returns one candidate per Razer hidraw node whose descriptor declares
usage page 0xFF00 or 0xFF01. ErrAbsent when there is none.

A Razer dock presents several interfaces and only the control one answers a
feature report; the others are the relayed mouse and keyboard. They are told
apart by the vendor usage page the control interface declares, for the reason
every driver here matches on a descriptor and not on a number: the numbering
moves when a device is replugged, and the product ID is not a device's
identity either. Find opens nothing.
*/
func (d Driver) Find(context.Context) ([]sanshoku.Candidate, error) {
	nodes, err := hidraw.Nodes(razerVendor, speaksRazer)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, sanshoku.ErrAbsent
	}
	settle, timeout := d.Settle, d.Timeout
	if settle <= 0 {
		settle = defaultSettle
	}
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	out := make([]sanshoku.Candidate, 0, len(nodes))
	for _, n := range nodes {
		id := identity(n)
		out = append(out, sanshoku.Candidate{
			Identity: id,
			Driver:   driverName,
			Open: func(context.Context) (sanshoku.Device, error) {
				h, err := hidraw.Open(n.Path)
				if err != nil {
					return nil, err
				}
				return &device{id: id, settle: settle, timeout: timeout, fd: h, handle: h}, nil
			},
		})
	}
	return out, nil
}

// speaksRazer is the node predicate: the control interface's vendor page.
func speaksRazer(n hidraw.Node) bool {
	return hidraw.UsagePage(controlPage)(n) || hidraw.UsagePage(controlPageAlt)(n)
}

// identity is what a node is called and where it is.
func identity(n hidraw.Node) sanshoku.Identity {
	bus := sanshoku.BusUSB
	if n.Bus == hidraw.BusBluetooth {
		bus = sanshoku.BusBluetooth
	}
	return sanshoku.Identity{
		Vendor: n.Vendor, Product: n.Product, Bus: bus,
		Name: n.Name, Phys: n.Phys, Path: n.Path,
	}
}

// featureDevice is a hidraw node spoken to with feature reports: a
// *hidraw.Handle, or a test's stand-in for one.
//
// Its own interface rather than hidraw.ReportDevice, because Razer devices
// declare no output report: a hidraw write would have nowhere to go, and the
// exchange is a pair of ioctls instead.
type featureDevice interface {
	SetFeature(ctx context.Context, report []byte) error
	GetFeature(ctx context.Context, report []byte) error
}

// device is one open Razer control node.
type device struct {
	id      sanshoku.Identity
	settle  time.Duration
	timeout time.Duration

	mu sync.Mutex

	// fd is what requests go through. Nil once closed.
	fd featureDevice
	// handle is the open node, which Close closes. Nil under a test.
	handle *hidraw.Handle

	// spoken is the transaction ID the device answered on, once known, so the
	// search for one costs the first poll and not every poll.
	spoken      byte
	spokenKnown bool
}

// Identity is the node's: the dock's, for a mouse read through one.
func (d *device) Identity() sanshoku.Identity { return d.id }

// Close closes the node.
func (d *device) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.fd = nil
	if d.handle == nil {
		return nil
	}
	err := d.handle.Close()
	d.handle = nil
	return err
}

/*
Batteries reads the battery the node answers for: the dock's, which for a Mouse
Dock Pro is the mouse's on it. The reading is named after the node, as hayami
names it.

**A poll that gets nothing is the ordinary case, not a fault.** The relay says
`timeout` once the mouse has passed its idle timer -- five minutes on the one
hayami measured -- and `busy` when another program is mid-exchange on the same
node, which the openrazer daemon is whenever it is running. Both return no
reading and no error. A node that has been unplugged is sanshoku.ErrGone.
*/
func (d *device) Batteries(ctx context.Context) ([]battery.Battery, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fd == nil {
		return nil, fmt.Errorf("reading %s: %w", d.id.Path, os.ErrClosed)
	}

	transaction, level, err := d.address(ctx)
	if errors.Is(err, errSilent) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	b := battery.Battery{
		Name:     d.id.Name,
		Kind:     kinds[d.id.Product],
		Level:    level,
		HasLevel: true,
	}

	// The charge state is a second exchange and an optional one: a device that
	// will not answer it still has a level worth drawing, and a missing answer
	// must not be read as "not charging".
	if _, charging, err := d.exchange(ctx, transaction, commandCharging); err == nil {
		switch {
		case !charging:
			b.State = battery.Discharging
		case b.Level == 100:
			b.State = battery.Full
		default:
			b.State = battery.Charging
		}
	}
	return []battery.Battery{b}, nil
}

/*
address finds the transaction ID this node answers on, and the level with it.

Remembered per device, because the search is the only expensive part: once a
device has answered, every later poll is one exchange again.

**A silent ID is not evidence that it is the wrong one.** `busy` and `timeout`
are what a contended or sleeping device says on the *right* ID -- several times
an hour on hayami's hardware -- so a remembered ID is kept through them rather
than being unlearnt and searched for again on the next poll, which would turn
an ordinary quiet minute into a burst of writes to a device.
*/
func (d *device) address(ctx context.Context) (byte, int, error) {
	if d.spokenKnown {
		level, _, err := d.exchange(ctx, d.spoken, commandBatteryLevel)
		return d.spoken, level, err
	}

	var err error
	for _, transaction := range transactions {
		var level int
		level, _, err = d.exchange(ctx, transaction, commandBatteryLevel)
		if err == nil {
			d.spoken, d.spokenKnown = transaction, true
			return transaction, level, nil
		}
		if errors.Is(err, sanshoku.ErrGone) || ctx.Err() != nil {
			// The node itself has gone, or the caller has: the next ID would
			// only say so again.
			return 0, 0, err
		}
	}
	return 0, 0, err
}

/*
exchange sends one getter and decodes its reply.

The reply comes back in the same shape as the request, with the status byte
filled in and the arguments replaced. Set, settle, get, each bounded by the
driver's timeout under the caller's context. The ioctls themselves cannot be
interrupted (hidraw.Handle.SetFeature); the context is checked around them and
the settle wait gives way to it. Running out of the driver's own timeout is
silence; the caller's context ending is the caller's error.
*/
func (d *device) exchange(ctx context.Context, transaction, command byte) (int, bool, error) {
	ectx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	level, charging, err := d.ask(ectx, transaction, command)
	if err != nil && ctx.Err() == nil && errors.Is(err, context.DeadlineExceeded) {
		return 0, false, fmt.Errorf("%#02x/%#02x on %s: %w", classPower, command, d.id.Path, errSilent)
	}
	return level, charging, err
}

func (d *device) ask(ctx context.Context, transaction, command byte) (int, bool, error) {
	req := request(transaction, classPower, command)
	if err := d.fd.SetFeature(ctx, req); err != nil {
		return 0, false, fmt.Errorf("asking a Razer device for %#02x/%#02x: %w", classPower, command, err)
	}

	timer := time.NewTimer(d.settle)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return 0, false, fmt.Errorf("waiting on a Razer device: %w", ctx.Err())
	case <-timer.C:
	}

	reply := make([]byte, len(req))
	if err := d.fd.GetFeature(ctx, reply); err != nil {
		return 0, false, fmt.Errorf("reading a Razer device's answer: %w", err)
	}
	return Decode(reply)
}
