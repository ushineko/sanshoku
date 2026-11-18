package nzxt

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"time"

	"github.com/ushineko/sanshoku/screen"
)

/*
Streamed frames: the panel's second path, which NZXT CAM uses for anything
that moves (spec 037, issue #27).

Per frame, on the control channel and the bulk endpoint, as CAM 4.76.9 sends
it on firmware 1.2.12:

	36 01 00 01 09        answered 37 01 in about 2 ms
	bulk: header, 09 00 00 00, <payload length LE32>
	bulk: the payload, 640 x 640 x 3 bytes of RGB, row by row
	36 02                 answered 37 02 in 3 to 20 ms

Nothing is stored and no bucket is involved. A frame shows at once, in full
colour, and about ten a second go through back to back. CAM also sends `36 04`
and `36 03` once when it starts; the bench measured no difference without
them, so they are not sent.

CAM also sends format `08`, compressed RGB565 behind a "q565" header, at about
twenty frames a second. Its encoding is not written down yet, and raw frames
cover everything drawn so far, so it is not offered.
*/

/*
frameRGB888 is the format byte for a raw frame: three bytes a pixel, red
first, rows top to bottom, no padding (1228800 bytes on a 640-pixel panel,
which is whole 1024-byte packets anyway). Read off CAM's capture.
*/
const frameRGB888 = 0x09

/*
frameHold is how long a caller may wait before sending a frame again.

The firmware takes the panel back for its readout 30 s after the last frame,
unasked (bench, 2026-11-18, timed by a person watching). A frame sent again
every 20 s held for two minutes with nothing visible at the resends. 25 s
leaves room for a caller's own tick to run late.
*/
const frameHold = 25 * time.Second

// framePanel is a panel whose model takes streamed frames.
type framePanel struct {
	*panelDevice
}

var _ screen.Framer = (*framePanel)(nil)

// FrameHold is the longest a caller may wait between frames for a still
// picture to stay up.
func (p *framePanel) FrameHold() time.Duration { return frameHold }

/*
Frame draws one picture and returns when the device answers `36 02`.

Once a frame is up no bucket is on screen, so the next Image may use any of
them, and Readout and Close hand the panel back as they do after an image.
*/
func (p *framePanel) Frame(ctx context.Context, img image.Image) error {
	if img == nil || img.Bounds().Empty() {
		return errors.New("a frame with no pixels")
	}
	payload := rgb888(img, p.model.Screen)

	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.panel(); err != nil {
		return err
	}
	reply, err := p.exchange(ctx, 0x36, 0x01, 0x00, 0x01, frameRGB888)
	if err != nil {
		return fmt.Errorf("start the frame: %w", err)
	}
	if reply[resultByte] != resultOK {
		return fmt.Errorf("the cooler refused the frame: reply %#02x", reply[resultByte])
	}
	info := make([]byte, 8)
	info[0] = frameRGB888
	binary.LittleEndian.PutUint32(info[4:], uint32(len(payload))) //nolint:gosec // the panel's size, a few MB at most
	if err := p.usb.Bulk(ctx, bulkEndpoint, append(append([]byte(nil), header...), info...)); err != nil {
		return err
	}
	if err := p.usb.Bulk(ctx, bulkEndpoint, payload); err != nil {
		return err
	}
	reply, err = p.exchange(ctx, 0x36, 0x02)
	if err != nil {
		return fmt.Errorf("finish the frame: %w", err)
	}
	if reply[resultByte] != resultOK {
		return fmt.Errorf("the cooler refused the frame's end: reply %#02x", reply[resultByte])
	}
	// The frame covers whatever bucket was showing; that slot is free again.
	p.showing = -1
	return nil
}

/*
rgb888 is the picture as the panel takes it, scaled by nearest neighbour where
its size differs, as fit scales a GIF.

Colour is read through image.Image's At, which hands back premultiplied
values: a transparent pixel is black, the panel's off.
*/
func rgb888(img image.Image, panel Size) []byte {
	b := img.Bounds()
	out := make([]byte, 0, panel.W*panel.H*3)
	if rgba, ok := img.(*image.RGBA); ok && b.Dx() == panel.W && b.Dy() == panel.H {
		for y := range panel.H {
			row := rgba.Pix[y*rgba.Stride : y*rgba.Stride+panel.W*4]
			for x := 0; x < len(row); x += 4 {
				out = append(out, row[x], row[x+1], row[x+2])
			}
		}
		return out
	}
	for y := range panel.H {
		sy := b.Min.Y + y*b.Dy()/panel.H
		for x := range panel.W {
			sx := b.Min.X + x*b.Dx()/panel.W
			r, g, bl, _ := img.At(sx, sy).RGBA()
			out = append(out, byte(r>>8), byte(g>>8), byte(bl>>8)) //nolint:gosec // RGBA is 16-bit, so the high byte fits
		}
	}
	return out
}
