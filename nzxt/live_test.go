package nzxt

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/cooling"
	"github.com/ushineko/sanshoku/screen"
)

/*
The tests that touch the real cooler, and skip where there is none.

Everything this driver knows about the cooler was learned from the cooler, so
a model of it proves nothing about the hardware. These are hotaru's live
tests, ported, so `go test` on the desk catches a regression without running
the bench by hand.

**Reading is free; writing is not.** A test that puts a picture on somebody's
screen changes their machine, so the screen test is opt-in through
SANSHOKU_LIVE_SCREEN=1 and puts the firmware readout back when it finishes.
*/
func live(t *testing.T) sanshoku.Device {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	found, err := Driver{}.Find(ctx)
	if errors.Is(err, sanshoku.ErrAbsent) {
		t.Skip("no NZXT node on this machine")
	}
	require.NoError(t, err)
	var last error
	for _, c := range found {
		dev, err := c.Open(ctx)
		if err == nil {
			t.Cleanup(func() { _ = dev.Close() })
			return dev
		}
		last = err
	}
	if errors.Is(last, sanshoku.ErrUnsupported) || errors.Is(last, sanshoku.ErrAbsent) {
		t.Skipf("no Kraken this driver speaks to answered: %v", last)
	}
	require.NoError(t, last, "see docs/udev.md")
	return nil
}

func TestLiveKrakenReportsNumbersThatMakeSense(t *testing.T) {
	s, err := live(t).(cooling.Source).Status(t.Context())
	require.NoError(t, err)
	t.Logf("coolant %.1f C, pump %d rpm (%d%%), fan %d rpm (%d%%)", s.Coolant, s.PumpRPM, s.PumpDuty, s.FanRPM, s.FanDuty)

	// Ranges rather than values: this is a running machine, not a fixture.
	require.Greater(t, s.Coolant, 10.0, "a coolant temperature below the room is a parse error")
	require.Less(t, s.Coolant, 90.0, "a coolant temperature this high is a fault or a parse error")
	require.Positive(t, s.PumpRPM, "a stopped pump, or the wrong bytes")
	require.LessOrEqual(t, s.PumpDuty, 100)
	require.LessOrEqual(t, s.FanDuty, 100)
	require.False(t, s.Taken.IsZero())
}

/*
The check that can catch a byte offset read off the wrong field: two
independent implementations of the protocol, asked in the same minute.

Compared with hotaru's tolerances, because these are live numbers: a pump
drifts by a few rpm between two readings a fraction of a second apart, and an
offset taken off the wrong field is wrong by hundreds, not by five.
*/
func TestLiveKrakenAgreesWithLiquidctl(t *testing.T) {
	dev := live(t)
	path, err := exec.LookPath("liquidctl")
	if err != nil {
		t.Skip("liquidctl is not installed, so there is nothing to compare against")
	}

	mine, err := dev.(cooling.Source).Status(t.Context())
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--match", "kraken", "status").Output()
	require.NoError(t, err)
	theirs := string(out)

	require.InDelta(t, mine.Coolant, number(t, theirs, "Liquid temperature"), 1.0, "coolant disagrees with liquidctl")
	require.InEpsilon(t, float64(mine.PumpRPM), number(t, theirs, "Pump speed"), 0.1, "pump speed disagrees with liquidctl")
	require.InEpsilon(t, float64(mine.FanRPM), number(t, theirs, "Fan speed"), 0.15, "fan speed disagrees with liquidctl")
}

// number pulls one labelled value out of liquidctl's report.
func number(t *testing.T, report, label string) float64 {
	t.Helper()
	for line := range strings.SplitSeq(report, "\n") {
		if !strings.Contains(line, label) {
			continue
		}
		for _, field := range strings.Fields(line) {
			if v, err := strconv.ParseFloat(field, 64); err == nil {
				return v
			}
		}
	}
	t.Fatalf("liquidctl did not report %q:\n%s", label, report)
	return 0
}

/*
A reading after the handle sits idle.

The cooler broadcasts about once a second, the kernel queues those per open
handle, and a reader that does not drain them finds a dozen stale reports and
concludes the device is silent. It shows up only after idleness, so a loop of
calls never sees it. Six seconds queues more than one search looks at, and is
longer than Freshness, so the second call reaches the device.
*/
func TestLiveKrakenSurvivesBeingLeftAlone(t *testing.T) {
	if testing.Short() {
		t.Skip("this one waits, deliberately")
	}
	src := live(t).(cooling.Source)
	_, err := src.Status(t.Context())
	require.NoError(t, err)

	time.Sleep(6 * time.Second)

	_, err = src.Status(t.Context())
	require.NoError(t, err, "a reading failed after the handle sat idle; the queue was not drained")
}

/*
The panel takes an image and gives the screen back, and the control channel
still answers while the panel is claimed, which is the coexistence the design
rests on. Opt-in; the readout is restored whatever happens.
*/
func TestLiveScreenTakesAnImageAndGivesItBack(t *testing.T) {
	if os.Getenv("SANSHOKU_LIVE_SCREEN") != "1" {
		t.Skip("set SANSHOKU_LIVE_SCREEN=1 to let this draw on the cooler")
	}
	dev := live(t)
	p, ok := dev.(screen.Panel)
	if !ok {
		t.Skip("this cooler has no panel this driver can reach")
	}
	defer func() { _ = p.Readout(context.Background()) }()

	require.NoError(t, p.Image(t.Context(), card(640, 640)), "the image did not reach the panel")
	_, err := dev.(cooling.Source).Status(t.Context())
	require.NoError(t, err, "claiming the panel broke the control channel")
	require.NoError(t, p.Readout(t.Context()), "the screen could not be given back")
}
