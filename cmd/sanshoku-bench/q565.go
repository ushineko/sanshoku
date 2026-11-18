package main

import (
	"context"
	"flag"
	"image"
	"image/color"
	"io"
	"slices"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/all"
	"github.com/ushineko/sanshoku/internal/q565"
	"github.com/ushineko/sanshoku/screen"
)

// encodedStreamer is the nzxt driver's format probe.
type encodedStreamer interface {
	StreamEncoded(ctx context.Context, format byte, payload []byte) error
}

// formatQ565 is the format byte NZXT CAM sends Q565 frames with.
const formatQ565 = 0x08

/*
q565Probe is a PROBE for format 08: a colour card, optionally a run of moving
frames at the device's pace, then nothing for --hold, and the readout. It
answers whether the panel takes Q565 from us, in which channel order, at what
rate, and whether the 30 s return applies. Not part of the bench's contract.
*/
func q565Probe(ctx context.Context, args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("q565", flag.ContinueOnError)
	fs.SetOutput(errOut)
	yes := fs.Bool("yes", false, "allow writing to the device's display")
	swap := fs.Bool("swap", false, "put blue in the high bits rather than red")
	animate := fs.Duration("animate", 0, "send moving frames for this long, as fast as the panel answers")
	hold := fs.Duration("hold", 10*time.Second, "after the last frame, send nothing for this long")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if !*yes {
		writeln(errOut, "q565 writes to a device's display; pass --yes to allow it")
		return 2
	}
	pack := q565.RGB565
	if *swap {
		pack = func(r, g, b uint8) uint16 { return q565.RGB565(b, g, r) }
	}

	found, scanErr := sanshoku.Scan(ctx, all.Drivers()...)
	if scanErr != nil {
		writef(errOut, "scan: %v\n", scanErr)
	}
	code, probed := 0, false
	for _, c := range found {
		dev, err := open(ctx, c)
		if err != nil {
			continue
		}
		s, isProbe := dev.(encodedStreamer)
		p, isPanel := dev.(screen.Panel)
		if isProbe && isPanel {
			probed = true
			if !q565One(ctx, out, c, p, s, pack, *animate, *hold) {
				code = 1
			}
		}
		if err := dev.Close(); err != nil {
			writef(out, "  close: %v\n", err)
			code = 1
		}
	}
	if !probed {
		writeln(out, "no panel that takes encoded frames found")
	}
	return code
}

func q565One(ctx context.Context, out io.Writer, c sanshoku.Candidate, p screen.Panel, s encodedStreamer,
	pack func(r, g, b uint8) uint16, animate, hold time.Duration,
) bool {
	w, h := p.Size()
	writef(out, "%s  %s  %s  panel %dx%d\n", stamp(), c.Driver, c.Identity, w, h)
	send := func(img image.Image) (enc, sent time.Duration, size int, ok bool) {
		start := time.Now()
		payload := q565.Encode(img, pack)
		enc = time.Since(start)
		fctx, cancel := context.WithTimeout(ctx, screenTimeout)
		defer cancel()
		start = time.Now()
		if err := s.StreamEncoded(fctx, formatQ565, payload); err != nil {
			writef(out, "%s  frame: error after %.1f ms: %v\n", stamp(), ms(time.Since(start)), err)
			return enc, 0, len(payload), false
		}
		return enc, time.Since(start), len(payload), true
	}

	card := bands(w, h)
	enc, sent, size, ok := send(card)
	if !ok {
		return false
	}
	writef(out, "%s  colour card: %d bytes, encoded in %.1f ms, sent in %.1f ms; top to bottom it should read red, green, blue\n",
		stamp(), size, ms(enc), ms(sent))

	if animate > 0 {
		var encs, sents []time.Duration
		var sizes []int
		began := time.Now()
		for i := 0; time.Since(began) < animate && ctx.Err() == nil; i++ {
			e, sd, n, ok := send(gradient(w, h, (i*8)%w))
			if !ok {
				return false
			}
			encs, sents, sizes = append(encs, e), append(sents, sd), append(sizes, n)
		}
		if n := len(sents); n > 0 {
			slices.Sort(encs)
			slices.Sort(sents)
			slices.Sort(sizes)
			writef(out, "%s  animated: %d frames in %s, %.1f fps; frames %d to %d bytes (median %d); "+
				"encode median %.1f ms, send median %.1f ms, max %.1f ms\n",
				stamp(), n, time.Since(began).Round(time.Millisecond), float64(n)/time.Since(began).Seconds(),
				sizes[0], sizes[n-1], sizes[n/2], ms(encs[n/2]), ms(sents[n/2]), ms(sents[n-1]))
		}
		if _, _, _, ok := send(card); !ok {
			return false
		}
		writef(out, "%s  colour card again\n", stamp())
	}

	writef(out, "%s  holding %s with nothing sent\n", stamp(), hold)
	select {
	case <-ctx.Done():
	case <-time.After(hold):
	}
	return step(context.WithoutCancel(ctx), out, "readout", p.Readout)
}

// bands is a card that names its channel order: red, green and blue bands,
// top to bottom, on black, so a swap shows as blue on top.
func bands(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	cols := []color.RGBA{{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255}}
	for y := range h {
		for x := range w {
			c := color.RGBA{A: 255}
			if band := (y - h/8) * 3 / (h * 3 / 4); y >= h/8 && band < 3 {
				c = cols[band]
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

// stamp is the time of day, for lining the output up with what a person saw.
func stamp() string { return time.Now().Format("15:04:05.000") }
