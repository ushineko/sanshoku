package nzxt

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ushineko/sanshoku/hidraw"
	"github.com/ushineko/sanshoku/lighting"
)

/*
The cooler's lighting: the pump ring and the fans chained to it, drawn as a
lighting.Canvas (spec 024).

The protocol is in docs/kraken-lighting.md, and every number here was measured
on the bench (spec 024, Verification). Each channel has one LED buffer and a
mode. A frame is a live write, `26 14`, which draws over whatever the mode
shows and changes neither. Committing writes the buffer with `26 04` and shows
it with `26 06`, which makes the colours the channel's own.

**The screen's readout takes a live frame away.** Switching the screen to the
firmware readout re-applies each channel's mode 150 to 300 ms later, and the
ring shows the buffer instead of the frame. So the driver commits the held
frame before every switch to the readout it makes, and on Release.
*/

const (
	// lightReportLen is every lighting report: the length the report
	// descriptor declares. A frame in 64 bytes lights only LEDs 1 to 20.
	lightReportLen = 512

	// channels is how many lighting channels the cooler has: the ring on 1,
	// the fans on 2.
	channels = 2

	/*
		slotsPer is the accessory slots per channel in the `21 03` reply, and
		slotsAt where channel 1's start. Channel n's are at
		slotsAt + slotsPer*n.
	*/
	slotsPer = 6
	slotsAt  = 0x0F

	/*
		accessoryWait bounds the wait for the accessory list. The cooler
		answers in 577 to 762 ms, behind every other report on the node; the
		reply is found by its prefix, never by counting reports (hotaru #206).
	*/
	accessoryWait = 2 * time.Second

	// frameWait bounds the wait for a frame's acknowledgement, which comes in
	// 1.3 to 9 ms, and 87 ms behind a screen upload.
	frameWait = 500 * time.Millisecond

	// commitWait bounds the wait for each half of a commit: `26 04` takes
	// about 335 ms, and `26 06` up to 120 ms over a live frame.
	commitWait = time.Second

	/*
		frameFloor is the shortest interval at which frames are sent.

		Every period from 100 ms down to 16 ms ran even on the bench with no
		frame lost, each frame waiting for its acknowledgement (spec 024 E2).
	*/
	frameFloor = 16 * time.Millisecond
)

/*
accessories is each known accessory id's LED count, from OpenRGB's Hue 2
driver (docs/kraken-lighting.md). An id not in here has an unknown count, and
its channel gets no keys rather than a guess.
*/
var accessories = map[byte]int{
	0x01: 10, 0x02: 8, 0x04: 10, 0x05: 8, 0x06: 6, 0x08: 14, 0x09: 15, 0x0A: 10,
	0x0B: 8, 0x0C: 8, 0x10: 8, 0x11: 1, 0x13: 18, 0x14: 18, 0x15: 20, 0x16: 20,
	0x17: 8, 0x18: 8, 0x19: 8, 0x1C: 16, 0x1D: 24, 0x1E: 24, 0x1F: 24,
}

// rings are the accessories that are a pump's ring, named "ring n".
var rings = map[byte]bool{0x10: true, 0x1E: true}

// fans are the accessories that are fans, named "fan n".
var fans = map[byte]bool{
	0x02: true, 0x0B: true, 0x0C: true, 0x13: true, 0x14: true, 0x15: true, 0x16: true,
	0x17: true, 0x18: true, 0x19: true, 0x1C: true, 0x1D: true, 0x1F: true,
}

// Both the cooler with a panel and the one without are a canvas, with zones.
var (
	_ lighting.Canvas = (*device)(nil)
	_ lighting.Canvas = (*panelDevice)(nil)
	_ lighting.Zoned  = (*device)(nil)
	_ lighting.Zoned  = (*panelDevice)(nil)
)

/*
Zones is one zone per channel with keys, named as consumers name them (hotaru
spec 073): "Ring" for the pump's ring, "Fan" for the fans, "Channel n" for
anything else. Each is a ring, and its parts are the accessories in its slots:
three fans of eight are parts 8, 8 and 8 (spec 032).
*/
func (d *device) Zones() []lighting.Zone {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.hid == nil || d.knowChannels(context.Background()) != nil {
		return nil
	}
	var out []lighting.Zone
	first := 0
	for i, c := range d.canvas.ch {
		if len(c.held) == 0 {
			continue
		}
		name := fmt.Sprintf("Channel %d", i+1)
		if c.name != "" {
			name = strings.ToUpper(c.name[:1]) + c.name[1:]
		}
		out = append(out, lighting.Zone{
			Name: name, First: first, Count: len(c.held),
			Shape: lighting.ShapeRing, Parts: append([]int(nil), c.parts...),
		})
		first += len(c.held)
	}
	return out
}

// Resize refuses: the cooler counts its accessories' lights itself.
func (d *device) Resize(zone string, _ int) error {
	return fmt.Errorf("%q on %s: %w", zone, d.id, lighting.ErrNoZone)
}

// colour is one LED's colour.
type colour struct{ r, g, b uint8 }

/*
channel is one lighting channel as the canvas holds it.

held is what the canvas shows, sent what the cooler last acknowledged as a
frame, and committed what the buffer holds from this handle. drawn is whether a
frame has named the channel since the last Release: a channel never drawn is
never sent, so a consumer drawing on the ring leaves the fans alone.
*/
type channel struct {
	name string
	// parts are the LEDs of each accessory in the channel's slots, in order.
	parts     []int
	held      []colour
	sent      []colour
	committed []colour
	drawn     bool
}

/*
canvasState is the canvas on one handle: the channels, once the accessory list
has been read. The zero value has not read it.
*/
type canvasState struct {
	known bool
	ch    [channels]channel
}

/*
Keys lists one key per LED: channel 1 first, then channel 2, each in the
channel's order, numbered from 0. A channel holding an accessory whose LED count
is not known has no keys.

The list is read from the cooler the first time it is asked for, and kept for
the life of the handle. A cooler that does not answer within two seconds gives
no keys, and is asked again next time.
*/
func (d *device) Keys() []lighting.Key {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.hid == nil {
		return nil
	}
	if err := d.knowChannels(context.Background()); err != nil {
		return nil
	}
	return d.canvas.keys()
}

// Floor is the shortest interval at which frames are sent: 16 ms.
func (d *device) Floor() time.Duration { return frameFloor }

/*
Frame draws px over the held frame and sends each channel that changed.

A channel is one `26 14` report with all its LEDs, and the next is not sent
until the cooler acknowledges the last: without the wait the firmware drops
frames it cannot keep up with. A key that is not in Keys is an error and
nothing is sent. A frame that goes unacknowledged is hidraw.ErrSilent; the
colours stay held, so the next Frame sends them again.
*/
func (d *device) Frame(ctx context.Context, px []lighting.Pixel) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.hid == nil {
		return fmt.Errorf("lighting %s: %w", d.id.Path, os.ErrClosed)
	}
	if err := d.knowChannels(ctx); err != nil {
		return fmt.Errorf("a frame to %s: %w", d.id, err)
	}
	if err := d.canvas.merge(px); err != nil {
		return fmt.Errorf("a frame to %s: %w", d.id, err)
	}
	for i := range d.canvas.ch {
		c := &d.canvas.ch[i]
		if !c.drawn || same(c.held, c.sent) {
			continue
		}
		if err := d.acked(ctx, frameWait, liveReport(i, c.held)); err != nil {
			return fmt.Errorf("a frame to %s: %w", d.id, err)
		}
		c.sent = append(c.sent[:0], c.held...)
	}
	return nil
}

/*
Release commits the held frame and forgets it.

Each channel the canvas has drawn gets its colours written to its buffer and
shown, so they are the channel's own: the screen's readout, which re-applies
the channel's mode, then shows them rather than whatever the buffer held. This
replaces a firmware effect on the channel. A channel never drawn is not sent.
A frame after a release starts from dark.
*/
func (d *device) Release(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.hid == nil {
		return fmt.Errorf("releasing %s: %w", d.id.Path, os.ErrClosed)
	}
	if err := d.commit(ctx); err != nil {
		return fmt.Errorf("releasing %s: %w", d.id, err)
	}
	for i := range d.canvas.ch {
		c := &d.canvas.ch[i]
		clear(c.held)
		c.sent, c.committed, c.drawn = nil, nil, false
	}
	return nil
}

/*
commit writes each drawn channel's held colours to its buffer and shows them,
unless the buffer already holds them from this handle. The caller holds mu.

Two commands, each waited for: `26 04` with the colours, which takes about
335 ms, then `26 06` to show the buffer. A `26 06` is never sent on its own:
during a firmware effect it shows the effect's last frame.
*/
func (d *device) commit(ctx context.Context) error {
	for i := range d.canvas.ch {
		c := &d.canvas.ch[i]
		if !c.drawn || same(c.held, c.committed) {
			continue
		}
		if err := d.acked(ctx, commitWait, bufferReport(i, c.held)); err != nil {
			return err
		}
		if err := d.acked(ctx, commitWait, applyReport(i, len(c.held))); err != nil {
			return err
		}
		c.committed = append(c.committed[:0], c.held...)
		c.sent = append(c.sent[:0], c.held...)
	}
	return nil
}

/*
knowChannels reads the accessory list once per handle. The caller holds mu.

`20 03` is answered by `21 03` in about 0.6 s, among whatever else the node is
carrying. The reply is found by its prefix within two seconds, however many
other reports arrive first: OpenRGB counts ten and parses the wrong one, which
is hotaru #206. No reply is hidraw.ErrSilent, and the question is asked again
at the next call.
*/
func (d *device) knowChannels(ctx context.Context) error {
	if d.canvas.known {
		return nil
	}
	actx, cancel := context.WithTimeout(ctx, accessoryWait)
	defer cancel()
	d.hid.Drain(hidraw.QueueDepth)
	matches := func(r []byte) bool { return len(r) >= slotsAt+slotsPer*channels && r[0] == 0x21 && r[1] == 0x03 }
	reply, err := hidraw.Exchange(actx, d.hid, lightReport(0x20, 0x03), matches, reportLen)
	if err != nil {
		return fmt.Errorf("asking its accessories: %w", err)
	}
	d.canvas = channelsFrom(reply)
	return nil
}

/*
channelsFrom is the canvas a `21 03` reply describes.

A channel's LEDs are the sum of its accessories'. One id whose count is not
known leaves the whole channel without keys, because the LEDs after it could
not be placed.
*/
func channelsFrom(reply []byte) canvasState {
	s := canvasState{known: true}
	for i := range s.ch {
		count, name := 0, ""
		var parts []int
		for _, id := range reply[slotsAt+slotsPer*i : slotsAt+slotsPer*(i+1)] {
			if id == 0 {
				continue
			}
			n, ok := accessories[id]
			if !ok {
				count, parts = 0, nil
				break
			}
			count += n
			parts = append(parts, n)
			switch {
			case rings[id] && name == "":
				name = "ring"
			case fans[id] && name == "":
				name = "fan"
			}
		}
		s.ch[i] = channel{name: name, parts: parts, held: make([]colour, count)}
	}
	return s
}

// keys is every channel's LEDs as keys, numbered from 0.
func (s *canvasState) keys() []lighting.Key {
	var out []lighting.Key
	for _, c := range s.ch {
		for n := range c.held {
			name := ""
			if c.name != "" {
				name = c.name + " " + strconv.Itoa(n+1)
			}
			out = append(out, lighting.Key{ID: byte(len(out)), Name: name}) //nolint:gosec // two channels of at most 40
		}
	}
	return out
}

// merge draws px over the held frame, or changes nothing and says which key
// is not one.
func (s *canvasState) merge(px []lighting.Pixel) error {
	type at struct{ ch, led int }
	where := make([]at, len(px))
	for i, p := range px {
		id, found := int(p.ID), false
		for ch := range s.ch {
			if id < len(s.ch[ch].held) {
				where[i], found = at{ch, id}, true
				break
			}
			id -= len(s.ch[ch].held)
		}
		if !found {
			return fmt.Errorf("key %d is not a light on this cooler", p.ID)
		}
	}
	for i, p := range px {
		c := &s.ch[where[i].ch]
		c.held[where[i].led] = colour{p.R, p.G, p.B}
		c.drawn = true
	}
	return nil
}

// same is whether two frames are the same colours. A frame never sent is not
// the same as any.
func same(a, b []colour) bool {
	if b == nil || len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

/*
acked writes a lighting report and waits for the cooler's acknowledgement. The
caller holds mu.

The acknowledgement is a report starting `FF 01` with the command at bytes 14
and 15. Bytes 2 to 9 are the device's serial and are never logged.
*/
func (d *device) acked(ctx context.Context, wait time.Duration, r []byte) error {
	actx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	d.hid.Drain(hidraw.QueueDepth)
	a, b := r[0], r[1]
	matches := func(rep []byte) bool {
		return len(rep) >= 16 && rep[0] == 0xFF && rep[1] == 0x01 && rep[14] == a && rep[15] == b
	}
	if _, err := hidraw.Exchange(actx, d.hid, r, matches, reportLen); err != nil {
		return fmt.Errorf("no acknowledgement of %02x%02x: %w", a, b, err)
	}
	return nil
}

// lightReport pads a lighting command to the lighting report length.
func lightReport(data ...byte) []byte {
	r := make([]byte, lightReportLen)
	copy(r, data)
	return r
}

// grb is a channel's colours in the cooler's order: G, R, B.
func grb(cs []colour) []byte {
	out := make([]byte, 0, 3*len(cs))
	for _, c := range cs {
		out = append(out, c.g, c.r, c.b)
	}
	return out
}

// channelByte is channel index i as the cooler numbers it, from 1.
func channelByte(i int) byte { return byte(i + 1) } //nolint:gosec // i < channels

/*
liveReport is the `26 14` frame for channel i, shown at once.

Byte 2 is the channel the colours are for, and byte 3 a mask of the channels
to show: bit 0 the ring, bit 1 the fans. A channel's frame is held until a
report with its bit arrives, so `26 14 02 01` sets the fans and shows only the
ring. CAM only ever sent `26 14 01 01`. Measured on the bench (spec 024).
*/
func liveReport(i int, cs []colour) []byte {
	return lightReport(append([]byte{0x26, 0x14, channelByte(i), 1 << i}, grb(cs)...)...)
}

// bufferReport is the `26 04` write of channel i's buffer.
func bufferReport(i int, cs []colour) []byte {
	return lightReport(append([]byte{0x26, 0x04, channelByte(i), 0x00}, grb(cs)...)...)
}

// applyReport is the `26 06` that shows channel i's buffer of n LEDs, in the fixed
// mode, as CAM sends it.
func applyReport(i, n int) []byte {
	return lightReport(0x26, 0x06, channelByte(i), 0x00, 0x01, 0x00, 0x00, byte(n), 0x00, 0x00, 0x80, 0x00, 0x32, 0x00, 0x00, 0x01) //nolint:gosec // n <= 40
}
