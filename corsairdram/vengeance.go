package corsairdram

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/ushineko/sanshoku/lighting"
)

/*
The original Vengeance RGB's own effects (#122), as OpenRGB 1.1's
CorsairVengeanceController offers them: the mode register A6 takes 0 a still
colour, 1 Fade and 2 Pulse, in the colour of B0 to B2. The module runs them
itself, smoothly, with nothing more sent; a colour drawn frame by frame is
five transfers through PawnIO, 78 ms (#120).

A4 is the fade time, which OpenRGB writes as 0 for a still colour; it is the
speed here. A5, the hold time, OpenRGB never writes, and neither does this.
*/

// vengeance is an original Vengeance RGB module: a canvas of one light, and
// the module's own effects.
type vengeance struct {
	*device
	// running is the effect this handle set, an index into vengeanceEffects,
	// or -1 for the frame's colour; given and fade are its colour and speed.
	running int
	given   *colour
	fade    int
}

// vengeanceEffect is one of the module's own effects.
type vengeanceEffect struct {
	name string
	mode byte
}

var vengeanceEffects = []vengeanceEffect{
	{"Breathing", 0x02}, // OpenRGB's Pulse
	{"Fading", 0x01},    // OpenRGB's Fade
}

/*
vengeanceSpeeds are the fade times offered, slowest first, in the module's
own units, and vengeanceSpeed the one an effect starts at. Measured by eye on
the Windows test machine.
*/
var (
	vengeanceSpeeds = []int{0x40, 0x20, 0x10, 0x08}
	vengeanceSpeed  = 0x20
)

var (
	_ lighting.Canvas = (*vengeance)(nil)
	_ lighting.Zoned  = (*vengeance)(nil)
	_ lighting.Tuned  = (*vengeance)(nil)
)

// Frame shows the colour still, ending an effect this handle set.
func (v *vengeance) Frame(ctx context.Context, px []lighting.Pixel) error {
	v.mu.Lock()
	if v.running >= 0 {
		v.sent = false
	}
	v.mu.Unlock()
	if err := v.device.Frame(ctx, px); err != nil {
		return err
	}
	v.mu.Lock()
	v.running = -1
	v.mu.Unlock()
	return nil
}

func vengeanceEffectAt(name string) int {
	return slices.IndexFunc(vengeanceEffects, func(e vengeanceEffect) bool { return strings.EqualFold(e.name, name) })
}

// Effects names the module's own effects.
func (v *vengeance) Effects() []string {
	out := make([]string, len(vengeanceEffects))
	for i, e := range vengeanceEffects {
		out[i] = e.name
	}
	return out
}

// Catalogue is each effect's speeds and its one colour.
func (v *vengeance) Catalogue() []lighting.EffectInfo {
	out := make([]lighting.EffectInfo, len(vengeanceEffects))
	for i, e := range vengeanceEffects {
		out[i] = vengeanceInfo(e)
	}
	return out
}

func vengeanceInfo(e vengeanceEffect) lighting.EffectInfo {
	return lighting.EffectInfo{Name: e.name, Number: int(e.mode), Speeds: vengeanceSpeeds, Speed: vengeanceSpeed, MinColours: 1, MaxColours: 1}
}

// Effect is the effect this handle set; before one, ErrUnknownEffect.
func (v *vengeance) Effect(context.Context) (lighting.Effect, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.running < 0 {
		return lighting.Effect{}, fmt.Errorf("%s: %w", v.id, lighting.ErrUnknownEffect)
	}
	c := v.effectColour()
	colour := lighting.Colour{R: c.r, G: c.g, B: c.b}
	speed := v.fade
	return lighting.Effect{Name: vengeanceEffects[v.running].name, Coloured: true, Colour: &colour,
		Colours: []lighting.Colour{colour}, Speed: &speed}, nil
}

// SetEffectWith runs the named effect with the colour and speed given; a
// nil field keeps what this handle set. The same again sends nothing.
func (v *vengeance) SetEffectWith(_ context.Context, name string, s lighting.Settings) (bool, error) {
	at := vengeanceEffectAt(name)
	if at < 0 {
		return false, fmt.Errorf("%q: %w", name, lighting.ErrNoEffect)
	}
	colours, speed, err := lighting.Fit(vengeanceInfo(vengeanceEffects[at]), s, true)
	if err != nil {
		return false, fmt.Errorf("%s: %w", v.id, err)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	given, fade := v.given, v.fade
	if len(colours) > 0 {
		c := colour{colours[0].R, colours[0].G, colours[0].B}
		given = &c
	}
	switch {
	case speed != nil:
		fade = *speed
	case fade == 0:
		fade = vengeanceSpeed
	}
	if v.running == at && !s.Restart && samePointer(given, v.given) && fade == v.fade {
		return false, nil
	}
	v.given, v.fade = given, fade
	if err := v.run(at); err != nil {
		return false, err
	}
	return true, nil
}

// SetEffect runs a named effect at its own speed in this handle's colour.
func (v *vengeance) SetEffect(ctx context.Context, name string) (bool, error) {
	if at := vengeanceEffectAt(name); at >= 0 {
		v.mu.Lock()
		running := v.running
		v.mu.Unlock()
		if running == at {
			return false, nil
		}
	}
	return v.SetEffectWith(ctx, name, lighting.Settings{})
}

// SetColour runs the effect this handle set again in c.
func (v *vengeance) SetColour(_ context.Context, c lighting.Colour) (bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.running < 0 {
		return false, fmt.Errorf("%s: %w", v.id, lighting.ErrUnknownEffect)
	}
	next := colour{c.R, c.G, c.B}
	if v.given != nil && *v.given == next {
		return false, nil
	}
	v.given = &next
	if err := v.run(v.running); err != nil {
		return false, err
	}
	return true, nil
}

// effectColour is the colour given, else the frame's, as the module shows
// it. The caller holds mu.
func (v *vengeance) effectColour() colour {
	if v.given != nil {
		return *v.given
	}
	return v.held[0]
}

// run writes an effect: the fade time, the colour, then the mode. The
// caller holds mu.
func (v *vengeance) run(at int) error {
	c := v.effectColour()
	for _, w := range [][2]byte{
		{regVengeanceFade, byte(v.fade)}, //nolint:gosec // a listed speed, under 0x100
		{regVengeanceRed, c.r}, {regVengeanceGreen, c.g}, {regVengeanceBlue, c.b},
		{regVengeanceMode, vengeanceEffects[at].mode},
	} {
		if err := v.write(w[0], w[1]); err != nil {
			return fmt.Errorf("setting %s on %s: %w", vengeanceEffects[at].name, v.id, err)
		}
	}
	v.running, v.sent = at, false
	return nil
}

func samePointer(a, b *colour) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
