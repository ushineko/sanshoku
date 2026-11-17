package asus

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ushineko/sanshoku/lighting"
)

const (
	// The configuration table's bytes: how many addressable headers, how
	// many lights the board has of its own, and how many of those are 12 V
	// RGB headers.
	tableHeaders    = 0x02
	tableBoardLEDs  = 0x1B
	tableRGBHeaders = 0x1D

	// boardDirect is the direct channel of the board's own lights.
	boardDirect = 0x04

	// headerMax is the most lights an addressable header may be given,
	// OpenRGB's limit.
	headerMax = 500

	// perPacket is the most lights one direct packet carries.
	perPacket = 20

	cmdEffect  = 0x35
	cmdDirect  = 0x40
	modeDirect = 0xFF
	// apply ends a frame on a channel; past255 marks an offset that counts
	// from light 256.
	apply   = 0x80
	past255 = 0x10

	// frameFloor is the shortest interval at which frames are sent. A
	// breathe on the board's light and three headers was smooth at 33 ms
	// and at 16 ms, writing a frame in about 8.4 ms (spec 027 E5).
	frameFloor = 16 * time.Millisecond

	boardZone  = "Aura Mainboard"
	headerZone = "Addressable RGB Header "
)

type colour struct{ r, g, b uint8 }

/*
channel is one of the controller's channels: the board's own lights, or an
addressable header. held is what it shows. claimed is whether this handle has
put it in direct mode, which is done once, before its first frame.
*/
type channel struct {
	name      string
	direct    byte
	effect    byte
	held      []colour
	sent      bool
	claimed   bool
	resizable bool
	// rgb is how many of the board's own lights are its 12 V RGB headers,
	// named apart from the rest, as OpenRGB names them.
	rgb int
}

/*
channelsFrom is the channels a configuration table describes: the board's own
lights first, if it has any, then each addressable header, at no lights until
a consumer gives it some. A board that reports fewer lights than RGB headers
is taken to have no RGB headers, as OpenRGB takes it.
*/
func channelsFrom(table []byte) []channel {
	var out []channel
	board, rgb := int(table[tableBoardLEDs]), int(table[tableRGBHeaders])
	if board < rgb {
		rgb = 0
	}
	effect := byte(0)
	if board > 0 {
		out = append(out, channel{name: boardZone, direct: boardDirect, effect: effect, held: make([]colour, board), rgb: rgb})
		effect++
	}
	for i := range int(table[tableHeaders]) {
		out = append(out, channel{
			name: headerZone + strconv.Itoa(i+1), direct: byte(i), effect: effect, resizable: true, //nolint:gosec // a table byte
		})
		effect++
	}
	return out
}

var (
	_ lighting.Canvas = (*device)(nil)
	_ lighting.Zoned  = (*device)(nil)
)

// Keys is every channel's lights in order, named as OpenRGB names them:
// "Aura Mainboard, LED 1", "Addressable RGB Header 2, LED 7".
func (d *device) Keys() []lighting.Key {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []lighting.Key
	for _, c := range d.ch {
		own := len(c.held) - c.rgb
		for i := range c.held {
			name := c.name + ", LED " + strconv.Itoa(i+1)
			if i >= own {
				name = c.name + ", RGB Header " + strconv.Itoa(i+1-own)
			}
			out = append(out, lighting.Key{ID: byte(len(out)), Name: name}) //nolint:gosec // see Resize
		}
	}
	return out
}

// Zones is each channel as a zone, the headers resizable from 0 to 500.
func (d *device) Zones() []lighting.Zone {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []lighting.Zone
	first := 0
	for _, c := range d.ch {
		z := lighting.Zone{Name: c.name, First: first, Count: len(c.held)}
		switch {
		case c.resizable:
			// A header's shape is what is plugged in, which the board cannot
			// tell (spec 032).
			z.Max = headerMax
		case len(c.held) == 1:
			z.Shape = lighting.ShapeSingle
		default:
			z.Shape = lighting.ShapeLine
		}
		out = append(out, z)
		first += len(c.held)
	}
	return out
}

/*
Resize sets an addressable header's length. The board cannot read what is
plugged in, so this is the consumer's to say; OpenRGB kept it in its own
profile. A key is a byte, so the lights of every channel together are at most
256.
*/
func (d *device) Resize(zone string, lights int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	at := -1
	for i, c := range d.ch {
		if c.name == zone && c.resizable {
			at = i
		}
	}
	if at < 0 {
		return fmt.Errorf("%q on %s: %w", zone, d.id, lighting.ErrNoZone)
	}
	if lights < 0 || lights > headerMax {
		return fmt.Errorf("%q on %s: %d lights is outside 0 to %d", zone, d.id, lights, headerMax)
	}
	total := lights
	for i, c := range d.ch {
		if i != at {
			total += len(c.held)
		}
	}
	if total > 256 {
		return fmt.Errorf("%q on %s: %d lights in all, more than a frame can name", zone, d.id, total)
	}
	c := &d.ch[at]
	held := make([]colour, lights)
	copy(held, c.held)
	c.held, c.sent = held, false
	return nil
}

// Floor is the shortest interval at which frames are sent.
func (d *device) Floor() time.Duration { return frameFloor }

/*
Frame draws px over the held frame and sends each channel that changed.

A channel is put in direct mode before its first frame on this handle, and
written 20 lights a packet, the last packet applying it. The controller
answers none of it. A key not in Keys is an error and nothing is sent.
*/
func (d *device) Frame(_ context.Context, px []lighting.Pixel) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rd == nil {
		return fmt.Errorf("lighting %s: %w", d.id.Path, os.ErrClosed)
	}
	type at struct{ ch, led int }
	where := make([]at, len(px))
	for i, p := range px {
		id, found := int(p.ID), false
		for ch := range d.ch {
			if id < len(d.ch[ch].held) {
				where[i], found = at{ch, id}, true
				break
			}
			id -= len(d.ch[ch].held)
		}
		if !found {
			return fmt.Errorf("a frame to %s: key %d is not a light on it", d.id, p.ID)
		}
	}
	for i, p := range px {
		c := &d.ch[where[i].ch]
		if next := (colour{p.R, p.G, p.B}); c.held[where[i].led] != next {
			c.held[where[i].led], c.sent = next, false
		}
	}
	for i := range d.ch {
		c := &d.ch[i]
		if len(c.held) == 0 || (c.sent && c.claimed) {
			continue
		}
		if err := d.send(c); err != nil {
			return fmt.Errorf("a frame to %s: %w", d.id, err)
		}
	}
	return nil
}

/*
Release forgets the frame and sends nothing. The controller has no command
known to hand its lights back to the effect stored in it; that returns when
the machine is next powered on. The lights keep the last frame.
*/
func (d *device) Release(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rd == nil {
		return fmt.Errorf("releasing %s: %w", d.id.Path, os.ErrClosed)
	}
	for i := range d.ch {
		c := &d.ch[i]
		clear(c.held)
		c.sent = false
	}
	return nil
}

// send puts a channel in direct mode if this handle has not, and writes its
// lights. The caller holds mu.
func (d *device) send(c *channel) error {
	if !c.claimed {
		if err := d.write(cmdEffect, c.effect, 0x00, 0x00, modeDirect); err != nil {
			return err
		}
		c.claimed = true
	}
	for _, p := range directPackets(c.direct, c.held) {
		if err := d.write(p...); err != nil {
			return err
		}
	}
	c.sent = true
	return nil
}

/*
directPackets is a channel's lights as direct packets: `40 <channel> <offset>
<count>` and up to 20 lights R, G, B. The last carries apply. A packet
starting at light 256 or later carries past255, and its offset counts from
256.
*/
func directPackets(direct byte, held []colour) [][]byte {
	var out [][]byte
	for start := 0; start < len(held); start += perPacket {
		n := min(perPacket, len(held)-start)
		cmd, offset := direct, start
		if offset >= 256 {
			cmd, offset = cmd|past255, offset-256
		}
		if start+n == len(held) {
			cmd |= apply
		}
		p := []byte{cmdDirect, cmd, byte(offset), byte(n)} //nolint:gosec // offset < 256, n <= 20
		for _, c := range held[start : start+n] {
			p = append(p, c.r, c.g, c.b)
		}
		out = append(out, p)
	}
	return out
}
