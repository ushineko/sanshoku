package lighting

import (
	"context"
	"errors"
	"fmt"
	"time"
)

/*
ErrNoCanvas is a device that is here and whose lights cannot be drawn on.

A driver wraps it the way screen.ErrNoPanel is wrapped: the device's other
capabilities still work, and a consumer treats it as it treats absence.
*/
var ErrNoCanvas = errors.New("no canvas on this device")

// Key is one addressable light, numbered as the device numbers it.
type Key struct {
	// ID is the device's id for the light; a HID usage on the Apex.
	ID byte
	// Name is the key's name where the driver knows it, else "".
	Name string
}

// Pixel is one light's colour for one frame. Colours are absolute: a
// brightness is the consumer's multiply, as it is in SteelSeries GG.
type Pixel struct {
	ID      byte
	R, G, B uint8
}

/*
Canvas is a device whose lights a program draws by streaming frames.

On the device this was written for, the SteelSeries Apex Pro TKL Wireless
Gen 3, that is the only way to light it: its vendor's own software renders
every effect on the host and streams the result. A Canvas therefore has no
effect, mode or brightness; it has a frame. A wireless device streamed to
spends its battery faster while an effect runs.
*/
type Canvas interface {
	// Keys lists the lights a frame may address, in the device's order.
	Keys() []Key

	// Frame shows one frame. A light not in px is left as it was.
	Frame(ctx context.Context, px []Pixel) error

	// Release hands the lighting back to the firmware.
	Release(ctx context.Context) error

	// Floor is the shortest interval at which frames reliably show.
	Floor() time.Duration
}

/*
Fading is a Canvas whose frame does not last: left alone, the device goes back
to its own lighting.

The AULA F75 is the first (spec 019). Its vendor's program streams all the
time, and the firmware's own effect is back a moment after the stream stops.
A consumer that wants a still frame kept up sends it again, on its own clock;
this module starts no goroutine to do it. On a wireless device that is radio
traffic for as long as the frame is wanted.

A Canvas that is not Fading holds its last frame until the next.
*/
type Fading interface {
	// Hold is how long a frame is shown before the device takes its lights
	// back. A consumer holding a still frame sends it again sooner than
	// this.
	Hold() time.Duration
}

// ErrNoColour is a firmware effect that takes no colour, such as off. A
// driver wraps it with the effect's name.
var ErrNoColour = errors.New("the effect takes no colour")

// ErrNoEffect is an effect name the device's firmware does not have. A
// driver wraps it with the name asked for.
var ErrNoEffect = errors.New("no such effect")

// ErrUnknownEffect is a device that cannot say which effect it runs: the
// Kraken reports none, and its driver knows only what it set (spec 025).
var ErrUnknownEffect = errors.New("the device does not report its running effect")

// Colour is one colour, absolute.
type Colour struct{ R, G, B uint8 }

// Effect is a firmware effect as the device reports it.
type Effect struct {
	// Name is the effect's name, as the vendor's program calls it.
	Name string
	// Coloured is whether the effect takes a colour. Off does not.
	Coloured bool
	// Colour is the effect's colour, or nil where it shows several.
	Colour *Colour
	// Colours are all of the effect's colours, first to last, where the
	// device or the handle knows them; Colour is then their first (spec
	// 032).
	Colours []Colour
	// Speed is the effect's speed in its catalogue's units, where known.
	Speed *int
}

/*
Onboard is a device whose firmware runs a lighting effect of its own, with a
colour the device stores.

The AULA F75 is the first (spec 023). It is a second way to light a device
that is also a Canvas, not a replacement for it: a consumer asks for the
effect's colour, or draws frames, and frames cover the effect while they run.
Setting a colour writes the device's storage, so the method writes only where
the stored colour differs, and a consumer asks only when a person has asked.
*/
type Onboard interface {
	// Effects names the effects the firmware runs, in the device's order.
	Effects() []string

	// Effect reads the effect the firmware is running now.
	Effect(ctx context.Context) (Effect, error)

	// SetEffect makes the firmware run the named effect, matched without
	// regard to case, with that effect's own stored settings. It writes the
	// device only where the running effect differs, and says whether it
	// wrote.
	SetEffect(ctx context.Context, name string) (wrote bool, err error)

	// SetColour makes the running effect show c and keeps the effect. It
	// writes the device only where the stored colour differs, and says
	// whether it wrote.
	SetColour(ctx context.Context, c Colour) (wrote bool, err error)
}

// EffectInfo is one firmware effect as a Catalogue describes it.
type EffectInfo struct {
	// Name is the effect's name, as Effects gives it.
	Name string
	// Number is the device's own number for the effect.
	Number int
	// Speeds are the speeds the effect is known to run at, slowest first,
	// in the device's own units, and Speed the one it runs at.
	Speeds []int
	Speed  int
	// MinColours and MaxColours are the fewest and most colours it takes;
	// zero for an effect that draws its own.
	MinColours, MaxColours int
}

/*
Catalogue is an Onboard device that describes its effects: each one's number,
speeds and colours. The Kraken is the first (spec 025): it takes an effect's
speed and colours with every command rather than storing them, so a consumer
that wants more than a name asks here.
*/
type Catalogue interface {
	Catalogue() []EffectInfo
}

// Zone is a run of a canvas's lights that is one thing: the board's own, a
// header's strip, a fan chain.
type Zone struct {
	// Name is the zone's name, as the device's own software calls it.
	Name string
	// First and Count are the zone's lights, by their position in Keys.
	First, Count int
	// Min and Max are the fewest and most lights a consumer may give a zone
	// whose length it sets; both zero for a zone the device knows.
	Min, Max int
	// Shape is how the zone's lights are laid out, where the device knows
	// (spec 032). An addressable header's is not known.
	Shape Shape
	// Parts are the lights of each thing chained in the zone, first to
	// last, where the device knows them: the Kraken's fans are 8, 8 and 8.
	Parts []int
}

// Shape is how a zone's lights are laid out. The zero value is not known.
type Shape string

// The shapes a zone can have.
const (
	ShapeSingle Shape = "single" // one light
	ShapeLine   Shape = "line"   // lights in a row: a strip
	ShapeRing   Shape = "ring"   // lights in a circle: a fan, a pump's ring
	ShapeGrid   Shape = "grid"   // rows and columns: a keyboard
	ShapeSet    Shape = "set"    // lights in no order: a mouse's
)

// Resizable is whether a consumer sets the zone's length.
func (z Zone) Resizable() bool { return z.Max > 0 }

/*
Zoned is a Canvas that says which of its lights are which zone, and takes the
length of a zone it cannot read from the consumer: an addressable header does
not know what is plugged into it (spec 027). A resizable zone starts at its
Min. Keys and Zones change with Resize.
*/
type Zoned interface {
	Zones() []Zone
	// Resize sets a resizable zone's length. A zone that is not resizable, a
	// name that is no zone, or a length outside Min to Max is an error and
	// changes nothing. The zone's held colours are kept as far as they go.
	Resize(zone string, lights int) error
}

/*
Settings are an effect's colours and speed, for Tuned (spec 032). A nil field
keeps what the effect has.
*/
type Settings struct {
	// Colours are the effect's colours, first to last.
	Colours []Colour
	// Speed is in the effect's catalogue units (EffectInfo.Speeds).
	Speed *int
}

/*
Tuned is an Onboard device whose effects take their colours and speed, as its
Catalogue describes them (spec 032).

SetEffectWith runs the named effect with the settings given, and writes only
what differs from what this handle set or read. Fewer colours than the
effect's MinColours repeat the last; more than its MaxColours are
ErrTooManyColours, and any for an effect that takes none ErrNoColour. A speed
outside the lowest and highest of the effect's Speeds is ErrSpeed; a device
that takes only the listed values uses the nearest.
*/
type Tuned interface {
	Onboard
	Catalogue
	SetEffectWith(ctx context.Context, name string, s Settings) (wrote bool, err error)
}

// ErrTooManyColours is more colours than an effect takes.
var ErrTooManyColours = errors.New("more colours than the effect takes")

// ErrSpeed is a speed outside an effect's range.
var ErrSpeed = errors.New("a speed outside the effect's range")

/*
Fit checks s against an effect's catalogue entry, and returns its colours with
fewer than MinColours repeated up to it, and its speed, the nearest listed
where nearest is set. A driver calls it before it writes.
*/
func Fit(info EffectInfo, s Settings, nearest bool) ([]Colour, *int, error) {
	colours := append([]Colour(nil), s.Colours...)
	switch {
	case len(colours) > 0 && info.MaxColours == 0:
		return nil, nil, fmt.Errorf("%s: %w", info.Name, ErrNoColour)
	case len(colours) > info.MaxColours:
		return nil, nil, fmt.Errorf("%s takes %d colours, not %d: %w", info.Name, info.MaxColours, len(colours), ErrTooManyColours)
	}
	for len(colours) > 0 && len(colours) < info.MinColours {
		colours = append(colours, colours[len(colours)-1])
	}
	if s.Speed == nil {
		return colours, nil, nil
	}
	if len(info.Speeds) == 0 {
		return nil, nil, fmt.Errorf("%s has no speed: %w", info.Name, ErrSpeed)
	}
	lo, hi := info.Speeds[0], info.Speeds[0]
	for _, v := range info.Speeds {
		lo, hi = min(lo, v), max(hi, v)
	}
	speed := *s.Speed
	if speed < lo || speed > hi {
		return nil, nil, fmt.Errorf("%s takes a speed from %d to %d, not %d: %w", info.Name, lo, hi, speed, ErrSpeed)
	}
	if nearest {
		best := info.Speeds[0]
		for _, v := range info.Speeds {
			if abs(v-speed) < abs(best-speed) {
				best = v
			}
		}
		speed = best
	}
	return colours, &speed, nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// ErrNoZone is a zone name a device does not have, or one it does not let a
// consumer resize. A driver wraps it with the name.
var ErrNoZone = errors.New("no such resizable zone")
