package main

import (
	"bytes"
	"context"
	"flag"
	"image"
	"image/color"
	"image/gif"
	"io"
	"math"
	"slices"
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
	frame := fs.Bool("frame", false, "draw the card as streamed frames rather than a GIF, on a panel that takes them")
	animate := fs.Duration("animate", 0, "with --frame: send moving frames for this long first, as fast as the panel answers")
	noResend := fs.Bool("no-resend", false, "with --frame: never send the card again while holding, to see the firmware take the panel back")
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
			if *frame {
				if !frames(ctx, out, c, p, framing{hold: *hold, animate: *animate, resend: !*noResend}) {
					code = 1
				}
			} else if !push(ctx, out, c, p, *hold) {
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
	wait := max(p.ImageFloor(encoded.Len()), hold)
	writef(out, "%s  %s  panel %dx%d, test card %d bytes, floor %s\n", c.Driver, c.Identity, w, h, encoded.Len(), p.ImageFloor(encoded.Len()))

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

// framing is how screen --frame runs.
type framing struct {
	hold, animate time.Duration
	resend        bool
}

/*
frames draws the gradient card as streamed frames (spec 037): optionally a
run of moving frames first, with the rate and each frame's time, then the
still card for the hold, sent again every FrameHold unless resend is off. The
readout goes back at the end, as after an image.
*/
func frames(ctx context.Context, out io.Writer, c sanshoku.Candidate, p screen.Panel, f framing) bool {
	fr, ok := p.(screen.Framer)
	if !ok {
		writef(out, "%s  %s  does not take frames\n", c.Driver, c.Identity)
		return true
	}
	w, h := p.Size()
	writef(out, "%s  %s  panel %dx%d, frames held %s\n", c.Driver, c.Identity, w, h, fr.FrameHold())
	send := func(img image.Image) (time.Duration, bool) {
		ctx, cancel := context.WithTimeout(ctx, screenTimeout)
		defer cancel()
		start := time.Now()
		if err := fr.Frame(ctx, img); err != nil {
			writef(out, "  frame: error after %.1f ms: %v\n", ms(time.Since(start)), err)
			return 0, false
		}
		return time.Since(start), true
	}
	ok = holdFrames(ctx, out, f, w, h, fr.FrameHold(), send)
	return step(context.WithoutCancel(ctx), out, "readout", p.Readout) && ok
}

// holdFrames is frames' body, between the panel's introduction and the readout.
func holdFrames(ctx context.Context, out io.Writer, f framing, w, h int, every time.Duration,
	send func(image.Image) (time.Duration, bool),
) bool {
	if f.animate > 0 {
		var took []time.Duration
		began := time.Now()
		for i := 0; time.Since(began) < f.animate && ctx.Err() == nil; i++ {
			d, ok := send(gradient(w, h, (i*8)%w))
			if !ok {
				return false
			}
			took = append(took, d)
		}
		if n := len(took); n > 0 {
			slices.Sort(took)
			writef(out, "  animated: %d frames in %s, %.1f fps; a frame took %.1f ms at least, %.1f median, %.1f at most\n",
				n, time.Since(began).Round(time.Millisecond), float64(n)/time.Since(began).Seconds(),
				ms(took[0]), ms(took[n/2]), ms(took[n-1]))
		}
	}
	card := gradient(w, h, -1)
	d, ok := send(card)
	if !ok {
		return false
	}
	writef(out, "  frame: ok (%.1f ms); holding %s, resending %v\n", ms(d), f.hold, f.resend)
	began, last := time.Now(), time.Now()
	for time.Since(began) < f.hold {
		select {
		case <-ctx.Done():
			return true
		case <-time.After(time.Second):
		}
		if f.resend && time.Since(last) >= every {
			d, ok := send(card)
			if !ok {
				return false
			}
			last = time.Now()
			writef(out, "  +%s  sent again (%.1f ms)\n", time.Since(began).Round(time.Second), ms(d))
		}
	}
	return true
}

/*
gradient is a raw frame that is plainly neither the readout nor the GIF test
card: hue across, brightness down, which a 256-colour GIF cannot show without
banding. A white bar 8 pixels wide stands at x when animating; -1 draws none.
*/
func gradient(w, h, bar int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		v := 1 - float64(y)/float64(h)*0.85
		for x := range w {
			if bar >= 0 && x >= bar && x < bar+8 {
				img.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
				continue
			}
			img.SetRGBA(x, y, hsv(float64(x)/float64(w)*360, v))
		}
	}
	return img
}

// hsv is a fully saturated colour at a hue in degrees and a value in 0-1.
func hsv(hue, v float64) color.RGBA {
	x := v * (1 - math.Abs(math.Mod(hue/60, 2)-1))
	var r, g, b float64
	switch {
	case hue < 60:
		r, g = v, x
	case hue < 120:
		r, g = x, v
	case hue < 180:
		g, b = v, x
	case hue < 240:
		g, b = x, v
	case hue < 300:
		r, b = x, v
	default:
		r, b = v, x
	}
	return color.RGBA{uint8(r * 255), uint8(g * 255), uint8(b * 255), 255}
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
