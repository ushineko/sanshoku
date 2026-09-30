package steelseries

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

/*
SteelSeries batteries, over the vendor's own HID protocol.

**The command is rivalcfg's, and it is not documented anywhere else.** Every
wireless mouse profile in that project carries `0x92` with a two-byte reply --
bit 7 of the value is charging, the rest is a level in steps of five -- and its
wireless variants OR `0x40` into the command byte so the same getter reaches a
device behind a dongle. Nothing published says this works on a keyboard. It
does: the Apex Pro TKL Wireless Gen 3 answers both forms, which is how hayami's
spec 016 read a battery that HeadsetControl, the kernel, OpenRGB, Solaar and
SDL between them have no protocol for.

So everything here is inferred from one device on one machine. A reply that is
not the shape this expects yields no reading rather than a guessed one.
*/

// driverName is the driver's name, as Name returns it and the support table keys it.
const driverName = "steelseries"

// steelseriesVendor is SteelSeries' USB vendor ID.
const steelseriesVendor = 0x1038

// vendorWord is how SteelSeries writes its own name at the front of a device's, and
// what battery.Product takes off a battery's name so the product is what is
// left to read.
const vendorWord = "SteelSeries"

// The SteelSeries vendor protocol, as hayami measured it on the Apex.
const (
	// controlPage is the usage page of the control endpoint. A keyboard
	// presents six interfaces and this is the only one that answers.
	controlPage = 0xFFC0

	// batteryCommand asks for the battery. wirelessFlag makes it the same
	// getter addressed through a dongle: rivalcfg's `_WIRELESS_FLAG`, which
	// is also exactly the offset between the two aliased command banks the
	// device exposes.
	batteryCommand = 0x92
	wirelessFlag   = 0x40

	// reportSize is the output and input report width. There is no report ID
	// in this descriptor, so a write carries a leading zero the kernel strips
	// and a read does not.
	reportSize = 64
)

/*
The Arctis Nova Pro Wireless base station's battery report, from HeadsetControl
4.0.0 (lib/devices/steelseries_arctis_nova_pro_wireless.hpp) and a probe of the
base station 1038:12e5 on 2026-09-29.

The base station's 0xFFC0 collection numbers its reports: its descriptor
declares report 0x06 with 63 bytes of input and 63 of output, so the report ID
is the first byte on hidraw both ways and an input report is 64 bytes with it.
*/
const (
	// novaProReport and novaProBattery are the request, `06 b0`, and the
	// first two bytes of its reply, which echoes them.
	novaProReport  = 0x06
	novaProBattery = 0xB0

	// novaProRequestSize is HeadsetControl's PACKET_SIZE_31: the request is
	// 31 bytes, zero after the two that say what it is.
	novaProRequestSize = 31

	// novaProReplySize is the input report with its ID: 1 + 63 bytes.
	novaProReplySize = 64
)

// defaultTimeout is how long the device has to answer one command. Short: a
// consumer polls on a timer, and a keyboard that is not going to reply does
// not start (hayami, SteelSeriesTimeout).
const defaultTimeout = 300 * time.Millisecond

/*
protocol is which battery command a SteelSeries device answers.

There is more than one and they are not compatible, so this is a fact about a
model rather than about the vendor.
*/
type protocol int

const (
	// unknown is a device this module has never been told about. It is the
	// default on purpose: a product that is not named below is not written to
	// at all.
	unknown protocol = iota

	// modern is command 0x92 with a two-byte reply: bit 7 charging and the
	// rest a level in steps of five.
	modern

	// novaPro is the Arctis Nova Pro Wireless base station's `06 b0`: one
	// report each way, the headset's level on a 0-8 scale and its status.
	novaPro

	// legacy is command 0xAA 0x01 with a three-byte reply. rivalcfg names the
	// devices that speak it; this module cannot read it.
	//
	// **Named but not implemented, on purpose.** Its framing differs from the
	// newer family in more than the command byte -- rivalcfg reads the level
	// out of the first byte of the reply, so there is no command echo to match
	// an answer on, which is the whole of how the newer family tells its reply
	// from the other traffic on that endpoint. Writing that from documentation
	// alone would be shipping a guess about framing to hardware nobody here
	// has. It is in the table so those devices are told apart from ones this
	// module has never heard of.
	legacy
)

/*
products says which command each known product answers.

**A product not in here is never written to.** Finding a node by vendor and
usage page is broad -- broad enough that the Arctis Nova Pro Wireless on
hayami's development machine matched it and was being sent `0x92` on every
poll, a command from a family it does not speak (hayami issue #62). Finding a
device and being entitled to talk to it are separate questions and this is the
second one. The Nova Pro is in the table now, as its own family, and is sent
only its own command.

The Apex appears twice because its product ID moves with its connection, and
both were measured. The keyboards and mice are from rivalcfg's device profiles,
which is the only place their protocol is written down at all; the headset
family is from HeadsetControl's, and speaks something else entirely.
*/
var products = map[uint16]protocol{
	// Measured by hayami, spec 016: 2.4 GHz and cable.
	0x1644: modern, // Apex Pro TKL Wireless Gen 3
	0x1646: modern, // the same keyboard, wired

	// rivalcfg's 0x92 family. Each mouse has a product for each connection.
	0x1838: modern, // Aerox 3 Wireless
	0x183A: modern, // Aerox 3 Wireless, wired
	0x1852: modern, // Aerox 5 Wireless
	0x1854: modern, // Aerox 5 Wireless, wired
	0x1858: modern, // Aerox 9 Wireless
	0x185A: modern, // Aerox 9 Wireless, wired
	0x1840: modern, // Prime Wireless
	0x1842: modern, // Prime Wireless, wired

	// HeadsetControl's Nova Pro Wireless family, spec 006: the base
	// station, not the headset, is the USB device and answers for it.
	0x12E0: novaPro, // Arctis Nova Pro Wireless base station, by protocol
	0x12E5: novaPro, // Arctis Nova Pro Wireless X base station, measured

	// rivalcfg's 0xAA family. Unverified: no hardware here speaks it, which is
	// exactly why the table exists -- it cannot reach anything else.
	0x1830: legacy, // Rival 3 Wireless
	0x1872: legacy, // Rival 3 Wireless Gen 2
	0x172B: legacy, // Rival 650 Wireless
}

/*
Driver reads SteelSeries batteries over hidraw.

The zero value is ready to use. Timeout bounds one command; zero means 300 ms.
*/
type Driver struct {
	Timeout time.Duration
}

// Name is "steelseries".
func (Driver) Name() string { return driverName }

/*
Find returns one candidate per SteelSeries hidraw node whose descriptor
declares usage page 0xFFC0. ErrAbsent when there is none.

Every such node is a candidate, including a product the allow-list does not
name, so the bench and a consumer can see that it was found. Opening one of
those returns sanshoku.ErrUnsupported with the kernel's name for it, and it is
never written to; so is a product of the legacy family, which is listed and
not implemented. Find opens nothing.
*/
func (d Driver) Find(context.Context) ([]sanshoku.Candidate, error) {
	nodes, err := hidraw.Nodes(steelseriesVendor, hidraw.UsagePage(controlPage))
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

// node is an open control endpoint: a *hidraw.Handle, or a test's stand-in.
type node interface {
	hidraw.ReportDevice
	io.Closer
	// Drain discards reports that arrived before the question is asked. See
	// drain.
	Drain(depth int)
}

// openNode opens a real control endpoint.
func openNode(path string) (node, error) {
	return hidraw.Open(path)
}

/*
candidate is one node as a candidate, opening through open.

The allow-list is consulted here, before anything is opened: a product this
module does not know, or knows and cannot read, is refused by name without its
node being opened, let alone written to.
*/
func candidate(n hidraw.Node, timeout time.Duration, open func(string) (node, error)) sanshoku.Candidate {
	id := identity(n)
	return sanshoku.Candidate{
		Identity: id,
		Driver:   driverName,
		Open: func(context.Context) (sanshoku.Device, error) {
			switch products[n.Product] {
			case unknown:
				return nil, fmt.Errorf("%s: not a product this driver speaks to: %w", id, sanshoku.ErrUnsupported)
			case legacy:
				return nil, fmt.Errorf("%s: rivalcfg's 0xAA battery protocol is not implemented: %w", id, sanshoku.ErrUnsupported)
			case modern, novaPro:
			}
			nd, err := open(n.Path)
			if err != nil {
				return nil, err
			}
			return &device{id: id, family: products[n.Product], timeout: timeout, rd: nd}, nil
		},
	}
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

// device is one open SteelSeries control endpoint of an allow-listed product.
type device struct {
	id sanshoku.Identity
	// family is which protocol the product speaks: modern or novaPro.
	family  protocol
	timeout time.Duration

	mu sync.Mutex
	// rd is the open node. Nil once closed.
	rd node
}

// Identity is the node's.
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
	return err //nolint:wrapcheck // hidraw.Handle.Close names the node already
}

/*
Batteries reads the device's battery. A headset base station is asked the one
question its family answers (see headset); a keyboard or mouse as follows.

Both command forms are tried, wired first, because the device does not say
which it wants: the product ID moves with the connection -- 0x1644 on 2.4 GHz
and 0x1646 on the cable -- and asking twice is cheaper than tracking that and
being wrong.

A device that answers neither is no reading and no error, rather than a level
of zero. A node that has been unplugged is sanshoku.ErrGone.
*/
func (d *device) Batteries(ctx context.Context) ([]battery.Battery, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rd == nil {
		return nil, fmt.Errorf("reading %s: %w", d.id.Path, os.ErrClosed)
	}
	if d.family == novaPro {
		return d.headset(ctx)
	}

	for _, cmd := range []byte{batteryCommand, batteryCommand | wirelessFlag} {
		reply, err := d.ask(ctx, cmd)
		if err != nil {
			if errors.Is(err, sanshoku.ErrGone) || ctx.Err() != nil {
				return nil, err
			}
			continue
		}
		b, err := DecodeModern(reply)
		if err != nil {
			continue
		}
		b.Name = battery.Product(vendorWord, d.id.Name)
		// KindOther, and deliberately not a guess. The control endpoint says
		// nothing about what the device is, and the interfaces beside it are
		// ambiguous in both directions: the Apex presents a mouse interface
		// for its media controls, and plenty of mice present a keyboard one
		// for their macro buttons. Reading either as the answer gets the
		// other wrong, and the only thing it costs to admit that is where the
		// reading sorts.
		b.Kind = battery.KindOther
		return []battery.Battery{b}, nil
	}
	return nil, nil
}

/*
ask writes one command and waits for the reply that echoes it.

**The echo is the whole of the addressing.** This endpoint carries unsolicited
traffic and late answers to earlier questions, and a reader that took the next
packet as its own would attribute one command's reply to another -- which
hayami did during its spec 016 investigation, where a late `d2 14` was read as
noise from a different command and the battery was missed for it.
*/
func (d *device) ask(ctx context.Context, cmd byte) ([]byte, error) {
	actx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	req := make([]byte, reportSize+1) // a leading report number of zero
	req[1] = cmd
	matches := func(r []byte) bool { return len(r) > 1 && r[0] == cmd }
	d.rd.Drain(hidraw.QueueDepth)
	reply, err := hidraw.Exchange(actx, d.rd, req, matches, reportSize)
	if err != nil {
		return nil, fmt.Errorf("asking a SteelSeries device for %#02x: %w", cmd, err)
	}
	return reply, nil
}

/*
headset reads the Arctis Nova Pro Wireless through its base station.

One command, and it is the only one this driver sends the base station. The
others in HeadsetControl's profile -- sidetone, lights, inactivity, equaliser --
write the headset's settings, and a battery reader has no business there.

The base station is on USB and does not sleep, so a reply that does not come,
or does not decode, is an error rather than silence, as it is in
HeadsetControl. A headset that is switched off is neither: the base station
answers and says so, and that is a reading without a level.
*/
func (d *device) headset(ctx context.Context) ([]battery.Battery, error) {
	actx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	req := make([]byte, novaProRequestSize)
	req[0], req[1] = novaProReport, novaProBattery
	// The echo is the match, as on the keyboards: the same node carries
	// report 0x07 from the base station's second collection, and a reply
	// that is not this one's is not read as it.
	matches := func(r []byte) bool { return len(r) > 1 && r[0] == novaProReport && r[1] == novaProBattery }
	/*
		Drain first. hidraw hands every open handle a copy of every input
		report, including the base station's replies to other programs'
		questions, so a handle held across polls accumulates `06 b0` reports
		faster than one read a poll consumes them, and a read that took the
		oldest answered with the state of the headset when the handle was
		opened: a panel opened with the headset off said "off" four minutes
		after it was switched on, while a fresh process read 75%. The same
		rule the Kraken driver and liquidctl's clear_enqueued_reports follow.
	*/
	d.rd.Drain(hidraw.QueueDepth)
	reply, err := hidraw.Exchange(actx, d.rd, req, matches, novaProReplySize)
	if err != nil {
		return nil, fmt.Errorf("asking a SteelSeries base station for its headset's battery: %w", err)
	}
	b, err := DecodeNovaPro(reply)
	if err != nil {
		return nil, err
	}
	b.Name = battery.Product(vendorWord, d.id.Name)
	return []battery.Battery{b}, nil
}
