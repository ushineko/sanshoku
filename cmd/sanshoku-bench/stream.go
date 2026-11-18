package main

import (
	"context"
	"flag"
	"io"
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/all"
	"github.com/ushineko/sanshoku/cooling"
	"github.com/ushineko/sanshoku/screen"
)

// streamer is the nzxt driver's streaming probe (issue #27).
type streamer interface {
	StreamOpen(ctx context.Context) error
	StreamFrame(ctx context.Context, format byte, payload []byte) (time.Duration, error)
}

/*
streamPush is the streaming probe: one raw frame, optionally a run of moving
frames, then nothing for --hold while the cooler's readings are taken, so
somebody watching can say how long the frame stays. The panel goes back to its
readout at the end. Issue #27.
*/
func streamPush(ctx context.Context, args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("stream", flag.ContinueOnError)
	fs.SetOutput(errOut)
	yes := fs.Bool("yes", false, "allow writing to the device's display")
	openFirst := fs.Bool("open", false, "send 36 04 and 36 03, hand back to the readout and empty every bucket first, as CAM does")
	animate := fs.Duration("animate", 0, "send moving frames for this long, as fast as the device answers, before holding")
	gap := fs.Duration("gap", 0, "after animating, wait this long before the still frame")
	hold := fs.Duration("hold", 2*time.Minute, "after the last frame, send nothing for this long")
	repeat := fs.Duration("repeat", 0, "while holding, send the still frame again this often (0: never)")
	poll := fs.Duration("poll", 5*time.Second, "take a reading this often while holding")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if !*yes {
		writeln(errOut, "stream writes to a device's display; pass --yes to allow it")
		return 2
	}
	found, scanErr := sanshoku.Scan(ctx, all.Drivers()...)
	if scanErr != nil {
		writef(errOut, "scan: %v\n", scanErr)
	}
	code, streamers := 0, 0
	for _, c := range found {
		dev, err := open(ctx, c)
		if err != nil {
			continue
		}
		s, isStreamer := dev.(streamer)
		if isStreamer {
			streamers++
			p, _ := dev.(screen.Panel)
			src, _ := dev.(cooling.Source)
			if !streamOne(ctx, out, c, p, s, src, *openFirst, *animate, *gap, *hold, *poll, *repeat) {
				code = 1
			}
		}
		start := time.Now()
		if err := dev.Close(); err != nil {
			writef(out, "  close: %v\n", err)
			code = 1
		} else if isStreamer {
			writef(out, "%s  closed, screen handed back to the readout (%.1f ms)\n", stamp(), ms(time.Since(start)))
		}
	}
	if streamers == 0 {
		writeln(out, "no screen that streams found")
	}
	return code
}

func streamOne(ctx context.Context, out io.Writer, c sanshoku.Candidate, p screen.Panel, s streamer,
	src cooling.Source, openFirst bool, animate, gap, hold, poll, repeat time.Duration,
) bool {
	w, h := p.Size()
	writef(out, "%s  %s  %s  panel %dx%d\n", stamp(), c.Driver, c.Identity, w, h)
	if openFirst {
		if !step(ctx, out, "open (36 04, 36 03, readout, empty buckets)", s.StreamOpen) {
			return false
		}
	}

	send := func(frame []byte) (time.Duration, bool) {
		fctx, cancel := context.WithTimeout(ctx, screenTimeout)
		defer cancel()
		start := time.Now()
		ack, err := s.StreamFrame(fctx, 0x09, frame)
		if err != nil {
			writef(out, "%s  frame: error after %.1f ms: %v\n", stamp(), ms(time.Since(start)), err)
			return 0, false
		}
		return ack, true
	}

	start := time.Now()
	ack, ok := send(gradient(w, h, -1))
	if !ok {
		return false
	}
	writef(out, "%s  first frame: ok, %.1f ms in all, 37 02 after %.1f ms\n", stamp(), ms(time.Since(start)), ms(ack))

	if animate > 0 {
		var acks []time.Duration
		began, until := time.Now(), time.Now().Add(animate)
		for i := 0; time.Now().Before(until) && ctx.Err() == nil; i++ {
			a, ok := send(gradient(w, h, (i*8)%w))
			if !ok {
				return false
			}
			acks = append(acks, a)
		}
		slices.Sort(acks)
		n := len(acks)
		if n > 0 {
			writef(out, "%s  animated: %d frames in %s, %.1f fps; 37 02 min %.1f median %.1f max %.1f ms\n",
				stamp(), n, time.Since(began).Round(time.Millisecond), float64(n)/time.Since(began).Seconds(),
				ms(acks[0]), ms(acks[n/2]), ms(acks[n-1]))
		}
		// The last frame shown is a still one, so a hold watches the same
		// picture whether or not it animated.
		if gap > 0 {
			writef(out, "%s  waiting %s before the still frame\n", stamp(), gap)
			select {
			case <-ctx.Done():
				return true
			case <-time.After(gap):
			}
		}
		if _, ok := send(gradient(w, h, -1)); !ok {
			return false
		}
		writef(out, "%s  still frame sent\n", stamp())
	}

	writef(out, "%s  holding %s with no frames; watch the panel and the ring\n", stamp(), hold)
	began := time.Now()
	resent := began
	tick := time.NewTicker(poll)
	defer tick.Stop()
	for time.Since(began) < hold {
		select {
		case <-ctx.Done():
			writef(out, "%s  interrupted after %s\n", stamp(), time.Since(began).Round(time.Second))
			return true
		case <-tick.C:
		}
		if repeat > 0 && time.Since(resent) >= repeat {
			ack, ok := send(gradient(w, h, -1))
			if !ok {
				return false
			}
			resent = time.Now()
			writef(out, "%s  +%s  resent the still frame (37 02 after %.1f ms)\n", stamp(), time.Since(began).Round(time.Second), ms(ack))
		}
		line := "no reading"
		if src != nil {
			rctx, cancel := context.WithTimeout(ctx, screenTimeout)
			st, err := src.Status(rctx)
			cancel()
			if err != nil {
				line = "reading failed: " + err.Error()
			} else {
				line = "coolant " + formatC(st.Coolant)
			}
		}
		writef(out, "%s  +%s  %s\n", stamp(), time.Since(began).Round(time.Second), line)
	}
	return true
}

func stamp() string { return time.Now().Format("15:04:05.000") }

func formatC(c float64) string { return strconv.FormatFloat(c, 'f', 1, 64) + " C" }

/*
gradient is a raw RGB888 frame that is plainly neither the readout nor the GIF
test card: hue across, brightness down, which a 256-colour GIF cannot show
without banding. A bar at x (white, 8 pixels wide) moves when animating; -1
draws none.
*/
func gradient(w, h, bar int) []byte {
	px := make([]byte, 0, w*h*3)
	for y := range h {
		v := 1 - float64(y)/float64(h)*0.85
		for x := range w {
			if bar >= 0 && x >= bar && x < bar+8 {
				px = append(px, 255, 255, 255)
				continue
			}
			r, g, b := hsv(float64(x)/float64(w)*360, 1, v)
			px = append(px, r, g, b)
		}
	}
	return px
}

func hsv(hue, s, v float64) (r, g, b byte) {
	c := v * s
	x := c * (1 - math.Abs(math.Mod(hue/60, 2)-1))
	var rf, gf, bf float64
	switch {
	case hue < 60:
		rf, gf = c, x
	case hue < 120:
		rf, gf = x, c
	case hue < 180:
		gf, bf = c, x
	case hue < 240:
		gf, bf = x, c
	case hue < 300:
		rf, bf = x, c
	default:
		rf, bf = c, x
	}
	m := v - c
	return byte((rf + m) * 255), byte((gf + m) * 255), byte((bf + m) * 255)
}
