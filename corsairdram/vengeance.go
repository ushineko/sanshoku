package corsairdram

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/ushineko/sanshoku/lighting"
)

/*
The original Vengeance RGB's own effects (#122). The module runs them
itself, smoothly, with nothing more sent; a colour drawn frame by frame is
five transfers through PawnIO, 78 ms (#120).

As liquidctl documents the module and Corsair Link's captured writes show
it: A4 is the time spent changing and A5 the time held, each 1 to 63, set
equal by Corsair Link; A7 is how many of the seven colour slots (B0 to C4) an
effect uses; A6 is the mode: 0 a still colour, or with A4 non-zero one colour
breathing; 1 fading through two to seven colours; 2 breathing through two or
more. A7 left at two with red in slot two is why mode 2 with one colour
written breathed blue and then showed red (the first live run).
*/

// vengeance is an original Vengeance RGB module: a canvas of one light, and
// the module's own effects.
type vengeance struct {
	*device
	// running is the effect this handle set, an index into vengeanceEffects,
	// or -1 for the frame's colour; given and speed are its colours and
	// speed.
	running int
	given   []colour
	speed   int
}

// vengeanceEffect is one of the module's own effects.
type vengeanceEffect struct {
	name     string
	min, max int
}

var vengeanceEffects = []vengeanceEffect{
	{"Breathing", 1, vengeanceSlots}, // mode 0 for one colour, 2 for more
	{"Fading", 2, vengeanceSlots},    // mode 1
}

/*
vengeanceSpeeds are the timing values offered, slowest first, liquidctl's
presets, and vengeanceSpeed the one an effect starts at, its "normal". A4
and A5 are both set to it, as Corsair Link sets them.
*/
var (
	vengeanceSpeeds = []int{63, 48, 32, 16, 1}
	vengeanceSpeed  = 32
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

// Catalogue is each effect's speeds and colours.
func (v *vengeance) Catalogue() []lighting.EffectInfo {
	out := make([]lighting.EffectInfo, len(vengeanceEffects))
	for i, e := range vengeanceEffects {
		out[i] = vengeanceInfo(i, e)
	}
	return out
}

func vengeanceInfo(i int, e vengeanceEffect) lighting.EffectInfo {
	return lighting.EffectInfo{Name: e.name, Number: i, Speeds: vengeanceSpeeds, Speed: vengeanceSpeed, MinColours: e.min, MaxColours: e.max}
}

// Effect is the effect this handle set; before one, ErrUnknownEffect.
func (v *vengeance) Effect(context.Context) (lighting.Effect, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.running < 0 {
		return lighting.Effect{}, fmt.Errorf("%s: %w", v.id, lighting.ErrUnknownEffect)
	}
	var colours []lighting.Colour
	for _, c := range v.effectColours(vengeanceEffects[v.running].min) {
		colours = append(colours, lighting.Colour{R: c.r, G: c.g, B: c.b})
	}
	speed := v.speed
	return lighting.Effect{Name: vengeanceEffects[v.running].name, Coloured: true, Colour: &colours[0],
		Colours: colours, Speed: &speed}, nil
}

// SetEffectWith runs the named effect with the colours and speed given; a
// nil field keeps what this handle set. The same again sends nothing.
func (v *vengeance) SetEffectWith(_ context.Context, name string, s lighting.Settings) (bool, error) {
	at := vengeanceEffectAt(name)
	if at < 0 {
		return false, fmt.Errorf("%q: %w", name, lighting.ErrNoEffect)
	}
	colours, speed, err := lighting.Fit(vengeanceInfo(at, vengeanceEffects[at]), s, true)
	if err != nil {
		return false, fmt.Errorf("%s: %w", v.id, err)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	given, sp := v.given, v.speed
	if len(colours) > 0 {
		given = nil
		for _, c := range colours {
			given = append(given, colour{c.R, c.G, c.B})
		}
	}
	switch {
	case speed != nil:
		sp = *speed
	case sp == 0:
		sp = vengeanceSpeed
	}
	if v.running == at && !s.Restart && slices.Equal(given, v.given) && sp == v.speed {
		return false, nil
	}
	v.given, v.speed = given, sp
	if err := v.run(at); err != nil {
		return false, err
	}
	return true, nil
}

// SetEffect runs a named effect at its own speed in this handle's colours.
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

// SetColour runs the effect this handle set again with c as its first
// colour.
func (v *vengeance) SetColour(_ context.Context, c lighting.Colour) (bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.running < 0 {
		return false, fmt.Errorf("%s: %w", v.id, lighting.ErrUnknownEffect)
	}
	next := colour{c.R, c.G, c.B}
	if len(v.given) > 0 && v.given[0] == next {
		return false, nil
	}
	if len(v.given) == 0 {
		v.given = []colour{next}
	} else {
		v.given[0] = next
	}
	if err := v.run(v.running); err != nil {
		return false, err
	}
	return true, nil
}

/*
effectColours is the colours given, else the frame's, at least `least` of
them: the last repeated, as Fit repeats it. The caller holds mu.
*/
func (v *vengeance) effectColours(least int) []colour {
	cs := v.given
	if len(cs) == 0 {
		cs = []colour{v.held[0]}
	}
	for len(cs) < least {
		cs = append(cs, cs[len(cs)-1])
	}
	return cs
}

/*
run writes an effect as Corsair Link writes it: the timing, both values the
speed; the colours into the slots from B0; the count; then the mode. One
colour breathing is mode 0 with the count 1, as Corsair Link and liquidctl
write it. The caller holds mu.
*/
func (v *vengeance) run(at int) error {
	e := vengeanceEffects[at]
	cs := v.effectColours(e.min)
	mode := byte(0x01)
	if e.name == "Breathing" {
		mode = 0x00
		if len(cs) > 1 {
			mode = 0x02
		}
	}
	t := byte(v.speed) //nolint:gosec // 1 to 63
	ws := [][2]byte{{regVengeanceFade, t}, {regVengeanceHold, t}}
	for i, c := range cs {
		r := byte(regVengeanceRed + 3*i) //nolint:gosec // seven slots at most
		ws = append(ws, [2]byte{r, c.r}, [2]byte{r + 1, c.g}, [2]byte{r + 2, c.b})
	}
	ws = append(ws, [2]byte{regVengeanceColors, byte(len(cs))}, [2]byte{regVengeanceMode, mode}) //nolint:gosec // seven at most
	for _, w := range ws {
		if err := v.write(w[0], w[1]); err != nil {
			return fmt.Errorf("setting %s on %s: %w", e.name, v.id, err)
		}
	}
	v.running, v.sent = at, false
	return nil
}
