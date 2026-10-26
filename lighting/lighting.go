package lighting

import (
	"context"
	"errors"
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
}

/*
Onboard is a device whose firmware runs a lighting effect of its own, with a
colour the device stores.

The AULA F75 is the first (spec 020). It is a second way to light a device
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
