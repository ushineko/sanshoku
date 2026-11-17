package keychron

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/ushineko/sanshoku/lighting"
)

/*
effect is one of the keyboard's own effects: its VIA number, and whether it
shows the stored colour. Names and numbers are OpenRGB 1.1's QMK Keychron
table, so a consumer's records made through OpenRGB find them (spec 031).
*/
type effect struct {
	name     string
	number   uint8
	coloured bool
	// paced is an effect that moves at the keyboard's one speed.
	paced bool
}

var effects = []effect{
	{"Solid Color", 1, true, false},
	{"Breathing", 2, true, true},
	{"Band Spiral", 3, true, true},
	{"Cycle All", 4, false, true},
	{"Cycle Left Right", 5, false, true},
	{"Cycle Up Down", 6, false, true},
	{"Rainbow Moving Chevron", 7, false, true},
	{"Cycle Out In", 8, false, true},
	{"Cycle Out In Dual", 9, false, true},
	{"Cycle Pinwheel", 10, false, true},
	{"Cycle Spiral", 11, false, true},
	{"Dual Beacon", 12, false, true},
	{"Rainbow Beacon", 13, false, true},
	{"Jellybean Raindrops", 14, false, false},
	{"Pixel Rain", 15, false, false},
	{"Typing Heatmap", 16, false, false},
	{"Digital Rain", 17, false, true},
	{"Solid Reactive Simple", 18, true, true},
	{"Solid Reactive Multiwide", 19, true, true},
	{"Solid Reactive Multinexus", 20, true, true},
	{"Splash", 21, false, true},
	{"Solid Splash", 22, true, true},
}

var _ lighting.Tuned = (*device)(nil)

// The keyboard's one speed, for every effect that moves: 0 to 255, 127 as it
// came (spec 032).
const (
	slowest      = 0
	fastest      = 255
	defaultSpeed = 127
)

// Effects names the keyboard's own effects, as OpenRGB names them, in the
// firmware's order.
func (d *device) Effects() []string {
	out := make([]string, len(effects))
	for i, e := range effects {
		out[i] = e.name
	}
	return out
}

// Catalogue is each effect's number and colours. Speeds are not given: the
// firmware keeps one speed for every effect.
func (d *device) Catalogue() []lighting.EffectInfo {
	out := make([]lighting.EffectInfo, len(effects))
	for i, e := range effects {
		out[i] = info(e)
	}
	return out
}

// info is e as the catalogue describes it.
func info(e effect) lighting.EffectInfo {
	out := lighting.EffectInfo{Name: e.name, Number: int(e.number)}
	if e.coloured {
		out.MinColours, out.MaxColours = 1, 1
	}
	if e.paced {
		out.Speeds, out.Speed = []int{slowest, fastest}, defaultSpeed
	}
	return out
}

/*
Effect reads the running effect and, for one that shows it, the stored
colour at the board's brightness. While frames are drawn the keyboard runs
the per-key effect, which is not one of its own: that, and an effect off the
list, is lighting.ErrUnknownEffect.
*/
func (d *device) Effect(ctx context.Context) (lighting.Effect, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rd == nil {
		return lighting.Effect{}, fmt.Errorf("reading %s: %w", d.id.Path, os.ErrClosed)
	}
	r, err := d.ask(ctx, cmdGetValue, matrix, valueEffect)
	if err != nil {
		return lighting.Effect{}, fmt.Errorf("reading %s: %w", d.id, err)
	}
	e, ok := byNumber(r[3])
	if !ok {
		return lighting.Effect{}, fmt.Errorf("%s runs effect %d: %w", d.id, r[3], lighting.ErrUnknownEffect)
	}
	out := lighting.Effect{Name: e.name, Coloured: e.coloured}
	if e.paced {
		sp, err := d.ask(ctx, cmdGetValue, matrix, valueSpeed)
		if err != nil {
			return lighting.Effect{}, fmt.Errorf("reading %s: %w", d.id, err)
		}
		speed := int(sp[3])
		out.Speed = &speed
	}
	if !e.coloured {
		return out, nil
	}
	h, s, v, err := d.stored(ctx)
	if err != nil {
		return lighting.Effect{}, fmt.Errorf("reading %s: %w", d.id, err)
	}
	c := rgb(h, s, v)
	out.Colour = &lighting.Colour{R: c.r, G: c.g, B: c.b}
	out.Colours = []lighting.Colour{*out.Colour}
	return out, nil
}

/*
SetEffectWith runs the named effect with its colour and the keyboard's speed
(spec 032), each written only where it differs from what the keyboard
reports. The speed is one value for every effect that moves.
*/
func (d *device) SetEffectWith(ctx context.Context, name string, s lighting.Settings) (bool, error) {
	e, ok := byName(name)
	if !ok {
		return false, fmt.Errorf("%q on %s: %w", name, d.id, lighting.ErrNoEffect)
	}
	colours, speed, err := lighting.Fit(info(e), s, false)
	if err != nil {
		return false, fmt.Errorf("%s: %w", d.id, err)
	}
	var wrote bool
	if s.Restart {
		// The effect written again, so it starts from its beginning (spec
		// 035).
		d.mu.Lock()
		_, err = d.ask(ctx, cmdSetValue, matrix, valueEffect, e.number)
		if err == nil {
			d.sentHS, d.bright, d.found = nil, -1, nil
		}
		d.mu.Unlock()
		wrote = err == nil
	} else {
		wrote, err = d.SetEffect(ctx, e.name)
	}
	if err != nil {
		return false, err
	}
	if len(colours) > 0 {
		w, err := d.SetColour(ctx, colours[0])
		if err != nil {
			return wrote, err
		}
		wrote = wrote || w
	}
	if speed == nil {
		return wrote, nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	r, err := d.ask(ctx, cmdGetValue, matrix, valueSpeed)
	if err != nil {
		return wrote, fmt.Errorf("setting %s: %w", d.id, err)
	}
	if int(r[3]) == *speed {
		return wrote, nil
	}
	if _, err := d.ask(ctx, cmdSetValue, matrix, valueSpeed, byte(*speed)); err != nil { //nolint:gosec // 0 to 255, checked by Fit
		return wrote, fmt.Errorf("setting %s: %w", d.id, err)
	}
	return true, nil
}

/*
SetEffect runs the named effect with the keyboard's stored colour and
brightness, where it runs another. Frames drawn after it take the keyboard
again, and Release then gives back this effect.
*/
func (d *device) SetEffect(ctx context.Context, name string) (bool, error) {
	e, ok := byName(name)
	if !ok {
		return false, fmt.Errorf("%q on %s: %w", name, d.id, lighting.ErrNoEffect)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rd == nil {
		return false, fmt.Errorf("setting %s: %w", d.id.Path, os.ErrClosed)
	}
	r, err := d.ask(ctx, cmdGetValue, matrix, valueEffect)
	if err != nil {
		return false, fmt.Errorf("setting %s: %w", d.id, err)
	}
	if r[3] == e.number {
		return false, nil
	}
	if _, err := d.ask(ctx, cmdSetValue, matrix, valueEffect, e.number); err != nil {
		return false, fmt.Errorf("setting %s: %w", d.id, err)
	}
	// The frame is gone; the next one claims afresh, and records this.
	d.sentHS, d.bright, d.found = nil, -1, nil
	return true, nil
}

/*
SetColour stores c as the effects' colour: its hue and saturation, and its
value as the board's brightness. Each is written only where it differs. The
keyboard keeps it until it is unplugged; nothing is saved (spec 028).
*/
func (d *device) SetColour(ctx context.Context, c lighting.Colour) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rd == nil {
		return false, fmt.Errorf("setting %s: %w", d.id.Path, os.ErrClosed)
	}
	h, s, v, err := d.stored(ctx)
	if err != nil {
		return false, fmt.Errorf("setting %s: %w", d.id, err)
	}
	wh, ws, wv := hsv(colour{c.R, c.G, c.B})
	wrote := false
	if wh != h || ws != s {
		if _, err := d.ask(ctx, cmdSetValue, matrix, valueColour, wh, ws); err != nil {
			return false, fmt.Errorf("setting %s: %w", d.id, err)
		}
		wrote = true
	}
	if wv != v {
		if _, err := d.ask(ctx, cmdSetValue, matrix, valueBrightness, wv); err != nil {
			return wrote, fmt.Errorf("setting %s: %w", d.id, err)
		}
		d.bright = int(wv)
		wrote = true
	}
	return wrote, nil
}

// stored reads the stored hue and saturation, and the board's brightness.
// The caller holds mu.
func (d *device) stored(ctx context.Context) (h, s, v uint8, err error) {
	c, err := d.ask(ctx, cmdGetValue, matrix, valueColour)
	if err != nil {
		return 0, 0, 0, err
	}
	b, err := d.ask(ctx, cmdGetValue, matrix, valueBrightness)
	if err != nil {
		return 0, 0, 0, err
	}
	return c[3], c[4], b[3], nil
}

func byNumber(n uint8) (effect, bool) {
	for _, e := range effects {
		if e.number == n {
			return e, true
		}
	}
	return effect{}, false
}

func byName(name string) (effect, bool) {
	for _, e := range effects {
		if strings.EqualFold(e.name, strings.TrimSpace(name)) {
			return e, true
		}
	}
	return effect{}, false
}

// rgb is the keyboard's hue (256 a full turn), saturation and value as a
// colour: hsv's inverse.
func rgb(h, s, v uint8) colour {
	if s == 0 {
		return colour{v, v, v}
	}
	deg := float64(h) * 360 / 256
	vf, sf := float64(v), float64(s)/255
	c := vf * sf
	x := c * (1 - math.Abs(math.Mod(deg/60, 2)-1))
	m := vf - c
	var r, g, b float64
	switch {
	case deg < 60:
		r, g = c, x
	case deg < 120:
		r, g = x, c
	case deg < 180:
		g, b = c, x
	case deg < 240:
		g, b = x, c
	case deg < 300:
		r, b = x, c
	default:
		r, b = c, x
	}
	u := func(f float64) uint8 { return uint8(math.Round(f + m)) } //nolint:gosec // 0 to 255
	return colour{u(r), u(g), u(b)}
}
