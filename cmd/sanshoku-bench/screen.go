package main

import (
	"bytes"
	"context"
	"flag"
	"image"
	"image/color"
	"image/gif"
	"io"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/all"
	"github.com/ushineko/sanshoku/screen"
)

// screenTimeout bounds one panel call. A transfer takes about a second on the
// Kraken; the test card is small.
const screenTimeout = 15 * time.Second

/*
screenPush is the bench's one write: push a generated test card to every
screen found, wait the panel's floor for that frame so it lands, and return
each panel to its readout. It refuses without --yes. Spec 001 R7.4, spec 005
R5.5.
*/
func screenPush(ctx context.Context, args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("screen", flag.ContinueOnError)
	fs.SetOutput(errOut)
	yes := fs.Bool("yes", false, "allow writing to the device's display")
	hold := fs.Duration("hold", 0, "keep the test card on the panel this long (at least its floor)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if !*yes {
		writeln(errOut, "screen writes to a device's display; pass --yes to allow it")
		return 2
	}

	found, scanErr := sanshoku.Scan(ctx, all.Drivers()...)
	if scanErr != nil {
		writef(errOut, "scan: %v\n", scanErr)
	}
	code, screens := 0, 0
	for _, c := range found {
		dev, err := open(ctx, c)
		if err != nil {
			continue
		}
		if p, ok := dev.(screen.Panel); ok {
			screens++
			if !push(ctx, out, c, p, *hold) {
				code = 1
			}
		}
		if err := dev.Close(); err != nil {
			writef(out, "  close: %v\n", err)
			code = 1
		}
	}
	if screens == 0 {
		writeln(out, "no screen found")
	}
	return code
}

// push draws the test card on one panel, waits its floor, and returns it to
// its readout, printing each step with its timing.
func push(ctx context.Context, out io.Writer, c sanshoku.Candidate, p screen.Panel, hold time.Duration) bool {
	w, h := p.Size()
	card := testCard(w, h)
	var encoded bytes.Buffer
	if err := gif.EncodeAll(&encoded, card); err != nil {
		writef(out, "%s: encoding the test card: %v\n", c.Identity, err)
		return false
	}
	// The card stays up for at least the floor, so it lands, and for longer
	// when someone wants to look at it.
	wait := max(p.Floor(encoded.Len()), hold)
	writef(out, "%s  %s  panel %dx%d, test card %d bytes, floor %s\n", c.Driver, c.Identity, w, h, encoded.Len(), p.Floor(encoded.Len()))

	ok := step(ctx, out, "image", func(ctx context.Context) error { return p.Image(ctx, card) })
	if ok {
		select {
		case <-ctx.Done():
		case <-time.After(wait):
			writef(out, "  waited %s\n", wait)
		}
	}
	// The readout goes back whatever happened above, on a context of its own
	// so that an interrupt does not leave the card on the panel.
	return step(context.WithoutCancel(ctx), out, "readout", p.Readout) && ok
}

// step runs one panel call under screenTimeout and prints its outcome.
func step(ctx context.Context, out io.Writer, what string, call func(context.Context) error) bool {
	ctx, cancel := context.WithTimeout(ctx, screenTimeout)
	defer cancel()
	start := time.Now()
	if err := call(ctx); err != nil {
		writef(out, "  %s: error after %.1f ms: %v\n", what, ms(time.Since(start)), err)
		return false
	}
	writef(out, "  %s: ok (%.1f ms)\n", what, ms(time.Since(start)))
	return true
}

/*
testCard is a one-frame GIF that is plainly not a coolant readout, so somebody
watching can tell the bench from the firmware: a checkerboard of two colours
between two dark bands. hotaru's live-test card, at the panel's size.
*/
func testCard(w, h int) *gif.GIF {
	pal := color.Palette{
		color.RGBA{10, 10, 20, 255},
		color.RGBA{240, 200, 60, 255},
		color.RGBA{60, 180, 220, 255},
	}
	img := image.NewPaletted(image.Rect(0, 0, w, h), pal)
	for y := range h {
		for x := range w {
			i := uint8(1)
			if (x/80+y/80)%2 == 0 {
				i = 2
			}
			if y < h*60/640 || y > h*580/640 {
				i = 0
			}
			img.SetColorIndex(x, y, i)
		}
	}
	return &gif.GIF{
		Image: []*image.Paletted{img}, Delay: []int{10},
		Config: image.Config{ColorModel: pal, Width: w, Height: h},
	}
}
