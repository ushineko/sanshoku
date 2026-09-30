package nzxt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/cooling"
	"github.com/ushineko/sanshoku/hidraw"
	"github.com/ushineko/sanshoku/usbfs"
)

/*
The NZXT Kraken, directly.

No liquidctl. A status reading costs about two milliseconds over /dev/hidraw
against a hundred and five through a Python interpreter, and the screen's bulk
endpoint is reachable through usbfs while usbhid keeps the HID interface. The
protocol was read off an Elite V2 on firmware 1.2.0 by hotaru (its spec 012,
which has the measurements); this is hotaru's internal/cooler, ported.
*/

// driverName is the driver's name, as Name returns it and the support table keys it.
const driverName = "nzxt"

// vendor is NZXT's USB vendor ID, which every product in Known belongs to.
const vendor = 0x1E71

// Size is a panel's width and height in pixels.
type Size struct {
	W, H int
}

/*
Model is a product this driver recognises.

The screen is part of the model rather than something asked of the device,
because claiming the panel is what opening it means, and a program that
claimed somebody's screen to find out whether they had one would take it away
from whatever else was drawing on it to answer a question nobody asked. The
zero Size is a model with no panel.
*/
type Model struct {
	Name   string
	Screen Size
}

/*
Known is the hardware this driver will write to.

A list rather than a vendor match. The Kraken family differs by product: the
protocol here was read off a Kraken Elite on firmware 1.2.0, and a cooler that
merely shares a vendor ID is not the same device. A product not in here is
found and refused with sanshoku.ErrUnsupported, never written to.

640 by 640 is the panel's size on every Kraken Elite that has one; nothing in
the protocol reports it, and a GIF of any other size displays blank.
*/
var Known = map[uint16]Model{
	0x3012: {Name: "Kraken Elite", Screen: Size{W: 640, H: 640}},
}

/*
The defaults for Driver's options.

defaultFreshness: short enough that a dashboard is honest and long enough that
three consumers asking at once cost one exchange. hotaru's API, dashboard and
tray polled in the same second. A reading takes about two milliseconds, so this
is not about the cost of the read: it is about not writing to a device more
often than anybody needs.

defaultProbe: the right node answers in about a millisecond. The wrong one
never answers at all, and without a bound that is a program that hangs at
startup rather than one that reports no cooler.
*/
const (
	defaultFreshness = 250 * time.Millisecond
	defaultProbe     = 500 * time.Millisecond
)

/*
Driver finds and opens NZXT Kraken coolers.

The zero value is ready to use. Freshness is how long a Status reading is
served to later callers before the device is asked again; zero means 250 ms.
ProbeTimeout bounds the status probe Open makes to find out whether a node is
the cooler; zero means 500 ms.
*/
type Driver struct {
	Freshness    time.Duration
	ProbeTimeout time.Duration
}

// Name is "nzxt".
func (Driver) Name() string { return driverName }

/*
Find returns one candidate per NZXT hidraw node, vendor-defined interfaces
first. ErrAbsent when there is none.

Candidates, not an answer. One device commonly exposes several hidraw nodes,
and sysfs cannot tell one of a device's nodes from another; which one actually
answers is settled by asking it, which is what Open does.

Every NZXT node is a candidate, including a product Known does not name, so the
bench and a consumer can see that it was found. Opening one of those returns
sanshoku.ErrUnsupported without the node being opened. Find opens nothing.
*/
func (d Driver) Find(context.Context) ([]sanshoku.Candidate, error) {
	nodes, err := hidraw.Nodes(vendor, nil)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, sanshoku.ErrAbsent
	}
	/*
		Vendor-defined interfaces first.

		This is the discriminator HID provides, and the one hidapi exposes as
		usage_page: a control protocol lives on a vendor-defined page, while a
		device's other collections declare standard ones. It is an ordering
		rather than a filter, because it says what an interface is *for* and
		not whether this particular firmware will answer on it, which only the
		device can say.
	*/
	sort.SliceStable(nodes, func(i, j int) bool {
		return vendorDefined(firstPage(nodes[i].Descriptor)) && !vendorDefined(firstPage(nodes[j].Descriptor))
	})
	o := d.options()
	out := make([]sanshoku.Candidate, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, candidate(n, o, openNode, openPanel))
	}
	return out, nil
}

// options is the driver's options with the defaults filled in.
func (d Driver) options() Driver {
	if d.Freshness <= 0 {
		d.Freshness = defaultFreshness
	}
	if d.ProbeTimeout <= 0 {
		d.ProbeTimeout = defaultProbe
	}
	return d
}

// vendorDefined is the usage page range reserved for whatever a vendor likes,
// which is where control protocols are found.
func vendorDefined(page uint16) bool { return page >= 0xFF00 }

/*
firstPage is the first Usage Page item of a report descriptor.

The first of them describes the collection this interface exposes. Anything
unreadable reports zero, which sorts last rather than failing: a descriptor
this driver cannot parse is not a reason to refuse a device that would have
answered.
*/
func firstPage(desc []byte) uint16 {
	var page uint16
	hidraw.Walk(desc, func(it hidraw.Item) bool {
		if it.Type == hidraw.TypeGlobal && it.Tag == hidraw.TagUsagePage {
			page = uint16(it.Value) //nolint:gosec // a usage page is sixteen bits
			return false
		}
		return true
	})
	return page
}

/*
node is the control channel: a *hidraw.Handle, or a test's stand-in.

Drain is on it because every question clears the queue first; there is no
method here that reads a report without matching it, and leaving one out of
the interface is cheaper than remembering not to call it.
*/
type node interface {
	hidraw.ReportDevice
	Drain(depth int)
	io.Closer
}

// openNode opens a real control channel.
func openNode(path string) (node, error) {
	return hidraw.Open(path)
}

// bulk is the panel's interface: a *usbfs.Interface, or a test's recorder.
type bulk interface {
	Bulk(ctx context.Context, endpoint byte, data []byte) error
	io.Closer
}

// openPanel claims the panel's interface on the real device.
func openPanel(path string) (bulk, error) {
	return usbfs.Open(path, bulkInterface)
}

/*
candidate is one node as a candidate, opening through dial and claiming the
panel through claim.

The allow-list is consulted before anything is opened. Then the node is asked
for a status reading under ProbeTimeout, and a node that does not answer `75 01`
is closed and reported absent. Asking is safe because the product is already
identified; it is decisive because a mismatched device answers with its own
prefix. A Corsair power supply, asked this on hotaru's development machine,
replied `74 96`, which is not a status reply and is rejected as one.
*/
func candidate(n hidraw.Node, o Driver, dial func(string) (node, error), claim func(string) (bulk, error)) sanshoku.Candidate {
	id := sanshoku.Identity{
		Vendor: n.Vendor, Product: n.Product, Bus: sanshoku.BusUSB,
		Name: n.Name, Phys: n.Phys, Path: n.Path,
	}
	return sanshoku.Candidate{
		Identity: id,
		Driver:   driverName,
		Open: func(ctx context.Context) (sanshoku.Device, error) {
			model, ok := Known[n.Product]
			if !ok {
				return nil, fmt.Errorf("%s: not a product this driver speaks to: %w", id, sanshoku.ErrUnsupported)
			}
			nd, err := dial(n.Path)
			if err != nil {
				return nil, err
			}
			d := &device{id: id, model: model, fresh: o.Freshness, hid: nd, showing: -1}
			probe, cancel := context.WithTimeout(ctx, o.ProbeTimeout)
			_, err = d.Status(probe)
			cancel()
			if err != nil {
				_ = nd.Close()
				return nil, fmt.Errorf("%s at %s did not answer: %w: %w", id, n.Path, sanshoku.ErrAbsent, err)
			}
			if model.Screen == (Size{}) || n.USBPath == "" {
				return d, nil
			}
			d.usbPath, d.claim = n.USBPath, claim
			return &panelDevice{d}, nil
		},
	}
}

/*
device is one open cooler: its control channel, and its panel once claimed.

One owner. Two callers writing to one HID endpoint interleave control transfers
on a single interrupt endpoint, which is a corruption risk rather than a
contention problem, so every exchange, status and panel alike, holds mu. hotaru
needed a priority queue for this in its predecessor, where every call was a
separate liquidctl process fighting for one node; holding the handle, a mutex
is the whole mechanism.
*/
type device struct {
	id    sanshoku.Identity
	model Model
	fresh time.Duration

	mu sync.Mutex
	// hid is the control channel. Nil once closed.
	hid node

	/*
		last and at coalesce reads by freshness, not by superseding. A caller
		whose request was superseded still wants a number, so concurrent
		readers share one recent answer instead of each paying for a reading
		and queueing behind the others.
	*/
	last cooling.Status
	at   time.Time

	// usbPath and claim are how the panel is claimed; usb is the claimed
	// interface, nil until the first panel call.
	usbPath string
	claim   func(string) (bulk, error)
	usb     bulk

	/*
		showing is the slot the panel is displaying, or -1 for the firmware's
		own readout.

		Tracked because deleting the slot that is on screen blanks the panel
		until the next image arrives: about a second, and unmistakable on a
		dashboard that updates every two. The device does not report which
		slot it is showing, so the driver remembers what it last asked for.
	*/
	showing int
}

// Identity is the node's.
func (d *device) Identity() sanshoku.Identity { return d.id }

/*
Status is the cooler's reading, taken now or within Freshness.

An error is never cached: a device that failed once is asked again rather than
being written off for a quarter of a second, because the next caller may be a
person who has just plugged it back in. A node that has been unplugged is
sanshoku.ErrGone.
*/
func (d *device) Status(ctx context.Context) (cooling.Status, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.hid == nil {
		return cooling.Status{}, fmt.Errorf("reading %s: %w", d.id.Path, os.ErrClosed)
	}
	if !d.at.IsZero() && time.Since(d.at) < d.fresh {
		return d.last, nil
	}
	reply, err := d.exchange(ctx, askStatus, askStatusB)
	if err != nil {
		return cooling.Status{}, err
	}
	s, err := DecodeStatus(reply)
	if err != nil {
		return cooling.Status{}, err
	}
	s.Taken = time.Now()
	d.last, d.at = s, s.Taken
	return s, nil
}

/*
Close releases the cooler, handing the screen back first.

A machine that is no longer running the program should not keep showing
whatever it last drew: the panel is somebody's cooler, and leaving a stale
dashboard on it is the same discourtesy as leaving their lights on a colour
they did not choose. The panel is returned only if it was claimed; a device
nobody drew on is left as it was.
*/
func (d *device) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.hid == nil {
		return nil
	}
	if d.usb != nil {
		ctx, cancel := context.WithTimeout(context.Background(), returnScreen)
		_ = d.liquid(ctx)
		cancel()
		_ = d.usb.Close()
		d.usb = nil
	}
	err := d.hid.Close()
	d.hid = nil
	return err //nolint:wrapcheck // hidraw.Handle.Close names the node already
}

// returnScreen bounds handing the panel back on Close. A shutdown that hangs
// on a screen is worse than one that leaves a picture.
const returnScreen = 2 * time.Second

// reportLen is every report this device sends or takes, in either direction.
// No report ID is prepended: the command's first byte is the report ID.
const reportLen = 64

/*
attempts bounds a search for a reply within one exchange.

The cooler streams status reports of its own accord, so a reply arrives among
them rather than instead of them. Twelve is liquidctl's number and is generous:
the reply is usually the first or second report to arrive.
*/
const attempts = 12

/*
exchanges is how many times a question is asked before giving up on it.

Not for a device that is slow (a read already waits) but for one whose replies
somebody else is reading. The OpenRGB server held this same hidraw node open
on hotaru's development machine, and a report it read was a report hotaru did
not: the reply to a status request simply did not arrive. Asking again is the
whole mitigation, and it is enough because losing a reply is occasional rather
than persistent.

It lives here rather than in the transport so that the fake exercises it too:
a retry the fake cannot exercise is a retry nobody has checked.
*/
const exchanges = 3

// exchange asks a question, and asks again if the answer went astray. The
// caller holds mu.
func (d *device) exchange(ctx context.Context, data ...byte) ([]byte, error) {
	var last error
	for range exchanges {
		reply, err := d.ask(ctx, data...)
		if err == nil {
			return reply, nil
		}
		if ctx.Err() != nil || errors.Is(err, sanshoku.ErrGone) {
			return nil, err
		}
		last = err
	}
	return nil, last
}

/*
ask sends a command and returns its reply, matched by prefix.

The queue is drained first. This cooler broadcasts its state about once a
second whether or not anybody asked, the kernel queues those per open handle,
and a consumer's handle is open for as long as it runs, so the queue is as deep
as the handle has been idle: a minute of quiet leaves sixty stale reports ahead
of the next reply, and a reader that looks at the first twelve finds none of
them are it. hotaru's service reported exactly that as "no 7501 reply in 12
reports", intermittently, because a run of calls keeps the queue empty and only
idleness fills it.
*/
func (d *device) ask(ctx context.Context, data ...byte) ([]byte, error) {
	d.hid.Drain(hidraw.QueueDepth)
	return await(ctx, d.hid, data...)
}

/*
await writes one command and returns the report that answers it, among at most
twelve.

A reply carries the command's prefix with the first byte incremented: `32 01`
is answered by `33 01`. The device also streams `75 02` reports nobody asked
for, and reading "the next report" after a command returns a temperature
reading about a third of the time; byte 14 of a temperature reading taken as a
result code is noise that looks like data. That mistake cost hotaru an evening.

A reply that never arrives is an error rather than a zero value: the caller is
about to read a result code out of it.
*/
func await(ctx context.Context, dev hidraw.ReportDevice, data ...byte) ([]byte, error) {
	a, b := data[0]+1, data[1]
	matches := func(r []byte) bool { return len(r) >= reportLen && r[0] == a && r[1] == b }
	reply, err := hidraw.Exchange(ctx, &budget{dev: dev, left: attempts}, report(data), matches, reportLen)
	if err != nil {
		return nil, fmt.Errorf("no %02x%02x reply: %w", a, b, err)
	}
	return reply, nil
}

// report pads a command to the report length.
func report(data []byte) []byte {
	r := make([]byte, reportLen)
	copy(r, data)
	return r
}

// errBudget is a search for a reply that read its twelve reports and found
// none of them.
var errBudget = fmt.Errorf("none in %d reports: %w", attempts, hidraw.ErrSilent)

/*
budget is a report device that stops after a number of reads.

hidraw.Exchange skips unmatched reports until its deadline; this device's rule
is liquidctl's twelve, so the count is enforced here, where Exchange reads.
*/
type budget struct {
	dev  hidraw.ReportDevice
	left int
}

func (b *budget) Write(r []byte) error { return b.dev.Write(r) }

func (b *budget) Read(ctx context.Context, buf []byte) (int, error) {
	if b.left <= 0 {
		return 0, errBudget
	}
	b.left--
	return b.dev.Read(ctx, buf)
}

/*
tell sends a command the device does not answer. The caller holds mu.

Most of this protocol is question and answer, and a few commands are not:
brightness and orientation are written and acknowledged by nothing. Waiting for
a reply to one of those times out after the full deadline, which reads as a
device that has stopped talking.
*/
func (d *device) tell(data ...byte) error {
	return d.hid.Write(report(data))
}
