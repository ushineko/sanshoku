package screen

import (
	"context"
	"image/gif"
	"time"
)

/*
Panel is a device with a display a program can draw on.

Floor is part of the device rather than the program because it is a property
of the device. On the Kraken hotaru measured, a frame pushed faster than its
floor is accepted (every transfer and the switch that shows it report
success) and the screen does not change. Nothing in the protocol announces
this, so a consumer asks the device before it pushes.
*/
type Panel interface {
	// Size is the panel's width and height in pixels.
	Size() (w, h int)

	// Image draws an image, animated or not, and shows it.
	Image(ctx context.Context, g *gif.GIF) error

	// Readout returns the panel to the firmware's own display, which is what
	// a program leaves behind when it stops.
	Readout(ctx context.Context) error

	// Appearance sets brightness as a percentage and rotation in degrees.
	Appearance(ctx context.Context, brightness, degrees int) error

	// Floor is the shortest interval at which a frame of the given encoded
	// size, in bytes, reliably appears.
	Floor(bytes int) time.Duration
}
