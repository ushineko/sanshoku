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
