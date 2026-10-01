package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/all"
	"github.com/ushineko/sanshoku/lighting"
)

// lightOptions are the light verb's flags, parsed.
type lightOptions struct {
	hold    time.Duration
	rate    time.Duration
	pattern string
	colour  [3]uint8
	only    []byte
}

// patterns are the bench's demonstration patterns, in the order `all` runs
// them. They prove that frames show; they are not an API (spec 010 R3.1).
var patterns = []string{"steady", "breathe", "wave"}

/*
light streams the demonstration patterns to every lighting canvas found and
releases each one, printing the acknowledgement statistics. It refuses
without --yes. With --keys it lists each canvas's keys and writes nothing.
Spec 010 R3.
*/
func light(ctx context.Context, args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("light", flag.ContinueOnError)
	fs.SetOutput(errOut)
	yes := fs.Bool("yes", false, "allow writing to the device's lights")
	keys := fs.Bool("keys", false, "list each canvas's keys and exit")
	hold := fs.Duration("hold", 5*time.Second, "how long each pattern runs")
	rate := fs.Duration("rate", 0, "the frame interval (default the canvas's floor)")
	pattern := fs.String("pattern", "all", "steady, breathe, wave or all")
	colour := fs.String("color", "ff0000", "the steady and breathe colour, RRGGBB")
	only := fs.String("only", "", "address only these key ids, comma-separated hex (e.g. 29,00)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	opts := lightOptions{hold: *hold, rate: *rate, pattern: *pattern}
	var err error
	if opts.colour, err = parseColour(*colour); err != nil {
		writef(errOut, "--color: %v\n", err)
		return 2
	}
	if opts.only, err = parseIDs(*only); err != nil {
		writef(errOut, "--only: %v\n", err)
		return 2
	}
	if opts.pattern != "all" && !slices.Contains(patterns, opts.pattern) {
		writef(errOut, "--pattern: %q is not one of steady, breathe, wave, all\n", opts.pattern)
		return 2
	}
	if !*keys && !*yes {
		writeln(errOut, "light writes to a device's lights; pass --yes to allow it")
		return 2
	}

	found, scanErr := sanshoku.Scan(ctx, all.Drivers()...)
	if scanErr != nil {
		writef(errOut, "scan: %v\n", scanErr)
	}
	code, canvases := 0, 0
	for _, c := range found {
		dev, err := open(ctx, c)
		if err != nil {
			continue
		}
		if cv, ok := dev.(lighting.Canvas); ok {
			canvases++
			writef(out, "%s  %s  %s  %d keys, floor %s\n", c.Driver, c.Identity, c.Path, len(cv.Keys()), cv.Floor())
			if *keys {
				for _, k := range cv.Keys() {
					writef(out, "  %#02x  %s\n", k.ID, k.Name)
				}
			} else if !stream(ctx, out, cv, opts) {
				code = 1
			}
		}
		if err := dev.Close(); err != nil {
			writef(out, "  close: %v\n", err)
			code = 1
		}
	}
	if canvases == 0 {
		writeln(out, "no lighting canvas found")
	}
	return code
}

// stream runs the chosen patterns on one canvas and releases it, whatever
// happened, on a context of its own so an interrupt still releases.
func stream(ctx context.Context, out io.Writer, cv lighting.Canvas, opts lightOptions) bool {
	ids := opts.only
	if len(ids) == 0 {
		for _, k := range cv.Keys() {
			ids = append(ids, k.ID)
		}
	}
	every := opts.rate
	if every <= 0 {
		every = cv.Floor()
	}
	run := patterns
	if opts.pattern != "all" {
		run = []string{opts.pattern}
	}

	ok := true
	for _, name := range run {
		st, err := runPattern(ctx, cv, name, ids, opts.colour, every, opts.hold)
		writef(out, "  %-8s %s\n", name, st)
		if err != nil {
			writef(out, "  %-8s stopped: %v\n", name, err)
			ok = false
			break
		}
		if ctx.Err() != nil {
			break
		}
	}

	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), screenTimeout)
	defer cancel()
	start := time.Now()
	if err := cv.Release(rctx); err != nil {
		writef(out, "  release: error after %.1f ms: %v\n", ms(time.Since(start)), err)
		return false
	}
	writef(out, "  release: ok (%.1f ms); watch the board for what the firmware shows\n", ms(time.Since(start)))
	return ok
}

// frameStats is one pattern's run: frames sent, acknowledged and timed out,
// and the latency of the acknowledged ones.
type frameStats struct {
	sent, acked, silent int
	interval            time.Duration
	latency             []time.Duration
}

// String is the line R3.2 asks for.
func (s frameStats) String() string {
	if len(s.latency) == 0 {
		return fmt.Sprintf("every %s: %d frames, %d acknowledged, %d timed out", s.interval, s.sent, s.acked, s.silent)
	}
	l := slices.Clone(s.latency)
	slices.Sort(l)
	at := func(q float64) time.Duration { return l[min(len(l)-1, int(q*float64(len(l))))] }
	return fmt.Sprintf("every %s: %d frames, %d acknowledged, %d timed out; ack p50 %.1f ms, p95 %.1f ms, max %.1f ms",
		s.interval, s.sent, s.acked, s.silent, ms(at(0.50)), ms(at(0.95)), ms(l[len(l)-1]))
}

/*
runPattern streams one pattern for hold at one frame per interval.

A frame that is not acknowledged is counted and the stream goes on, because
how many time out at a given rate is what E2 measures. A device that has gone
stops the run.
*/
func runPattern(ctx context.Context, cv lighting.Canvas, name string, ids []byte, colour [3]uint8,
	interval, hold time.Duration,
) (frameStats, error) {
	st := frameStats{interval: interval}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	start := time.Now()
	for time.Since(start) < hold {
		px := render(name, ids, colour, time.Since(start))
		sent := time.Now()
		err := cv.Frame(ctx, px)
		st.sent++
		switch {
		case err == nil:
			st.acked++
			st.latency = append(st.latency, time.Since(sent))
		case errors.Is(err, sanshoku.ErrGone), ctx.Err() != nil:
			return st, err
		default:
			st.silent++
		}
		select {
		case <-ctx.Done():
			return st, nil
		case <-tick.C:
		}
	}
	return st, nil
}

// breathePeriod and wavePeriod are how long one breath and one pass of the
// wave take.
const (
	breathePeriod = 3 * time.Second
	wavePeriod    = 2 * time.Second
)

// render is one frame of a pattern at time t: every id the same colour
// (steady), that colour scaled by a sine (breathe), or a hue that moves along
// the ids (wave).
func render(name string, ids []byte, c [3]uint8, t time.Duration) []lighting.Pixel {
	px := make([]lighting.Pixel, len(ids))
	phase := func(period time.Duration) float64 { return float64(t%period) / float64(period) }
	for i, id := range ids {
		r, g, b := c[0], c[1], c[2]
		switch name {
		case "breathe":
			k := 0.5 - 0.5*math.Cos(2*math.Pi*phase(breathePeriod))
			r, g, b = scale(r, k), scale(g, k), scale(b, k)
		case "wave":
			r, g, b = hue(float64(i)/float64(len(ids)) + phase(wavePeriod))
		}
		px[i] = lighting.Pixel{ID: id, R: r, G: g, B: b}
	}
	return px
}

// scale multiplies a channel by k in [0, 1].
func scale(v uint8, k float64) uint8 { return uint8(math.Round(float64(v) * k)) }

// hue is the fully saturated colour at h turns round the colour wheel.
func hue(h float64) (r, g, b uint8) {
	h = (h - math.Floor(h)) * 6
	x := uint8(math.Round(255 * (1 - math.Abs(math.Mod(h, 2)-1))))
	switch int(h) {
	case 0:
		return 255, x, 0
	case 1:
		return x, 255, 0
	case 2:
		return 0, 255, x
	case 3:
		return 0, x, 255
	case 4:
		return x, 0, 255
	default:
		return 255, 0, x
	}
}

// parseColour reads RRGGBB.
func parseColour(s string) ([3]uint8, error) {
	v, err := strconv.ParseUint(strings.TrimPrefix(s, "#"), 16, 24)
	if err != nil || len(strings.TrimPrefix(s, "#")) != 6 {
		return [3]uint8{}, fmt.Errorf("%q is not RRGGBB", s)
	}
	return [3]uint8{uint8(v >> 16), uint8(v >> 8 & 0xff), uint8(v & 0xff)}, nil //nolint:gosec // 24 bits, parsed as such
}

// parseIDs reads a comma-separated list of hex key ids.
func parseIDs(s string) ([]byte, error) {
	if s == "" {
		return nil, nil
	}
	var ids []byte
	for f := range strings.SplitSeq(s, ",") {
		v, err := strconv.ParseUint(strings.TrimPrefix(strings.TrimSpace(f), "0x"), 16, 8)
		if err != nil {
			return nil, fmt.Errorf("%q is not a hex key id", f)
		}
		ids = append(ids, byte(v))
	}
	return ids, nil
}
