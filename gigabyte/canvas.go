package gigabyte

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ushineko/sanshoku/lighting"
)

// Commands, from OpenRGB 1.1's Fusion 2 USB controller.
const (
	// cmdEffectFirst to cmdEffectLast are the effect registers of the
	// single-colour lights, 20 + the light's index; start clears them.
	cmdEffectFirst = 0x20
	cmdEffectLast  = 0x27
	// cmdCommit applies the effect registers named by its mask.
	cmdCommit = 0x28
	// cmdBeat is the sound-reactive mode, which start turns off.
	cmdBeat = 0x31
	// cmdHeaderEffect turns a header's own effect off where its bit is set.
	cmdHeaderEffect = 0x32
	// cmdLengths sets each header's length class.
	cmdLengths = 0x34
	// cmdLampArray is the lamp-array mode, which start turns off.
	cmdLampArray = 0x48

	// effectStatic is the effect a single-colour light is given to show one
	// colour, as OpenRGB does in Direct mode.
	effectStatic = 0x01

	// perPacket is the most lights one header packet carries.
	perPacket = 19

	// headerMax is the most lights a header on an IT57xx controller may be
	// given, OpenRGB's limit.
	headerMax = 256
)

// frameFloor is the shortest interval at which frames are sent. A breathe on
// LED_C was smooth at 33 ms and at 16 ms, writing a frame in about 3.8 ms
// (spec 039 E5).
const frameFloor = 16 * time.Millisecond

type colour struct{ r, g, b uint8 }

/*
order is where a header's controller puts red, green and blue in each light's
three bytes, as the controller reports it.
*/
type order struct{ r, g, b int }

/*
orderFrom reads a colour order as the controller reports it: blue's position
in the low byte, then green's, then red's. A word that names no order is taken
as GRB, which is what both headers on the bench reported (spec 039 E0).
*/
func orderFrom(word uint32) order {
	b, g, r, top := int(word&0xFF), int(word>>8&0xFF), int(word>>16&0xFF), word>>24
	if top != 0 || r > 2 || g > 2 || b > 2 || r == g || r == b || g == b {
		return order{r: 1, g: 0, b: 2}
	}
	return order{r: r, g: g, b: b}
}

/*
zone is one of the controller's lights: an addressable header or a
single-colour light. held is what it shows; sent is whether the controller has
been given it.
*/
type zone struct {
	def  zoneDef
	ord  order
	held []colour
	sent bool
}

// zonesFrom lays out a controller's zones: headers at no lights until a
// consumer gives them some, single-colour lights at one.
func zonesFrom(layout []zoneDef, orders [2]order) []zone {
	out := make([]zone, 0, len(layout))
	for _, def := range layout {
		z := zone{def: def}
		if def.header {
			z.ord = orders[def.slot]
		} else {
			z.held = make([]colour, 1)
		}
		out = append(out, z)
	}
	return out
}

var (
	_ lighting.Canvas = (*device)(nil)
	_ lighting.Zoned  = (*device)(nil)
)

// Keys is every zone's lights in order, named as OpenRGB names them:
// "ARGB_V2_2, LED 7" on a header, "LED_C" for a single-colour light.
func (d *device) Keys() []lighting.Key {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []lighting.Key
	for _, z := range d.zones {
		for i := range z.held {
			name := z.def.name
			if z.def.header {
				name += ", LED " + strconv.Itoa(i+1)
			}
			out = append(out, lighting.Key{ID: byte(len(out)), Name: name}) //nolint:gosec // see Resize
		}
	}
	return out
}

// Zones is each zone, the headers resizable from 0 to 256.
func (d *device) Zones() []lighting.Zone {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []lighting.Zone
	first := 0
	for _, z := range d.zones {
		lz := lighting.Zone{Name: z.def.name, First: first, Count: len(z.held)}
		if z.def.header {
			// A header's shape is what is plugged in, which the board cannot
			// tell (spec 032).
			lz.Max = headerMax
		} else {
			lz.Shape = lighting.ShapeSingle
		}
		out = append(out, lz)
		first += len(z.held)
	}
	return out
}

/*
Resize sets an addressable header's length. The board cannot read what is
plugged in, so this is the consumer's to say. A key is a byte, so the lights
of every zone together are at most 256. It sends nothing: the length classes
go to the controller with the next frame.
*/
func (d *device) Resize(name string, lights int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	at := -1
	for i, z := range d.zones {
		if z.def.name == name && z.def.header {
			at = i
		}
	}
	if at < 0 {
		return fmt.Errorf("%q on %s: %w", name, d.id, lighting.ErrNoZone)
	}
	if lights < 0 || lights > headerMax {
		return fmt.Errorf("%q on %s: %d lights is outside 0 to %d", name, d.id, lights, headerMax)
	}
	total := lights
	for i, z := range d.zones {
		if i != at {
			total += len(z.held)
		}
	}
	if total > 256 {
		return fmt.Errorf("%q on %s: %d lights in all, more than a frame can name", name, d.id, total)
	}
	z := &d.zones[at]
	held := make([]colour, lights)
	copy(held, z.held)
	z.held, z.sent = held, false
	return nil
}

// Floor is the shortest interval at which frames are sent.
func (d *device) Floor() time.Duration { return frameFloor }

/*
Frame draws px over the held frame and sends each zone that changed.

Before the first header frame on this handle, and whenever a header's length
class changed, the length classes are sent and the headers' own effect turned
off. A header is written 19 lights a packet, in the order its controller
reported. A single-colour light is given a static effect in its colour, and
one commit applies them. The controller answers none of it. A key not in Keys
is an error and nothing is sent.
*/
func (d *device) Frame(ctx context.Context, px []lighting.Pixel) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rd == nil {
		return fmt.Errorf("lighting %s: %w", d.id.Path, os.ErrClosed)
	}
	type at struct{ zone, led int }
	where := make([]at, len(px))
	for i, p := range px {
		id, found := int(p.ID), false
		for z := range d.zones {
			if id < len(d.zones[z].held) {
				where[i], found = at{z, id}, true
				break
			}
			id -= len(d.zones[z].held)
		}
		if !found {
			return fmt.Errorf("a frame to %s: key %d is not a light on it", d.id, p.ID)
		}
	}
	for i, p := range px {
		z := &d.zones[where[i].zone]
		if next := (colour{p.R, p.G, p.B}); z.held[where[i].led] != next {
			z.held[where[i].led], z.sent = next, false
		}
	}
	if err := d.send(ctx); err != nil {
		return fmt.Errorf("a frame to %s: %w", d.id, err)
	}
	return nil
}

/*
Release forgets the frame and sends nothing. Whether the controller can be
handed back to the effect stored in it is spec 039 E7's to find; until then
the lights keep the last frame.
*/
func (d *device) Release(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rd == nil {
		return fmt.Errorf("releasing %s: %w", d.id.Path, os.ErrClosed)
	}
	for i := range d.zones {
		z := &d.zones[i]
		clear(z.held)
		z.sent = false
	}
	return nil
}

// send writes every zone that has not been sent. The caller holds mu.
func (d *device) send(ctx context.Context) error {
	if err := d.prepareHeaders(ctx); err != nil {
		return err
	}
	var mask uint32
	for i := range d.zones {
		z := &d.zones[i]
		if z.sent || len(z.held) == 0 {
			continue
		}
		if z.def.header {
			for _, p := range headerPackets(z.def.packet, z.ord, z.held) {
				if err := d.write(ctx, p...); err != nil {
					return err
				}
			}
		} else {
			if err := d.write(ctx, staticEffect(z.def.light, z.held[0])...); err != nil {
				return err
			}
			mask |= 1 << z.def.light
		}
		z.sent = true
	}
	if mask == 0 {
		return nil
	}
	commit := []byte{cmdCommit, 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(commit[1:], mask)
	return d.write(ctx, commit...)
}

/*
prepareHeaders sends the headers' length classes where they changed, and turns
the headers' own effect off once per handle, before a header's lights are
written. The caller holds mu.
*/
func (d *device) prepareHeaders(ctx context.Context) error {
	var classes [2]byte
	var effectOff byte
	pending := false
	for _, z := range d.zones {
		if !z.def.header {
			continue
		}
		classes[z.def.slot] = lengthClass(len(z.held))
		effectOff |= 1 << z.def.slot
		pending = pending || (!z.sent && len(z.held) > 0)
	}
	if !pending {
		return nil
	}
	if !d.lengthsKnow || classes != d.lengthsSent {
		if err := d.write(ctx, cmdLengths, classes[1]<<4|classes[0], 0x00, 0x00); err != nil {
			return err
		}
		d.lengthsSent, d.lengthsKnow = classes, true
	}
	if !d.claimed {
		if err := d.write(ctx, cmdHeaderEffect, effectOff); err != nil {
			return err
		}
		d.claimed = true
	}
	return nil
}

// lengthClass is the class a header's length falls in: up to 32, 64 or 256
// lights.
func lengthClass(n int) byte {
	switch {
	case n <= 32:
		return 0
	case n <= 64:
		return 1
	default:
		return 2
	}
}

/*
headerPackets is a header's lights as packets after the report ID: the
header's packet byte, the offset and count in bytes (the offset two bytes,
low first), and up to 19 lights in the controller's order.
*/
func headerPackets(header byte, ord order, held []colour) [][]byte {
	var out [][]byte
	for start := 0; start < len(held); start += perPacket {
		n := min(perPacket, len(held)-start)
		p := make([]byte, 4+3*perPacket)
		p[0] = header
		binary.LittleEndian.PutUint16(p[1:], uint16(start*3)) //nolint:gosec // at most 256 lights
		p[3] = byte(n * 3)                                    //nolint:gosec // at most 57
		for i, c := range held[start : start+n] {
			at := 4 + 3*i
			p[at+ord.r], p[at+ord.g], p[at+ord.b] = c.r, c.g, c.b
		}
		out = append(out, p)
	}
	return out
}

/*
staticEffect is a single-colour light's effect register given one colour,
after the report ID: the register (20 + the light), the light's bit in the
zone mask, the static effect at full brightness, and the colour as blue,
green, red.
*/
func staticEffect(light byte, c colour) []byte {
	p := make([]byte, 21)
	p[0] = cmdEffectFirst + light
	binary.LittleEndian.PutUint32(p[1:], 1<<light)
	p[10] = effectStatic
	p[11] = 0xFF // the most brightness
	p[13], p[14], p[15] = c.b, c.g, c.r
	return p
}
