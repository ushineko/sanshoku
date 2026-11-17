package msi

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/ushineko/sanshoku/lighting"
)

// The controller's registers (docs/msi-gpu.md).
const (
	regMode       = 0x22
	regUnknown    = 0x2E // always written 00 first
	regR, regG    = 0x30, 0x31
	regB          = 0x32
	regBlock1     = 0x27 // colour blocks, B G R
	regBlock2     = 0x28
	regBlock3     = 0x29
	regBrightness = 0x36
	regSpeed      = 0x38
	regControl    = 0x46

	modeIdle   = 0x1C // the mode the card is put in while it is set up
	modeStatic = 0x13
	modeOff    = 0x01

	// fullBrightness is the brightness register at its top step (5 × 20).
	fullBrightness = 100
	// mediumSpeed is OpenRGB's middle speed. The register is a period:
	// 04 slow, 02 medium, 01 fast.
	mediumSpeed = 0x02

	// controlUserColours tells a rainbow or magic mode to use the colour
	// blocks; 00 is its own colours, rightwards.
	controlUserColours = 0x01
)

/*
frameFloor is the shortest interval at which colours are sent. A colour is six
writes with 20 ms after each, and the card must sit in its static mode for 70
to 100 ms or the next colour cancels the change: about four colours a second
(hotaru docs/hardware.md, "One GPU that needs time between frames").
*/
const frameFloor = 250 * time.Millisecond

type colour struct{ r, g, b uint8 }

// shape is how an effect takes its colours.
type shape int

const (
	none  shape = iota // its own colours
	one                // one colour, registers 30 to 32
	two                // two colour blocks
	three              // three colour blocks, and the control register
)

// effect is one of the card's own effects, as OpenRGB names and writes it.
type effect struct {
	name  string
	mode  byte
	shape shape
	// control is the control register's value for a rainbow or magic mode
	// that takes no colours, written before (rainbow) or after (magic) the
	// idle mode, as OpenRGB writes it; -1 for none.
	control     int
	controlLast bool
	min, max    int
}

var effects = []effect{
	{"Off", modeOff, none, -1, false, 0, 0},
	{"Rainbow Wave", 0x08, none, 0x00, false, 0, 0},
	{"Magic", 0x07, none, 0x00, true, 0, 0},
	{"Color Cycle", 0x07, three, -1, false, 1, 3},
	{"Patrolling", 0x05, one, -1, false, 1, 1},
	{"Streaming", 0x06, none, -1, false, 0, 0},
	{"Lightning", 0x15, none, -1, false, 0, 0},
	{"Wave", 0x08, three, -1, false, 1, 3},
	{"Meteor", 0x16, one, -1, false, 1, 1},
	{"Stack", 0x0D, one, -1, false, 1, 1},
	{"Rhythm", 0x0C, one, -1, false, 1, 1},
	{"Flowing", 0x09, one, -1, false, 1, 1},
	{"Whirling", 0x0F, one, -1, false, 1, 1},
	{"Fade In", 0x14, two, -1, false, 1, 2},
	{"Breathing", 0x04, two, -1, false, 1, 2},
	{"Flashing", 0x02, one, -1, false, 1, 1},
	{"Double Flashing", 0x03, one, -1, false, 1, 1},
}

// defaults are the colours an effect starts with, OpenRGB's: red, green,
// blue.
var defaults = [3]colour{{0xFF, 0, 0}, {0, 0xFF, 0}, {0, 0, 0xFF}}

var (
	_ lighting.Canvas = (*device)(nil)
	_ lighting.Zoned  = (*device)(nil)
	_ lighting.Tuned  = (*device)(nil)
)

// Keys is the card's one light, "Color", as OpenRGB names it.
func (d *device) Keys() []lighting.Key { return []lighting.Key{{ID: 0, Name: "Color"}} }

// Zones is the one zone, "GPU", a single light.
func (d *device) Zones() []lighting.Zone {
	return []lighting.Zone{{Name: "GPU", Count: 1, Shape: lighting.ShapeSingle}}
}

// Resize refuses: the card has one light.
func (d *device) Resize(zone string, _ int) error {
	return fmt.Errorf("%q on %s: %w", zone, d.id, lighting.ErrNoZone)
}

// Floor is 250 ms.
func (d *device) Floor() time.Duration { return frameFloor }

/*
Frame shows the light's colour as the card's static mode: the unknown
register, the idle mode, the colour, full brightness, then static. A colour
the card already shows is not sent. It replaces an effect this handle set.
*/
func (d *device) Frame(_ context.Context, px []lighting.Pixel) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.bus == nil {
		return fmt.Errorf("lighting %s: %w", d.id.Path, os.ErrClosed)
	}
	next := d.held
	for _, p := range px {
		if p.ID != 0 {
			return fmt.Errorf("a frame to %s: key %d is not its light", d.id, p.ID)
		}
		next = colour{p.R, p.G, p.B}
	}
	if d.shown && next == d.held {
		return nil
	}
	d.held, d.shown = next, false
	for _, w := range [][2]byte{
		{regUnknown, 0x00}, {regMode, modeIdle},
		{regR, next.r}, {regG, next.g}, {regB, next.b},
		{regBrightness, fullBrightness}, {regMode, modeStatic},
	} {
		if err := d.write(w[0], w[1]); err != nil {
			return fmt.Errorf("a frame to %s: %w", d.id, err)
		}
	}
	d.shown, d.running = true, -1
	return nil
}

// Release forgets the colour and sends nothing: the card keeps what it shows.
func (d *device) Release(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.bus == nil {
		return fmt.Errorf("releasing %s: %w", d.id.Path, os.ErrClosed)
	}
	d.held, d.shown = colour{}, false
	return nil
}

// Effects names the card's own effects, as OpenRGB names them.
func (d *device) Effects() []string {
	out := make([]string, len(effects))
	for i, e := range effects {
		out[i] = e.name
	}
	return out
}

// Catalogue is each effect's number, speeds (slowest first) and colours.
func (d *device) Catalogue() []lighting.EffectInfo {
	out := make([]lighting.EffectInfo, len(effects))
	for i, e := range effects {
		out[i] = info(e)
	}
	return out
}

// info is e as the catalogue describes it. The speed register is a period:
// 04 slow, 02 medium, 01 fast.
func info(e effect) lighting.EffectInfo {
	out := lighting.EffectInfo{Name: e.name, Number: int(e.mode), MinColours: e.min, MaxColours: e.max}
	if e.mode != modeOff {
		out.Speeds, out.Speed = []int{0x04, 0x02, 0x01}, mediumSpeed
	}
	return out
}

/*
Effect is the effect this handle last set, with its first colour. The card
answers no question, so before this handle has set one it is
lighting.ErrUnknownEffect.
*/
func (d *device) Effect(context.Context) (lighting.Effect, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running < 0 {
		return lighting.Effect{}, fmt.Errorf("%s: %w", d.id, lighting.ErrUnknownEffect)
	}
	e := effects[d.running]
	out := lighting.Effect{Name: e.name, Coloured: e.shape != none}
	if out.Coloured {
		cs := d.colours()
		for _, c := range cs[:e.max] {
			out.Colours = append(out.Colours, lighting.Colour{R: c.r, G: c.g, B: c.b})
		}
		out.Colour = &out.Colours[0]
	}
	if e.mode != modeOff {
		speed := int(d.speedValue())
		out.Speed = &speed
	}
	return out, nil
}

/*
SetEffectWith runs the named effect with the colours and speed given (spec
032): one colour into registers 30 to 32, two or three into the colour
blocks; the speed is the nearest of the card's three. A nil field keeps what
this handle set. The same settings again send nothing.
*/
func (d *device) SetEffectWith(_ context.Context, name string, s lighting.Settings) (bool, error) {
	at := -1
	for i, e := range effects {
		if strings.EqualFold(e.name, name) {
			at = i
		}
	}
	if at < 0 {
		return false, fmt.Errorf("%q: %w", name, lighting.ErrNoEffect)
	}
	colours, speed, err := lighting.Fit(info(effects[at]), s, true)
	if err != nil {
		return false, fmt.Errorf("%s: %w", d.id, err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	given, sp := d.given, d.speed
	if len(colours) > 0 {
		given = given[:0:0]
		for _, c := range colours {
			given = append(given, colour{c.R, c.G, c.B})
		}
	}
	if speed != nil {
		sp = byte(*speed) //nolint:gosec // 1, 2 or 4
	}
	if d.running == at && slices.Equal(given, d.given) && sp == d.speed {
		return false, nil
	}
	d.given, d.speed = given, sp
	if len(given) > 0 {
		d.colour = &given[0]
	}
	if err := d.run(at); err != nil {
		return false, err
	}
	return true, nil
}

// speedValue is the speed register's value: what SetEffectWith was given,
// or medium. The caller holds mu.
func (d *device) speedValue() byte {
	if d.speed != 0 {
		return d.speed
	}
	return mediumSpeed
}

// SetEffect runs a named effect at medium speed, in the colour this handle
// was last given, or OpenRGB's colours. The same effect again sends nothing.
func (d *device) SetEffect(_ context.Context, name string) (bool, error) {
	at := -1
	for i, e := range effects {
		if strings.EqualFold(e.name, name) {
			at = i
		}
	}
	if at < 0 {
		return false, fmt.Errorf("%q: %w", name, lighting.ErrNoEffect)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running == at {
		return false, nil
	}
	if err := d.run(at); err != nil {
		return false, err
	}
	return true, nil
}

// SetColour runs the effect this handle set again with c as its first colour,
// and keeps c for later effects.
func (d *device) SetColour(_ context.Context, c lighting.Colour) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running < 0 {
		return false, fmt.Errorf("%s: %w", d.id, lighting.ErrUnknownEffect)
	}
	if effects[d.running].shape == none {
		return false, fmt.Errorf("%s: %w", effects[d.running].name, lighting.ErrNoColour)
	}
	next := colour{c.R, c.G, c.B}
	if d.colour != nil && *d.colour == next {
		return false, nil
	}
	d.colour = &next
	if len(d.given) > 0 {
		d.given[0] = next
	}
	if err := d.run(d.running); err != nil {
		return false, err
	}
	return true, nil
}

// colours is an effect's colours: OpenRGB's, replaced by those this handle
// was given, or the first by the colour it was last given. The caller holds
// mu.
func (d *device) colours() [3]colour {
	cs := defaults
	if d.colour != nil {
		cs[0] = *d.colour
	}
	copy(cs[:], d.given)
	return cs
}

/*
run writes an effect as OpenRGB writes it: the unknown register and the idle
mode, the effect's colours by its shape, full brightness and medium speed,
then the effect's mode. Off takes neither brightness nor speed. The caller
holds mu.
*/
func (d *device) run(at int) error {
	if d.bus == nil {
		return fmt.Errorf("%s: %w", d.id.Path, os.ErrClosed)
	}
	e, cs := effects[at], d.colours()
	type w struct {
		reg   byte
		value byte
		block bool
		c     colour
	}
	var ws []w
	if e.control >= 0 && !e.controlLast {
		ws = append(ws, w{reg: regControl, value: byte(e.control)}) //nolint:gosec // a table value, 00
	}
	if e.shape == three && e.mode == 0x08 {
		ws = append(ws, w{reg: regControl, value: controlUserColours})
	}
	ws = append(ws, w{reg: regUnknown}, w{reg: regMode, value: modeIdle})
	if e.control >= 0 && e.controlLast {
		ws = append(ws, w{reg: regControl, value: byte(e.control)}) //nolint:gosec // a table value, 00
	}
	switch e.shape {
	case none:
	case one:
		ws = append(ws, w{reg: regR, value: cs[0].r}, w{reg: regG, value: cs[0].g}, w{reg: regB, value: cs[0].b})
	case two:
		ws = append(ws, w{reg: regBlock1, block: true, c: cs[0]}, w{reg: regBlock2, block: true, c: cs[1]})
	case three:
		if e.mode != 0x08 {
			ws = append(ws, w{reg: regControl, value: controlUserColours})
		}
		ws = append(ws, w{reg: regBlock1, block: true, c: cs[0]}, w{reg: regBlock2, block: true, c: cs[1]},
			w{reg: regBlock3, block: true, c: cs[2]})
	}
	if e.mode != modeOff {
		ws = append(ws, w{reg: regBrightness, value: fullBrightness}, w{reg: regSpeed, value: d.speedValue()})
	}
	ws = append(ws, w{reg: regMode, value: e.mode})
	for _, x := range ws {
		var err error
		if x.block {
			err = d.block(x.reg, x.c)
		} else {
			err = d.write(x.reg, x.value)
		}
		if err != nil {
			return fmt.Errorf("setting %s on %s: %w", e.name, d.id, err)
		}
	}
	d.running, d.shown = at, false
	return nil
}
