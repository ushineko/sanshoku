package keychron

import (
	"context"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/ushineko/sanshoku/lighting"
)

// perPacket is the most keys one colour packet carries.
const perPacket = 9

/*
frameFloor is the shortest interval at which frames are sent. A frame of 100
keys is 12 request and reply packets, 62 ms on the bench, where a hue wave
sent flat out was very smooth (spec 028).
*/
const frameFloor = 70 * time.Millisecond

// zoneName is the one zone, as OpenRGB names it.
const zoneName = "Keyboard"

type colour struct{ r, g, b uint8 }

var (
	_ lighting.Canvas = (*device)(nil)
	_ lighting.Zoned  = (*device)(nil)
)

// Keys is one per light in the firmware's order, as OpenRGB orders them. The
// names are not read.
func (d *device) Keys() []lighting.Key {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]lighting.Key, len(d.held))
	for i := range out {
		out[i] = lighting.Key{ID: byte(i)} //nolint:gosec // at most 256, checked at start
	}
	return out
}

// Zones is the one zone, "Keyboard", of every key.
func (d *device) Zones() []lighting.Zone {
	d.mu.Lock()
	defer d.mu.Unlock()
	return []lighting.Zone{{Name: zoneName, First: 0, Count: len(d.held), Shape: lighting.ShapeGrid}}
}

// Resize refuses: the keyboard's one zone is the keyboard.
func (d *device) Resize(zone string, _ int) error {
	return fmt.Errorf("%q on %s: %w", zone, d.id, lighting.ErrNoZone)
}

// Floor is 70 ms.
func (d *device) Floor() time.Duration { return frameFloor }

/*
Frame draws px over the held frame and sends what changed.

The running effect is read first and the per-key effect claimed where the
keyboard runs another: its own keys can change it. Each key goes as its hue
and saturation, in packets of 9, and only the packets that changed; the
board's brightness follows the brightest key. A key not in Keys is an error
and nothing is sent.
*/
func (d *device) Frame(ctx context.Context, px []lighting.Pixel) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rd == nil {
		return fmt.Errorf("lighting %s: %w", d.id.Path, os.ErrClosed)
	}
	for _, p := range px {
		if int(p.ID) >= len(d.held) {
			return fmt.Errorf("a frame to %s: key %d is not a light on it", d.id, p.ID)
		}
	}
	for _, p := range px {
		d.held[p.ID] = colour{p.R, p.G, p.B}
	}
	if err := d.claim(ctx); err != nil {
		return fmt.Errorf("a frame to %s: %w", d.id, err)
	}
	if err := d.sendKeys(ctx); err != nil {
		return fmt.Errorf("a frame to %s: %w", d.id, err)
	}
	if err := d.sendBrightness(ctx); err != nil {
		return fmt.Errorf("a frame to %s: %w", d.id, err)
	}
	return nil
}

/*
Release puts back the effect and brightness the keyboard had when this handle
took it, and forgets the frame. A handle that never drew sends nothing.
*/
func (d *device) Release(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rd == nil {
		return fmt.Errorf("releasing %s: %w", d.id.Path, os.ErrClosed)
	}
	if d.found != nil {
		if _, err := d.ask(ctx, cmdSetValue, matrix, valueEffect, d.found.effect); err != nil {
			return fmt.Errorf("releasing %s: %w", d.id, err)
		}
		if _, err := d.ask(ctx, cmdSetValue, matrix, valueBrightness, d.found.brightness); err != nil {
			return fmt.Errorf("releasing %s: %w", d.id, err)
		}
	}
	clear(d.held)
	d.sentHS, d.bright, d.found = nil, -1, nil
	return nil
}

/*
claim reads the running effect and, where it is not the per-key one, records
what the keyboard ran (the first time) and takes it: the per-key effect, then
per-key solid. A claim makes every key and the brightness go out again. The
caller holds mu.
*/
func (d *device) claim(ctx context.Context) error {
	e, err := d.ask(ctx, cmdGetValue, matrix, valueEffect)
	if err != nil {
		return err
	}
	if e[3] == perKeyEffect {
		return nil
	}
	if d.found == nil {
		b, err := d.ask(ctx, cmdGetValue, matrix, valueBrightness)
		if err != nil {
			return err
		}
		d.found = &setting{effect: e[3], brightness: b[3]}
	}
	if _, err := d.ask(ctx, cmdSetValue, matrix, valueEffect, perKeyEffect); err != nil {
		return err
	}
	if _, err := d.ask(ctx, cmdRGB, rgbSetType, solid); err != nil {
		return err
	}
	d.sentHS, d.bright = nil, -1
	return nil
}

// sendKeys sends each packet of 9 keys whose hue or saturation changed, and
// every packet after a claim. The caller holds mu.
func (d *device) sendKeys(ctx context.Context) error {
	all := d.sentHS == nil
	if all {
		d.sentHS = make([][2]uint8, len(d.held))
	}
	for start := 0; start < len(d.held); start += perPacket {
		n := min(perPacket, len(d.held)-start)
		p := []byte{cmdRGB, rgbSetColour, byte(start), byte(n)} //nolint:gosec // at most 256 keys
		next := make([][2]uint8, n)
		changed := all
		for i := range n {
			h, s, v := hsv(d.held[start+i])
			next[i] = [2]uint8{h, s}
			changed = changed || d.sentHS[start+i] != next[i]
			p = append(p, h, s, v)
		}
		if !changed {
			continue
		}
		if _, err := d.ask(ctx, p...); err != nil {
			d.sentHS = nil // not known what the keyboard shows: all again next time
			return err
		}
		copy(d.sentHS[start:], next)
	}
	return nil
}

// sendBrightness sets the board's brightness to the brightest key's value,
// where it changed. The caller holds mu.
func (d *device) sendBrightness(ctx context.Context) error {
	v := uint8(0)
	for _, c := range d.held {
		_, _, kv := hsv(c)
		v = max(v, kv)
	}
	if int(v) == d.bright {
		return nil
	}
	if _, err := d.ask(ctx, cmdSetValue, matrix, valueBrightness, v); err != nil {
		return err
	}
	d.bright = int(v)
	return nil
}

/*
hsv is a colour as the keyboard takes it: hue scaled so 256 is a full turn,
saturation and value 0 to 255. Grey, white and black have no hue, and go as
hue 0 with saturation 0.
*/
func hsv(c colour) (h, s, v uint8) {
	mx := max(c.r, c.g, c.b)
	mn := min(c.r, c.g, c.b)
	v = mx
	if mx == 0 {
		return 0, 0, 0
	}
	delta := float64(mx - mn)
	s = uint8(math.Round(255 * delta / float64(mx)))
	if delta == 0 {
		return 0, 0, v
	}
	r, g, b := float64(c.r), float64(c.g), float64(c.b)
	var deg float64
	switch mx {
	case c.r:
		deg = math.Mod((g-b)/delta, 6)
	case c.g:
		deg = (b-r)/delta + 2
	default:
		deg = (r-g)/delta + 4
	}
	deg *= 60
	if deg < 0 {
		deg += 360
	}
	return uint8(int(math.Round(deg*256/360)) & 0xFF), s, v //nolint:gosec // a full turn wraps to 0
}
