package logitech

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/battery"
	"github.com/ushineko/sanshoku/hidraw"
)

// driverName is the driver's name, as Name returns it and the support table keys it.
const driverName = "logitech"

// logitechVendor is Logitech's USB vendor ID.
const logitechVendor = 0x046D

// vendorWord is how Logitech writes its own name at the front of a device's, and
// what battery.Product takes off a battery's name so the product is what is
// left to read.
const vendorWord = "Logitech"

// vendorPage is the first usage page of the vendor-defined range. A HID++
// node declares report 0x10 on a page at or above it.
const vendorPage = 0xFF00

/*
defaultTimeout is how long one HID++ request attempt may take.

Generous against what a request costs -- a reply arrives in about five
milliseconds, and an empty index is refused in about one, measured by hayami on
a Lightspeed receiver -- because it is not what discovery runs on. It is the
backstop for a device that is asleep and answers nothing at all, and a caller
polling on a timer cannot wait longer than this for one that never will.
*/
const defaultTimeout = 300 * time.Millisecond

// deviceIndices are the indices a receiver can hold. A receiver pairs at most
// six devices and answers for every index, so all six are asked once and the
// ones that answer are kept.
var deviceIndices = []byte{1, 2, 3, 4, 5, 6}

// wiredIndex is the index a device plugged in by its own cable answers on,
// where there is no receiver to number it.
const wiredIndex = 0xFF

/*
Driver reads Logitech batteries over HID++ 1.0 and 2.0 on hidraw.

The zero value is ready to use. Timeout bounds one request attempt; zero means
300 ms, hayami's RequestTimeout.
*/
type Driver struct {
	Timeout time.Duration
}

// Name is "logitech".
func (Driver) Name() string { return driverName }

/*
Find returns one candidate per Logitech hidraw node that speaks HID++: a node
whose descriptor declares report 0x10 on a vendor usage page. ErrAbsent when
there is none.

A Logitech receiver presents three nodes and only one is the HID++ endpoint;
the other two are the mouse and keyboard interfaces and never answer a
request. They are told apart by the descriptor, never by product and never by
node number. Find opens nothing.
*/
func (d Driver) Find(context.Context) ([]sanshoku.Candidate, error) {
	nodes, err := hidraw.Nodes(logitechVendor, hidraw.HasReportID(vendorPage, reportShort))
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
		id := identity(n)
		out = append(out, sanshoku.Candidate{
			Identity: id,
			Driver:   driverName,
			Open: func(context.Context) (sanshoku.Device, error) {
				h, err := hidraw.Open(n.Path)
				if err != nil {
					return nil, err
				}
				return newDevice(id, n, timeout, h), nil
			},
		})
	}
	return out, nil
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

/*
Presence is what is on a HID++ node beyond what could be read.

An empty result used to mean "no Logitech receiver" in hayami, and on a machine
with a receiver, a ten-year-old keyboard and a leftover pairing slot it meant
three other things instead (hayami issue #66). Each of them is a different
sentence and a different thing for a reader to do.
*/
type Presence struct {
	// Nodes is how many hidraw nodes speak HID++. A Device is one node, so it
	// reports 1; a consumer sums its devices' Presence to count them.
	Nodes int

	// Quiet is how many paired indices did not answer at the last discovery.
	// **Not named**, on purpose: a pairing table outlives the hardware in it,
	// and a receiver that has been round a few machines carries slots for
	// devices that were never on this desk. A count says something true; a
	// name would not.
	Quiet int

	// TooOld are devices that answered and do not speak HID++ 2.0, and whose
	// HID++ 1.0 register would not read either, by the name the kernel gives
	// their node. These are named because they answered -- something is there.
	TooOld []string
}

// Presencer is a device that reports its Presence. A logitech Device
// satisfies it; a consumer asserts it rather than naming the concrete type.
type Presencer interface {
	Presence() Presence
}

/*
device is one open HID++ node.

It remembers what it found. Discovery is cheap but not free, and the index of a
mouse that is on the desk does not change between polls; it changes when the
mouse is unpaired, and the reading failing is how that is noticed.
*/
type device struct {
	id      sanshoku.Identity
	node    hidraw.Node
	timeout time.Duration

	mu sync.Mutex

	// rd is what requests go through: handle, or a test's stand-in for one.
	// Nil once closed.
	rd hidraw.ReportDevice
	// handle is the open node, which Close closes. Nil under a test.
	handle *hidraw.Handle

	// known is what the last poll found: the indices on this node that
	// answered. Empty until the first poll, and emptied when a poll finds
	// nothing there any more.
	known []located

	// childNodes says whether the system gives a device paired to a
	// receiver a node of its own (hidraw.SplitsReceivers), which decides
	// whether a receiver node reads one. A field so a test can say which
	// system it is on.
	childNodes bool

	// presence is what the last discovery learned besides the batteries.
	presence Presence
}

// located is one device behind the node, where it was found.
type located struct {
	index byte

	// old marks a device that answered that it does not know HID++ 2.0 at all.
	// Its battery is a register rather than a feature.
	old bool

	// name is what to call an old device, which has no name feature, and empty
	// where the node cannot lend a device a name of its own -- see the
	// exclusions in discover.
	name string
}

func newDevice(id sanshoku.Identity, n hidraw.Node, timeout time.Duration, h *hidraw.Handle) *device {
	return &device{
		id: id, node: n, timeout: timeout, rd: h, handle: h,
		childNodes: hidraw.SplitsReceivers,
		presence:   Presence{Nodes: 1},
	}
}

// Identity is the node's: its kernel name, which for a receiver is the
// receiver's and not a paired device's. A reading carries the device's own.
func (d *device) Identity() sanshoku.Identity { return d.id }

// Close closes the node.
func (d *device) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.rd = nil
	if d.handle == nil {
		return nil
	}
	err := d.handle.Close()
	d.handle = nil
	return err
}

// Presence is what the last poll found besides batteries.
func (d *device) Presence() Presence {
	d.mu.Lock()
	defer d.mu.Unlock()
	p := d.presence
	p.TooOld = append([]string(nil), d.presence.TooOld...)
	if len(p.TooOld) == 0 {
		p.TooOld = nil
	}
	return p
}

/*
Batteries reads every device on the node that answers.

A device that is paired but asleep answers nothing, and is absent from the
result rather than being reported at zero, with no error. Readings that did
come back are returned beside a joined error for the ones that failed. A node
that has been unplugged is sanshoku.ErrGone.
*/
func (d *device) Batteries(ctx context.Context) ([]battery.Battery, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rd == nil {
		return nil, fmt.Errorf("reading %s: %w", d.id.Path, os.ErrClosed)
	}

	where := d.known
	if len(where) == 0 {
		var err error
		if where, err = d.discover(ctx); err != nil {
			return nil, err
		}
	}

	var (
		found []battery.Battery
		still []located
		errs  []error
	)
	unread := map[string]bool{}

	for _, loc := range where {
		b, err := d.read(ctx, loc)
		if err != nil && loc.old && loc.name != "" {
			// A device from before the feature protocol whose register would
			// not read either. It is there and it is not a reading, and
			// Presence says so by name rather than by silence.
			unread[loc.name] = true
		}
		if err != nil {
			// The device has gone quiet, or gone. Either way its index is not
			// worth remembering; the next poll rediscovers.
			// errOldProtocol among them: a register read that comes back
			// "invalid sub-id" is a device from before this protocol refusing
			// the question, which is an answer and not a fault. It is named
			// above instead.
			if !errors.Is(err, errNoDevice) && !errors.Is(err, errUnknownFeature) &&
				!errors.Is(err, errSilent) && !errors.Is(err, errOldProtocol) {
				errs = append(errs, err)
			}
			if errors.Is(err, sanshoku.ErrGone) || ctx.Err() != nil {
				// The node itself has gone, or the caller has: asking the
				// next index would only say so again.
				break
			}
			continue
		}
		found = append(found, b)
		still = append(still, loc)
	}

	d.known = still
	d.presence.TooOld = names(unread)
	return found, errors.Join(errs...)
}

/*
discover asks every index on the node which of them hold a device.

Every index answers -- a paired one with a feature index, an empty one with a
HID++ 1.0 error -- so this costs milliseconds rather than the timeout, and only
because reply knows both error forms. A reader that knew only the 2.0 form
would sit out the timeout on each empty index instead.

That is a receiver's node. A paired child's node answers for its one device
and no other, so it is asked at its own index only (spec 007): on the Unifying
receiver the other six probes of a child node could only be silent, and a
silent index costs five attempts of the timeout each.
*/
func (d *device) discover(ctx context.Context) ([]located, error) {
	var found []located
	quiet := 0
	for _, index := range d.indices() {
		_, err := featureIndex(ctx, d.rd, d.timeout, index, featureUnifiedBattery)
		switch {
		case err == nil, errors.Is(err, errUnknownFeature):
			// There, and either with a fuel gauge or with the older feature
			// read falls back to.
			found = append(found, located{index: index})
		case errors.Is(err, errOldProtocol) && index != wiredIndex && hidraw.PairedChild(d.node.Phys):
			// A *paired device* answered, and answered that it has no
			// features at all. Its battery is a HID++ 1.0 register, which
			// read asks for instead.
			//
			// Two exclusions, and the real hardware taught both.
			//
			// Index 0xFF addresses the thing being spoken to, and a Unifying
			// receiver is a 1.0 device by construction with no battery of its
			// own -- reading it asked a receiver for a register it does not
			// have, and reporting it made every machine with one claim an
			// unreadable device two lines under a mouse that was drawing fine.
			//
			// And a receiver node answers for every device paired to it, so
			// what arrives there carries the receiver's name and not the
			// device's: the keyboard was reported twice, once correctly and
			// once as "Logitech USB Receiver".
			//
			// A device wired in by its own cable would answer on 0xFF and is
			// missed by this. None was to hand to check against, and a missed
			// reading is the better of the two mistakes.
			found = append(found, located{index: index, old: true, name: d.node.Name})
		case errors.Is(err, errOldProtocol) && index != wiredIndex && !d.childNodes:
			// The same paired device, on a system that gives it no node of
			// its own (Windows, spec 012). The receiver node is the only way
			// to it, so it is read here -- and the reason for the exclusion
			// above does not arise, because there is no child node to read it
			// twice. The receiver's name is still not the device's, so the
			// device's is asked of the receiver's pairing register.
			found = append(found, located{index: index, old: true, name: d.pairedName(ctx, index)})
		case errors.Is(err, errNotReachable):
			quiet++
		case errors.Is(err, sanshoku.ErrGone):
			return nil, err
		case ctx.Err() != nil:
			return nil, err
		}
	}
	d.presence = Presence{Nodes: 1, Quiet: quiet}
	return found, nil
}

// indices are the indices discover asks: a paired child's own, or on any
// other node 0xFF and every index a receiver can hold.
func (d *device) indices() []byte {
	if index, ok := hidraw.PairedIndex(d.node.Phys); ok {
		return []byte{index}
	}
	return append([]byte{wiredIndex}, deviceIndices...)
}

// names is a set of device names, in a stable order so a consumer's lines do
// not change places between two polls.
func names(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// read takes one device's battery, in whichever protocol it speaks.
//
// A device from before the feature protocol has no 0x0005 to ask its name of
// either, so it keeps the one the kernel gave its node.
func (d *device) read(ctx context.Context, loc located) (battery.Battery, error) {
	if loc.old {
		b, err := readOldBattery(ctx, d.rd, d.timeout, loc.index)
		if err != nil {
			return battery.Battery{}, err
		}
		b.Name = battery.Product(vendorWord, loc.name)
		if b.Name == "" {
			b.Name = defaultName
		}
		return b, nil
	}

	b, err := d.readFeature(ctx, loc.index, featureUnifiedBattery, 0x01, DecodeUnifiedBattery)
	if errors.Is(err, errUnknownFeature) {
		b, err = d.readFeature(ctx, loc.index, featureBatteryStatus, 0x00, DecodeBatteryStatus)
	}
	if err != nil {
		return battery.Battery{}, err
	}

	// The name the device gives is already the product's; Product is a no-op
	// on it and is applied anyway so no battery name skips the rule.
	var name string
	name, b.Kind = d.identify(ctx, loc.index)
	b.Name = battery.Product(vendorWord, name)
	return b, nil
}

// readFeature looks up a battery feature and reads it with one function.
func (d *device) readFeature(ctx context.Context, index byte, feature uint16, function byte,
	decode func([]byte) (battery.Battery, error)) (battery.Battery, error) {
	at, err := featureIndex(ctx, d.rd, d.timeout, index, feature)
	if err != nil {
		return battery.Battery{}, err
	}
	params, err := request(ctx, d.rd, d.timeout, index, at, function)
	if err != nil {
		return battery.Battery{}, err
	}
	return decode(params)
}

// identify asks the device what it is called and what it is.
//
// Both come from feature 0x0005, which is looked up once and used twice: the
// name is a label and the kind is where a consumer puts it, and neither is
// worth a second round trip to the receiver for.
func (d *device) identify(ctx context.Context, index byte) (string, battery.Kind) {
	feature, err := featureIndex(ctx, d.rd, d.timeout, index, featureDeviceName)
	if err != nil {
		return defaultName, battery.KindOther
	}
	return d.name(ctx, index, feature), d.kind(ctx, index, feature)
}

// featureDeviceName is feature 0x0005, which carries the name the device calls
// itself -- "G502 X PLUS" rather than "Logitech USB Receiver", which is all the
// kernel knows -- and, on its third function, what sort of device it is.
const featureDeviceName = 0x0005

// The functions of feature 0x0005: the name's length, a chunk of the name at an
// offset, and the device type.
const (
	functionDeviceName      = 0x00
	functionDeviceNameChunk = 0x01
	functionDeviceType      = 0x02
)

// The device types feature 0x0005 reports, as Logitech numbers them.
//
// Named rather than inline because the mapping in kind is the only thing in
// this module that has an opinion about what a trackball is, and a reader
// checking it against the protocol should not have to count the constants.
const (
	typeKeyboard  = 0x00
	typeNumpad    = 0x02
	typeMouse     = 0x03
	typeTouchpad  = 0x04
	typeTrackball = 0x05
)

// defaultName is what a device that will not say is called.
const defaultName = "Logitech"

// maxNameLength bounds the name a device may claim to have. The protocol hands
// over a length and then that many bytes in chunks, and a device answering
// with nonsense should cost one refused read rather than a loop.
const maxNameLength = 64

// name asks the device what it is called.
//
// A device that will not say is not a failure. The name is a label, and a
// reading labelled "Logitech" that carries the right number is better than no
// reading -- but it is *not* used to tell devices apart, which is why a blank
// one here does not stop the reading.
func (d *device) name(ctx context.Context, index, feature byte) string {
	params, err := request(ctx, d.rd, d.timeout, index, feature, functionDeviceName)
	if err != nil || len(params) < 1 {
		return defaultName
	}

	length := int(params[0])
	if length <= 0 || length > maxNameLength {
		return defaultName
	}

	out := make([]byte, 0, length)
	for len(out) < length {
		// The offset fits a byte because length is bounded by maxNameLength
		// above, which is well under one.
		offset := byte(len(out) & 0xFF)
		chunk, err := request(ctx, d.rd, d.timeout, index, feature, functionDeviceNameChunk, offset)
		if err != nil || len(chunk) == 0 {
			break
		}
		out = append(out, chunk...)
	}

	/*
		**A name has to arrive whole, and be a name all the way through.**

		The device declares its own length before sending any of it, so a reply
		that does not fill that length did not belong to this request -- and
		the printable-byte filter is happy to turn one stray byte into a
		plausible label. That is how a battery level of 81 became a peripheral
		called "Q" on hayami's panel, beside the mouse it had been read from:
		`chr(81)`, drawn as confidently as the real name next to it (hayami
		issue #58).

		Counting the bytes is not enough on its own, because a short reply
		arrives in a report padded with zeroes and a few of those make it look
		long enough. What a name cannot survive is a hole: the declared run has
		to be printable from end to end, which the real thing is and a stray
		byte followed by padding is not.

		defaultName is the honest answer. A device whose name will not read is
		still a battery worth reporting, and an unnamed one says so rather than
		inventing something that looks right.
	*/
	if len(out) < length {
		return defaultName
	}
	name, whole := printableName(out[:length])
	if !whole {
		return defaultName
	}
	// Trimmed only for display: the check above is against the declared run,
	// and a device whose name really does end in a space should keep its name.
	if trimmed := strings.TrimSpace(name); trimmed != "" {
		return trimmed
	}
	return defaultName
}

// kind asks the device what sort of device it is.
//
// A device that will not say is KindOther. This is a label like the name and
// not part of the reading: it is not worth failing a battery over, and a mouse
// whose type request was lost is still a mouse with a percentage.
func (d *device) kind(ctx context.Context, index, feature byte) battery.Kind {
	params, err := request(ctx, d.rd, d.timeout, index, feature, functionDeviceType)
	if err != nil || len(params) < 1 {
		return battery.KindOther
	}

	switch params[0] {
	case typeMouse, typeTouchpad, typeTrackball:
		return battery.KindMouse
	case typeKeyboard, typeNumpad:
		return battery.KindKeyboard
	default:
		// A remote, a presenter, the receiver itself, and anything Logitech
		// has numbered since. None of them is a kind this module names.
		return battery.KindOther
	}
}

// printableName keeps a name's printable bytes and reports whether *every*
// byte was one.
//
// The two answers are separate because they are asked for different reasons.
// A control character is dropped rather than drawn, since one in a label would
// move the column it sits in. But a hole in the declared run is the mark of a
// reply that did not belong to this request, and only the caller comparing
// against the declared length can see that -- so the fact is handed back
// rather than quietly repaired.
func printableName(b []byte) (string, bool) {
	out := make([]rune, 0, len(b))
	for _, c := range b {
		if c >= 0x20 && c < 0x7F {
			out = append(out, rune(c))
		}
	}
	return string(out), len(out) == len(b)
}
