package nzxt

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ushineko/sanshoku/lighting"
)

/*
The cooler's firmware effects, as lighting.Onboard and lighting.Catalogue
(spec 025).

Each is one `2A 04` command per channel carrying the effect, its speed and its
colours; the cooler stores none of them, and reports none back. So the driver
keeps, per handle, the effect it set and the colour it was last given, and
`Effect` reports that.

Every number in the table is NZXT CAM's, read off a capture on firmware
1.2.12, with the speed lists and colour ranges from OpenRGB's Hue 2 driver
where it has them (docs/kraken-lighting.md).
*/

// effect is one firmware effect: what CAM sends for it, and what it takes.
type effect struct {
	name  string
	mode  byte
	speed int
	// speeds are the speeds known to work, slowest (largest) first.
	speeds []int
	// colours are CAM's, in R, G, B; their count is how many slots CAM fills.
	colours []lighting.Colour
	// minColours and maxColours are what the effect takes.
	minColours, maxColours int
	// b37, b39 and b3b are bytes 0x37, 0x39 and 0x3B as CAM sends them.
	b37, b39, b3b byte
}

var (
	camPurple = lighting.Colour{R: 0x39, G: 0x00, B: 0x58}
	camGreen  = lighting.Colour{R: 0x00, G: 0xBC, B: 0x00}
	camRed    = lighting.Colour{R: 0xBC, G: 0x00, B: 0x00}
	camBlue   = lighting.Colour{R: 0x00, G: 0x00, B: 0xBC}
	camTeal   = lighting.Colour{R: 0x00, G: 0xBC, B: 0x83}
	camAmber  = lighting.Colour{R: 0xBC, G: 0x78, B: 0x00}
	rainbow   = []int{350, 300, 250, 150, 80}
)

// effects are the cooler's effects in mode order.
var effects = []effect{
	{"Fading", 0x01, 40, []int{80, 60, 40, 20, 10}, []lighting.Colour{camPurple, camGreen, camBlue}, 1, 8, 0x00, 0x08, 0x03},
	{"Spectrum Wave", 0x02, 250, rainbow, nil, 0, 0, 0x00, 0x00, 0x03},
	{"Covering Marquee", 0x04, 150, []int{150}, []lighting.Colour{camBlue, camGreen}, 1, 8, 0x00, 0x00, 0x03},
	{"Alternating", 0x05, 500, []int{1600, 1300, 1000, 800, 600, 500}, []lighting.Colour{camPurple, camTeal}, 1, 2, 0x01, 0x00, 0x04},
	{"Pulse", 0x06, 15, []int{25, 20, 15, 7, 4}, []lighting.Colour{camPurple}, 1, 8, 0x00, 0x08, 0x03},
	{"Breathing", 0x07, 20, []int{40, 30, 20, 10, 4}, []lighting.Colour{camPurple}, 1, 8, 0x00, 0x08, 0x03},
	{"Candle", 0x08, 50, []int{50}, []lighting.Colour{camAmber}, 1, 8, 0x00, 0x00, 0x03},
	{"Starry Night", 0x09, 15, []int{25, 20, 15, 7, 4}, []lighting.Colour{camPurple}, 1, 1, 0x01, 0x00, 0x03},
	{"Rainbow Flow", 0x0B, 250, rainbow, nil, 0, 0, 0x00, 0x00, 0x03},
	{"Super Rainbow", 0x0C, 150, rainbow, nil, 0, 0, 0x00, 0x00, 0x03},
	{"Rainbow Pulse", 0x0D, 250, rainbow, nil, 0, 0, 0x00, 0x00, 0x03},
	{"Tai Chi", 0x0E, 30, []int{30}, []lighting.Colour{camRed, camBlue}, 2, 2, 0x00, 0x05, 0x03},
	{"Water Cooler", 0x0F, 30, []int{30}, []lighting.Colour{camRed, camBlue}, 2, 2, 0x00, 0x05, 0x03},
	{"Loading", 0x10, 10, []int{10}, []lighting.Colour{camPurple}, 1, 1, 0x00, 0x04, 0x03},
}

var (
	_ lighting.Onboard   = (*device)(nil)
	_ lighting.Catalogue = (*device)(nil)
	_ lighting.Onboard   = (*panelDevice)(nil)
	_ lighting.Catalogue = (*panelDevice)(nil)
)

/*
firmwareState is the effect this handle set, and the colour it was last
given. running is -1 before an effect is set.
*/
type firmwareState struct {
	running int
	colour  *lighting.Colour
}

// Effects names the cooler's effects in mode order.
func (d *device) Effects() []string {
	out := make([]string, len(effects))
	for i, e := range effects {
		out[i] = e.name
	}
	return out
}

// Catalogue describes each effect: its number, speeds and colours.
func (d *device) Catalogue() []lighting.EffectInfo {
	out := make([]lighting.EffectInfo, len(effects))
	for i, e := range effects {
		out[i] = lighting.EffectInfo{
			Name: e.name, Number: int(e.mode), Speeds: append([]int(nil), e.speeds...), Speed: e.speed,
			MinColours: e.minColours, MaxColours: e.maxColours,
		}
	}
	return out
}

/*
Effect is the effect this handle last set, with its colour where it takes
one. The cooler does not report its effect, so before this handle has set one
it is lighting.ErrUnknownEffect. Nothing is sent.
*/
func (d *device) Effect(context.Context) (lighting.Effect, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.hid == nil {
		return lighting.Effect{}, fmt.Errorf("reading %s's effect: %w", d.id.Path, os.ErrClosed)
	}
	if !d.fx.set() {
		return lighting.Effect{}, fmt.Errorf("%s: %w", d.id, lighting.ErrUnknownEffect)
	}
	e := effects[d.fx.running]
	out := lighting.Effect{Name: e.name, Coloured: len(e.colours) > 0}
	if cs := d.fx.coloursFor(e); len(cs) > 0 {
		c := cs[0]
		out.Colour = &c
	}
	return out, nil
}

/*
SetEffect runs the named effect on every channel with keys, at CAM's speed,
in the colour this handle was last given or else CAM's. It sends nothing when
this handle already set that effect. The channels' held frames are forgotten,
so the commit before a readout does not replace the effect.
*/
func (d *device) SetEffect(ctx context.Context, name string) (bool, error) {
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
	if d.hid == nil {
		return false, fmt.Errorf("setting %s's effect: %w", d.id.Path, os.ErrClosed)
	}
	if d.fx.running == at {
		return false, nil
	}
	if err := d.runEffect(ctx, at, d.fx.colour); err != nil {
		return false, err
	}
	return true, nil
}

/*
SetColour runs the effect this handle set again, with c as its first colour
and CAM's in the rest, and keeps c for later effects. An effect that draws its own
colours is lighting.ErrNoColour; no effect set is lighting.ErrUnknownEffect.
*/
func (d *device) SetColour(ctx context.Context, c lighting.Colour) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.hid == nil {
		return false, fmt.Errorf("setting %s's colour: %w", d.id.Path, os.ErrClosed)
	}
	if !d.fx.set() {
		return false, fmt.Errorf("%s: %w", d.id, lighting.ErrUnknownEffect)
	}
	e := effects[d.fx.running]
	if len(e.colours) == 0 {
		return false, fmt.Errorf("%s: %w", e.name, lighting.ErrNoColour)
	}
	if d.fx.colour != nil && *d.fx.colour == c {
		return false, nil
	}
	if err := d.runEffect(ctx, d.fx.running, &c); err != nil {
		return false, err
	}
	return true, nil
}

/*
runEffect sends effect at to every channel with keys, and records it. The
caller holds mu.
*/
func (d *device) runEffect(ctx context.Context, at int, colour *lighting.Colour) error {
	if err := d.knowChannels(ctx); err != nil {
		return fmt.Errorf("setting %s's effect: %w", d.id, err)
	}
	next := firmwareState{running: at, colour: colour}
	e := effects[at]
	for i := range d.canvas.ch {
		c := &d.canvas.ch[i]
		if len(c.held) == 0 {
			continue
		}
		if err := d.acked(ctx, commitWait, effectReport(i, e, next.coloursFor(e), len(c.held))); err != nil {
			return fmt.Errorf("setting %s's effect: %w", d.id, err)
		}
		clear(c.held)
		c.sent, c.committed, c.drawn = nil, nil, false
	}
	d.fx = next
	return nil
}

// set is whether this handle has set an effect.
func (f firmwareState) set() bool { return f.running >= 0 }

/*
coloursFor is the colours effect e runs in: CAM's, with the colour last given
in the first slot. Only the first: Tai Chi with both its colours the same is a
still ring, as the maintainer saw through hotaru (spec 025).
*/
func (f firmwareState) coloursFor(e effect) []lighting.Colour {
	out := append([]lighting.Colour(nil), e.colours...)
	if f.colour != nil && len(out) > 0 {
		out[0] = *f.colour
	}
	return out
}

/*
effectReport is the `2A 04` that runs effect e on channel i of n LEDs: the
channel twice, the mode, the speed in 16 bits little-endian, up to eight
colours in G, R, B, and bytes 0x37 to 0x3B as CAM sends them.
*/
func effectReport(i int, e effect, colours []lighting.Colour, n int) []byte {
	ch := channelByte(i)
	r := lightReport(0x2A, 0x04, ch, ch, e.mode, byte(e.speed), byte(e.speed>>8)) //nolint:gosec // speeds fit 16 bits
	for k, c := range colours {
		if k == 8 {
			break
		}
		r[7+3*k], r[8+3*k], r[9+3*k] = c.G, c.R, c.B
	}
	r[0x37], r[0x38], r[0x39], r[0x3A], r[0x3B] = e.b37, byte(len(colours)), e.b39, byte(n), e.b3b //nolint:gosec // at most 8 colours and 40 LEDs
	return r
}
