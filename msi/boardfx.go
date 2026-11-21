package msi

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/ushineko/sanshoku/lighting"
	"github.com/ushineko/sanshoku/pawnio"
)

/*
The header's own effects (#122), run by the Super I/O chip itself: smooth,
and nothing to send once set. OpenRGB 1.1 drives the header in Direct mode
only; the registers are as nagisa's msi-rgb documents them (ISC):

  - E4 is pbbb: p smooth pulsing; bbb the blink period, 000 always on, 001
    every light off, 010 fastest to 111 slowest.
  - F0-FB hold eight frames of four bits a channel, two frames a register,
    frame 1 in the high nibble of the first: the chip steps through them.
  - FE and FF's lowest bit are the nine-bit step time between frames.
*/

const (
	regStepTime  = 0xFE
	pulseBit     = 0x08
	stepTimeHigh = 0x01 // FF's bit 8 of the step time
)

// boardEffect is one of the header's own effects.
type boardEffect struct {
	name string
	// speeds are the effect's speeds in register values, slowest first; nil
	// for an effect with none, and speed the one it starts at.
	speeds   []int
	speed    int
	min, max int
}

var boardEffects = []boardEffect{
	// The chip's pulse, in one colour. Its pace is the chip's own.
	{"Breathing", nil, 0, 1, 1},
	// A blink, at the six periods E4 offers.
	{"Flashing", []int{7, 6, 5, 4, 3, 2}, 4, 1, 1},
	// Up to eight colours, the chip stepping from one to the next. The step
	// time is in the chip's own units.
	{"Color Cycle", []int{0x1FF, 0x100, 0x80, 0x40, 0x20}, 0x80, 1, 8},
}

var _ lighting.Tuned = (*boardDevice)(nil)

func (e boardEffect) info(i int) lighting.EffectInfo {
	return lighting.EffectInfo{Name: e.name, Number: i, Speeds: e.speeds, Speed: e.speed, MinColours: e.min, MaxColours: e.max}
}

// Effects names the header's own effects.
func (d *boardDevice) Effects() []string {
	out := make([]string, len(boardEffects))
	for i, e := range boardEffects {
		out[i] = e.name
	}
	return out
}

// Catalogue is each effect's speeds and colours.
func (d *boardDevice) Catalogue() []lighting.EffectInfo {
	out := make([]lighting.EffectInfo, len(boardEffects))
	for i, e := range boardEffects {
		out[i] = e.info(i)
	}
	return out
}

func boardEffectAt(name string) int {
	return slices.IndexFunc(boardEffects, func(e boardEffect) bool { return strings.EqualFold(e.name, name) })
}

/*
Effect is the effect this handle set, with its colours. The chip is not read
for it, so before this handle has set one it is lighting.ErrUnknownEffect.
*/
func (d *boardDevice) Effect(context.Context) (lighting.Effect, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running < 0 {
		return lighting.Effect{}, fmt.Errorf("%s: %w", d.id, lighting.ErrUnknownEffect)
	}
	e := boardEffects[d.running]
	out := lighting.Effect{Name: e.name, Coloured: true}
	for _, c := range d.effectColours() {
		out.Colours = append(out.Colours, lighting.Colour{R: c.r, G: c.g, B: c.b})
	}
	out.Colour = &out.Colours[0]
	if e.speeds != nil {
		speed := d.speed
		out.Speed = &speed
	}
	return out, nil
}

/*
SetEffectWith runs the named effect with the colours and speed given; a nil
field keeps what this handle set. The same settings again send nothing,
unless asked to restart.
*/
func (d *boardDevice) SetEffectWith(_ context.Context, name string, s lighting.Settings) (bool, error) {
	at := boardEffectAt(name)
	if at < 0 {
		return false, fmt.Errorf("%q: %w", name, lighting.ErrNoEffect)
	}
	colours, speed, err := lighting.Fit(boardEffects[at].info(at), s, true)
	if err != nil {
		return false, fmt.Errorf("%s: %w", d.id, err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	given, sp := d.given, d.speed
	if len(colours) > 0 {
		given = nil
		for _, c := range colours {
			given = append(given, colour{c.R, c.G, c.B})
		}
	}
	if speed != nil {
		sp = *speed
	}
	if d.running != at {
		if speed == nil {
			sp = boardEffects[at].speed
		}
	}
	if d.running == at && !s.Restart && slices.Equal(given, d.given) && sp == d.speed {
		return false, nil
	}
	d.given, d.speed = given, sp
	if err := d.run(at); err != nil {
		return false, err
	}
	return true, nil
}

// SetEffect runs a named effect at its own speed in this handle's colours.
// The same effect again sends nothing.
func (d *boardDevice) SetEffect(ctx context.Context, name string) (bool, error) {
	at := boardEffectAt(name)
	if at < 0 {
		return false, fmt.Errorf("%q: %w", name, lighting.ErrNoEffect)
	}
	d.mu.Lock()
	running := d.running
	d.mu.Unlock()
	if running == at {
		return false, nil
	}
	speed := boardEffects[at].speed
	if boardEffects[at].speeds == nil {
		return d.SetEffectWith(ctx, name, lighting.Settings{})
	}
	return d.SetEffectWith(ctx, name, lighting.Settings{Speed: &speed})
}

// SetColour runs the effect this handle set again with c as its first
// colour.
func (d *boardDevice) SetColour(_ context.Context, c lighting.Colour) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running < 0 {
		return false, fmt.Errorf("%s: %w", d.id, lighting.ErrUnknownEffect)
	}
	next := colour{c.R, c.G, c.B}
	if len(d.given) > 0 && d.given[0] == next {
		return false, nil
	}
	if len(d.given) == 0 {
		d.given = []colour{next}
	} else {
		d.given[0] = next
	}
	if err := d.run(d.running); err != nil {
		return false, err
	}
	return true, nil
}

// effectColours is the running effect's colours: those given, else the
// frame's colour, else red. The caller holds mu.
func (d *boardDevice) effectColours() []colour {
	switch {
	case len(d.given) > 0:
		return d.given
	case d.sent:
		return []colour{d.held}
	}
	return []colour{{0xFF, 0, 0}}
}

/*
run writes an effect: the chip readied as a frame readies it, the eight
frames (the colours in turn for Color Cycle, else the one colour), the step
time and E4. Every register is the header's own; nothing is stored. The
caller holds mu.
*/
func (d *boardDevice) run(at int) error {
	if d.sio == nil {
		return fmt.Errorf("%s: %w", d.id, os.ErrClosed)
	}
	e := boardEffects[at]
	cs := d.effectColours()
	if e.max == 1 {
		cs = cs[:1]
	}
	var frames [8]colour
	for i := range frames {
		frames[i] = cs[i%len(cs)]
	}
	var e4 byte
	var stepTime int
	switch e.name {
	case "Breathing":
		e4 = pulseBit
	case "Flashing":
		e4 = byte(d.speed) & 0x07 //nolint:gosec // 2 to 7
	case "Color Cycle":
		stepTime = d.speed
	}
	err := d.sio.In(func(c pawnio.Chip) error {
		if !d.claimed {
			if err := d.claim(c); err != nil {
				return err
			}
		}
		if err := c.Outb(sioLogicalDevice, ldnRGB); err != nil {
			return err //nolint:wrapcheck // pawnio names it
		}
		for ch := range 3 {
			for i := range 4 {
				lo, hi := nibble(frames[2*i], ch), nibble(frames[2*i+1], ch)
				if err := c.Outb(regRGBRedFirst+byte(ch*4+i), hi<<4|lo); err != nil { //nolint:gosec // 12 registers
					return err //nolint:wrapcheck // pawnio names it
				}
			}
		}
		return d.effectRegisters(c, e4, stepTime)
	})
	if err != nil {
		return fmt.Errorf("setting %s on %s: %w", e.name, d.id, err)
	}
	d.claimed, d.running, d.sent = true, at, false
	return nil
}

// effectRegisters writes E4, the step time and FF, the RGB logical device
// selected. FF keeps the header on and the board's colour order.
func (d *boardDevice) effectRegisters(c pawnio.Chip, e4 byte, stepTime int) error {
	cfg := byte(rgbConfig3)
	if d.invert {
		cfg |= rgbConfig3Swap
	}
	if stepTime>>8&1 == 1 {
		cfg |= stepTimeHigh
	}
	for _, w := range [][2]byte{{regRGBConfig1, e4}, {regStepTime, byte(stepTime & 0xFF)}, {regRGBConfig3, cfg}} { //nolint:gosec // eight bits
		if err := c.Outb(w[0], w[1]); err != nil {
			return err //nolint:wrapcheck // pawnio names it
		}
	}
	return nil
}

// nibble is a channel's top four bits: 0 red, 1 green, 2 blue.
func nibble(c colour, ch int) byte {
	return [3]byte{c.r, c.g, c.b}[ch] >> 4
}
