package nzxt

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/gif"
)

/*
fit encodes a GIF at the size the panel takes, scaling it if it is not already.

**A GIF of the wrong size displays as nothing at all.** Not an error, not a
refusal, not a garbled picture: the transfer succeeds, the bucket switch
succeeds, and the screen goes blank. Four of hotaru's nine animations were
640x640 and played; the other five were 480x480 and were silently blank, which
is how the panel says "no" (hotaru spec 016).

Scaled in palette space, by nearest neighbour. That is the cheap way and here
it is also the honest one: each frame keeps its own palette exactly, so nothing
is re-quantised and nothing grows. It is blocky under a magnifying glass and
indistinguishable at arm's length on a 640-pixel circle.

The caller's GIF is not modified. Its size is its Config's, or, where the
Config is unset (a GIF built in memory rather than decoded), its first frame's.
*/
func fit(g *gif.GIF, panel Size) ([]byte, error) {
	if g == nil || len(g.Image) == 0 {
		return nil, errors.New("an image with no frames")
	}
	w, h := g.Config.Width, g.Config.Height
	if w == 0 || h == 0 {
		b := g.Image[0].Bounds()
		w, h = b.Max.X, b.Max.Y
	}
	if w <= 0 || h <= 0 {
		return nil, errors.New("the image reports no size")
	}

	out := *g
	out.Config.Width, out.Config.Height = panel.W, panel.H
	if w != panel.W || h != panel.H {
		out.Image = make([]*image.Paletted, len(g.Image))
		for i, frame := range g.Image {
			out.Image[i] = stretch(frame, w, h, panel)
		}
	}

	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, &out); err != nil {
		return nil, fmt.Errorf("encoding the image: %w", err)
	}
	return buf.Bytes(), nil
}

/*
stretch scales one frame, keeping its palette and its place in the canvas.

A GIF's frames are often partial (a rectangle somewhere inside the canvas that
changes), so the frame's own bounds are scaled along with its pixels, and the
result sits where the original did.
*/
func stretch(frame *image.Paletted, canvasW, canvasH int, panel Size) *image.Paletted {
	scaleX := float64(panel.W) / float64(canvasW)
	scaleY := float64(panel.H) / float64(canvasH)

	bounds := frame.Bounds()
	out := image.Rect(
		int(float64(bounds.Min.X)*scaleX), int(float64(bounds.Min.Y)*scaleY),
		int(float64(bounds.Max.X)*scaleX), int(float64(bounds.Max.Y)*scaleY),
	)
	if out.Empty() {
		out = image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Min.X+1, bounds.Min.Y+1)
	}

	scaled := image.NewPaletted(out, frame.Palette)
	for y := out.Min.Y; y < out.Max.Y; y++ {
		source := bounds.Min.Y + int(float64(y-out.Min.Y)/scaleY)
		if source >= bounds.Max.Y {
			source = bounds.Max.Y - 1
		}
		for x := out.Min.X; x < out.Max.X; x++ {
			from := bounds.Min.X + int(float64(x-out.Min.X)/scaleX)
			if from >= bounds.Max.X {
				from = bounds.Max.X - 1
			}
			scaled.SetColorIndex(x, y, frame.ColorIndexAt(from, source))
		}
	}
	return scaled
}
