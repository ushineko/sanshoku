package screen

import (
	"context"
	"errors"
	"image"
	"image/gif"
	"time"
)

/*
ErrNoPanel is a device that is here and whose panel cannot be drawn on.

A driver wraps it around whatever the kernel said, and around
sanshoku.ErrAbsent, because the cause is somebody's machine (a usbfs node
this user may not open, another program holding the interface) and the
caller's question is the same in every case: there is no screen to draw on,
and the device's other capabilities still work. A consumer treats it the way
it treats absence. From hotaru's ErrNoScreen.
*/
var ErrNoPanel = errors.New("no panel on this device")

/*
Panel is a device with a display a program can draw on.

ImageFloor is part of the device rather than the program because it is a
property of the device. On the Kraken hotaru measured, a frame pushed faster
than its floor is accepted (every transfer and the switch that shows it report
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

	// ImageFloor is the shortest interval at which a frame of the given
	// encoded size, in bytes, reliably appears. It is not called Floor
	// because lighting.Canvas has a Floor of its own, and a device with a
	// screen and lights (the Kraken) is both (spec 024).
	ImageFloor(bytes int) time.Duration
}

/*
Streamer is a panel that also takes pictures one frame at a time, without
storing them on the device (spec 037).

A frame is not kept. The firmware shows it until StreamHold has passed with
nothing newer, then takes the panel back for its own readout, unasked. A
caller that wants a still picture to stay sends it again within StreamHold;
the driver keeps no timer of its own. Image is still the way to show a picture
that plays by itself.

Not called Frame, for the reason ImageFloor is not called Floor: a device with
a screen and lights is both a Streamer and a lighting.Canvas, whose Frame takes
pixels. A method of each name hid the canvas's, and the Kraken Elite stopped
being a canvas (found on hotaru's spec 080 live run).
*/
type Streamer interface {
	// Stream draws one picture and returns when the device has taken it. A
	// picture of another size is scaled to the panel. The device's answer
	// paces a caller that sends frame after frame; there is no floor.
	Stream(ctx context.Context, img image.Image) error

	// StreamHold is the longest a caller may wait before sending a frame
	// again, if the picture is to stay up.
	StreamHold() time.Duration
}

/*
Mover is a Streamer that also takes frames made for motion: fewer colours,
far fewer bytes, and the device's full rate (spec 038).

On the Kraken Elite a Move is a Q565 frame, 16-bit colour, at about 20 a
second; a Stream is 24-bit colour at about 10. A caller drawing something that
moves calls Move; one drawing a still picture where every colour counts calls
Stream. Frames of the two may follow each other in any order, and StreamHold
applies to both.

Not called Frame or Floor, for the reason Streamer is not: those names are
lighting.Canvas's, and a device with a screen and lights is both.
*/
type Mover interface {
	Streamer

	// Move draws one picture as a motion frame and returns when the device
	// has taken it. A picture of another size is scaled to the panel.
	Move(ctx context.Context, img image.Image) error
}
