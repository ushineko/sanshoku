package nzxt

import (
	"context"
	"encoding/binary"
	"fmt"
	"image/gif"
	"os"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/screen"
)

/*
The cooler's screen.

Images live in *buckets*: sixteen slots in about 24 MB of device memory. Putting
a picture up means finding a free slot, telling the device where the data will
go, streaming it over the bulk endpoint, and then pointing the screen at the
slot. hotaru's spec 012 records how this was read off liquidctl and what it
cost.

**A transfer the device fully accepts can display nothing.** Every step reports
success and the screen keeps whatever it was showing, because a freshly written
bucket is not displayed until it is selected. That is the ordinary shape of
this protocol, and the reason showing is a separate step.

Static images are not offered: the firmware drops one after five to ten
seconds (measured by peripheral-battery-monitor with nothing else touching the
device), while a GIF plays indefinitely. The RGB565 framebuffer path is left
out as well; hotaru's prototype of it flickered. So are fan and pump duty
writes, which the firmware discards.
*/
const (
	// bulkEndpoint carries image data. The HID interface carries control.
	bulkEndpoint = 0x02
	// bulkInterface is the vendor-defined interface the endpoint sits on.
	bulkInterface = 0

	buckets   = 16
	packetLen = 1024

	/*
		maxImage is the largest picture the screen will hold.

		The device has about 24 MB for images, in packets of a kilobyte. An
		image beyond it is a caller's mistake rather than a transfer to try,
		and checking here is what lets the sizes below be converted safely.
	*/
	maxImage = 24 * 1000 * 1024

	// modeBucket shows a stored image; modeLiquid hands the screen back to
	// the firmware's own coolant readout.
	modeBucket = 0x04
	modeLiquid = 0x02

	// The result byte of a reply, and the values it takes.
	resultByte    = 14
	resultOK      = 0x01
	resultRefused = 0x04 // out of sequence, the slot is occupied, or an address the device did not choose
	resultStuck   = 0x09 // the slot would not clear; try the next one
)

/*
capacity is the device's image memory, counted in packets.

liquidctl's number, and the placement arithmetic is in the same units as the
sizes the device reports.
*/
const capacity = 24320

// header is the twelve bytes every bulk transfer starts with, followed by
// eight describing the transfer itself.
var header = []byte{0x12, 0xFA, 0x01, 0xE8, 0xAB, 0xCD, 0xEF, 0x98, 0x76, 0x54, 0x32, 0x10}

/*
panelDevice is a cooler whose model has a panel and whose USB device was found.

It is a separate type so that a device without one does not satisfy
screen.Panel: a consumer asks with a type assertion, and a cooler whose panel
could never be claimed should say so there rather than at the first draw.
*/
type panelDevice struct {
	*device
}

var _ screen.Panel = (*panelDevice)(nil)

// Size is the model's panel size.
func (p *panelDevice) Size() (w, h int) { return p.model.Screen.W, p.model.Screen.H }

/*
Floor is the shortest interval at which a frame of a given encoded size
appears on this panel.

Measured by hotaru on a Kraken Elite by pushing a frame repeatedly and watching
an indicator that advances on every update:

	frame                         1 s      1.5 s   2 s     3 s
	5 KB, seven-segment digits    lands
	8 KB, flat background         never    skips   lands
	21 KB, full starfield                          lands   lands

It is a settling time that scales with frame size, not a size limit. The device
accepts every transfer either way (the HID exchange succeeds, the bulk write
completes, the bucket switch returns success) and the screen simply does not
change. Nothing in the protocol announces any of this. Another cooler will have
its own floor, and pushing too fast is invisible: every write reports success
and the panel quietly stops updating.
*/
func (p *panelDevice) Floor(bytes int) time.Duration {
	switch {
	case bytes <= 6*1024:
		return time.Second
	case bytes <= 24*1024:
		return 2 * time.Second
	}
	// Beyond anything measured. Extrapolating downwards would be a guess that
	// looks like a fact on a screen nobody is watching closely.
	return 3 * time.Second
}

/*
Image puts a GIF on the screen and shows it, then deletes the slot it replaced.

A GIF of the wrong size transfers successfully, switches buckets successfully
and leaves the screen blank, so it is fitted to the panel here, where every
path that puts a picture on the panel goes through.
*/
func (p *panelDevice) Image(ctx context.Context, g *gif.GIF) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.panel(); err != nil {
		return err
	}
	data, err := fit(g, p.model.Screen)
	if err != nil {
		return err
	}
	bucket, err := p.send(ctx, data)
	if err != nil {
		return err
	}
	if err := p.show(ctx, bucket, modeBucket); err != nil {
		return err
	}
	/*
		The old picture is released only once the new one is up.

		This is double buffering, and on this panel it is not an optimisation.
		The transfer takes about a second, during which the device keeps
		showing whatever slot it was pointed at; write over that slot and the
		screen is blank for the duration. Left to itself the placement walks
		through all sixteen slots, fills them, and then starts deleting the
		one on screen on every update, which looks like a panel blanking at
		random.

		A failed delete is not an error. The slot stays occupied, placement
		skips it, and the only cost is memory the next wrap reclaims.
	*/
	if previous := p.showing; previous >= 0 && previous != bucket {
		_, _ = p.empty(ctx, previous)
	}
	p.showing = bucket
	return nil
}

/*
Readout hands the screen back to the cooler's own display: the recovery, and
what a program leaves behind when it stops.
*/
func (p *panelDevice) Readout(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.panel(); err != nil {
		return err
	}
	return p.liquid(ctx)
}

/*
Appearance sets brightness as a percentage and orientation in degrees, which
the device keeps across restarts.

One command carries both, so each is sent with the other's value; there is no
way to set one without stating the other. The device acknowledges it with
nothing, so it is written and not asked.
*/
func (p *panelDevice) Appearance(_ context.Context, brightness, degrees int) error {
	if brightness < 0 || brightness > 100 {
		return fmt.Errorf("brightness %d is outside 0-100", brightness)
	}
	switch degrees {
	case 0, 90, 180, 270:
	default:
		return fmt.Errorf("orientation %d is not 0, 90, 180 or 270", degrees)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.panel(); err != nil {
		return err
	}
	//nolint:gosec // both values are range-checked above
	return p.tell(0x30, 0x02, 0x01, byte(brightness), 0x00, 0x00, 0x01, byte(degrees/90))
}

/*
panel claims the panel's interface on first use and keeps it. The caller holds
mu.

Every screen command travels the same control channel as a status reading
(only the pixels go over the bulk endpoint), so the device holds both under one
lock. Claiming lazily means a program that never draws anything never claims
the interface, leaving it to whatever else wants it.

A panel that will not be claimed is screen.ErrNoPanel wrapping
sanshoku.ErrAbsent, reported as an absent screen rather than a failure: from
every caller's side it is a machine without one, and the telemetry still works.
*/
func (p *panelDevice) panel() error {
	if p.hid == nil {
		return fmt.Errorf("drawing on %s: %w", p.id.Path, os.ErrClosed)
	}
	if p.usb != nil {
		return nil
	}
	u, err := p.claim(p.usbPath)
	if err != nil {
		return fmt.Errorf("%w: %w: %w", screen.ErrNoPanel, sanshoku.ErrAbsent, err)
	}
	p.usb = u
	return nil
}

/*
liquid points the screen at the firmware's readout. The caller holds mu.

Nothing of this driver's is on screen afterwards, so every slot is reusable.
*/
func (d *device) liquid(ctx context.Context) error {
	if err := d.show(ctx, 0, modeLiquid); err != nil {
		return err
	}
	d.showing = -1
	return nil
}

/*
show points the screen at a bucket, or at the firmware's readout.

The step that is easy to leave out and impossible to notice leaving out: every
part of a transfer reports success without it, and the screen keeps whatever it
was showing.
*/
func (d *device) show(ctx context.Context, bucket int, mode byte) error {
	if bucket < 0 || bucket >= buckets {
		return fmt.Errorf("slot %d is outside 0-%d", bucket, buckets-1)
	}
	reply, err := d.exchange(ctx, 0x38, 0x01, mode, byte(bucket))
	if err != nil {
		return fmt.Errorf("show slot %d: %w", bucket, err)
	}
	if reply[resultByte] != resultOK {
		return fmt.Errorf("the cooler refused to show slot %d: reply %#02x", bucket, reply[resultByte])
	}
	return nil
}

/*
slots asks the device about every bucket.

The replies carry where each image sits and how much room it has, which is what
placement needs. The device owns that map: an address the driver picks for
itself is refused, which is what `04` means and how hotaru got this wrong.

	slot 0:  index=00 asset=01 .. addr=0000 size=0007 used=01
	slot 1:  index=01 asset=02 .. addr=0016 size=0016 used=01
*/
func (d *device) slots(ctx context.Context) ([][]byte, error) {
	out := make([][]byte, buckets)
	for i := range buckets {
		reply, err := d.exchange(ctx, 0x30, 0x04, byte(i))
		if err != nil {
			return nil, fmt.Errorf("ask about slot %d: %w", i, err)
		}
		out[i] = reply
	}
	return out, nil
}

// vacant is a bucket with nothing in it: everything from byte 15 is zero.
func vacant(reply []byte) bool {
	for _, b := range reply[15:] {
		if b != 0 {
			return false
		}
	}
	return true
}

/*
free is the first bucket holding nothing, or -1 when they are all taken.

The slot on screen is never free, whatever it holds: writing into it is what
blanks the panel. Pass -1 when nothing of this driver's is displayed.
*/
func free(slots [][]byte, showing int) int {
	for i, reply := range slots {
		if i != showing && vacant(reply) {
			return i
		}
	}
	return -1
}

func u16(b []byte) int { return int(b[0]) | int(b[1])<<8 }

/*
place works out where in the device's memory an image may go.

Ported from liquidctl, because the device refuses an address it did not arrive
at itself. In order: reuse the slot's own space if the image still fits; keep
its address if nothing else has grown into it; otherwise go after everything
else; otherwise take the room at the start. Failing all of that the memory is
too fragmented to place anything, and the caller clears it and begins again.

The arithmetic looks fussy and is not optional: addresses move between writes,
so they are read back from the device every time rather than remembered.
*/
func place(slots [][]byte, bucket, packets int) (int, bool) {
	current := slots[bucket]
	offset, size := u16(current[17:19]), u16(current[19:21])

	if packets <= size {
		return offset, true
	}

	lowest, highest, overlaps := offset, 0, false
	for i, reply := range slots {
		start := u16(reply[17:19])
		end := start + u16(reply[19:21])
		if end > highest {
			highest = end
		}
		if start < lowest {
			lowest = start
		}
		if (start > offset && start < offset+packets) ||
			(start < offset && end > start) ||
			(start == offset && i != bucket) {
			overlaps = true
		}
	}
	switch {
	case !overlaps:
		return offset, true
	case highest+packets < capacity:
		return highest, true
	case packets < lowest:
		return 0, true
	}
	return 0, false
}

/*
empty clears one slot, and says whether the device agreed.

The result matters. A slot can refuse to clear (it answers `09`), and a setup on
a slot that still holds something is refused with `04`. Deleting and carrying on
regardless produces exactly that, which is what hotaru saw:

	32 02 delete slot 0    result[14]=0x09   the delete failed
	30 04 query slot 0     still occupied
	32 01 setup            result[14]=0x04   refused
*/
func (d *device) empty(ctx context.Context, bucket int) (bool, error) {
	reply, err := d.exchange(ctx, 0x32, 0x02, byte(bucket)) //nolint:gosec // callers pass a slot index, checked by prepare
	if err != nil {
		return false, fmt.Errorf("clear slot %d: %w", bucket, err)
	}
	return reply[resultByte] == resultOK, nil
}

/*
prepare finds a slot that will actually take an image.

liquidctl's logic, and the recursion is the point: a slot that refuses to clear
is skipped rather than insisted upon, and a slot that held data is cleared
twice. Neither is guessable from the protocol; both were read off a working
implementation, and leaving either out fails in a way that looks like the
device rejecting a good address.
*/
func (d *device) prepare(ctx context.Context, bucket int, filled bool) (int, error) {
	if bucket >= buckets {
		return 0, fmt.Errorf("every slot on the screen refused to clear (%#02x)", resultStuck)
	}
	if bucket == d.showing {
		// Clearing it would blank the panel for the length of the transfer.
		return d.prepare(ctx, bucket+1, filled)
	}
	ok, err := d.empty(ctx, bucket)
	if err != nil {
		return 0, err
	}
	if !ok {
		return d.prepare(ctx, bucket+1, true)
	}
	if filled {
		return d.prepare(ctx, bucket, false)
	}
	return bucket, nil
}

/*
send streams an image into a bucket and returns which.

The payload is padded to the packet count the device was told to expect. The
transfer is described in whole 1024-byte packets and an image rarely divides
evenly, so sending only the image leaves the tail of the last packet holding
whatever was in that memory, which the panel draws as a band of noise along the
bottom.
*/
func (d *device) send(ctx context.Context, image []byte) (int, error) {
	if len(image) > maxImage {
		return 0, fmt.Errorf("an image of %d bytes is larger than the screen's memory", len(image))
	}

	/*
		The order is the protocol, and it is not obvious.

		`36 03` opens the exchange and everything else follows it, including
		asking which slots are free. Choosing a slot first and opening the
		exchange afterwards is refused with `04`, which is the device saying a
		setup arrived out of sequence rather than anything about the slot.
	*/
	if _, err := d.exchange(ctx, 0x36, 0x03); err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	slots, err := d.slots(ctx)
	if err != nil {
		return 0, err
	}
	start, filled := free(slots, d.showing), false
	if start < 0 {
		// Every slot is taken, so one has to be reused: any of them except
		// the one being displayed.
		start, filled = 0, true
		if start == d.showing {
			start = 1
		}
	}
	bucket, err := d.prepare(ctx, start, filled)
	if err != nil {
		return 0, err
	}

	info := make([]byte, 8)
	info[0] = 0x01                                              // a GIF
	binary.LittleEndian.PutUint32(info[4:], uint32(len(image))) //nolint:gosec // bounded above
	preamble := append(append([]byte(nil), header...), info...)

	packets := (len(preamble) + len(image) + packetLen - 1) / packetLen
	offset, ok := place(slots, bucket, packets)
	if !ok {
		// Too fragmented to fit anything. Clearing every slot is the only
		// way back, and is what liquidctl does.
		if err := d.clear(ctx); err != nil {
			return 0, err
		}
		bucket, offset = 0, 0
	}

	var address, size [2]byte
	binary.LittleEndian.PutUint16(address[:], uint16(offset)) //nolint:gosec // bounded by capacity
	binary.LittleEndian.PutUint16(size[:], uint16(packets))   //nolint:gosec // bounded by maxImage

	//nolint:gosec // bucket is below buckets, checked by prepare
	reply, err := d.exchange(ctx, 0x32, 0x01, byte(bucket), byte(bucket+1),
		address[0], address[1], size[0], size[1], 0x01)
	if err != nil {
		return 0, fmt.Errorf("reserve slot %d: %w", bucket, err)
	}
	if reply[resultByte] != resultOK {
		return 0, fmt.Errorf("the cooler refused slot %d: reply %#02x", bucket, reply[resultByte])
	}

	if _, err := d.exchange(ctx, 0x36, 0x01, byte(bucket)); err != nil { //nolint:gosec // as above
		return 0, fmt.Errorf("start the transfer: %w", err)
	}
	if err := d.usb.Bulk(ctx, bulkEndpoint, preamble); err != nil {
		return 0, err
	}
	padded := make([]byte, packets*packetLen-len(preamble))
	copy(padded, image)
	if err := d.usb.Bulk(ctx, bulkEndpoint, padded); err != nil {
		return 0, err
	}
	if _, err := d.exchange(ctx, 0x36, 0x02); err != nil {
		return 0, fmt.Errorf("finish the transfer: %w", err)
	}
	return bucket, nil
}

/*
clear empties every slot and hands the screen back to the firmware first.

The last resort when the device's memory is too fragmented to place an image.
Switching to the readout before clearing matters: deleting the slot that is on
screen is the one operation with a visible consequence.
*/
func (d *device) clear(ctx context.Context) error {
	if err := d.liquid(ctx); err != nil {
		return err
	}
	for i := range buckets {
		if _, err := d.empty(ctx, i); err != nil {
			return err
		}
	}
	return nil
}
