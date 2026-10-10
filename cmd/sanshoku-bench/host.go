package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ushineko/sanshoku/gpu"
	"github.com/ushineko/sanshoku/lhm"
)

/*
host reads what Windows gives a program about the machine itself: the graphics
card through D3DKMT and the GPU Engine counters, and the processor's
temperature through LibreHardwareMonitor's web server (spec 018). Read-only:
the one request made of LibreHardwareMonitor is a GET of its sensor tree.

Neither source is a device a driver finds, so they are not in scan or read. A
source that is not there is said and is not a failure: a machine without a
card, or without LibreHardwareMonitor, is an ordinary machine. The exit status
is 1 only when a source answered with something this could not read.
*/
func host(ctx context.Context, args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("host", flag.ContinueOnError)
	fs.SetOutput(errOut)
	url := fs.String("lhm", lhm.URL, "where LibreHardwareMonitor serves its sensor tree")
	repeat := fs.Int("repeat", 1, "how many readings to take")
	interval := fs.Duration("interval", 2*time.Second, "the pause between readings")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	var reader gpu.Reader
	defer func() { _ = reader.Close() }()
	client := &http.Client{Timeout: lhm.Timeout}

	code := 0
	for i := range max(*repeat, 1) {
		if i > 0 {
			select {
			case <-ctx.Done():
				return code
			case <-time.After(*interval):
			}
		}
		if !hostCard(ctx, &reader, out) || !hostProcessor(ctx, client, *url, out) {
			code = 1
		}
	}
	return code
}

// hostCard prints one reading of the card, and whether it was a reading or an
// ordinary absence.
func hostCard(ctx context.Context, reader *gpu.Reader, out io.Writer) bool {
	start := time.Now()
	card, err := reader.Read(ctx)
	took := time.Since(start)
	switch {
	case errors.Is(err, gpu.ErrNoCard), errors.Is(err, errors.ErrUnsupported):
		writef(out, "gpu  none: %v\n", err)
		return true
	case err != nil:
		writef(out, "gpu  failed: %v\n", err)
		return false
	}
	writef(out, "gpu  %s  %s  %s  (%.1f ms)\n", orDash(card.Name),
		figure(card.Temperature, card.HasTemperature, "%.1f °C"), figure(card.Load, card.HasLoad, "%.0f %%"),
		float64(took.Microseconds())/1000)
	return true
}

// hostProcessor prints the processor's temperature, or why there is none with
// what the machine has of LibreHardwareMonitor.
func hostProcessor(ctx context.Context, client *http.Client, url string, out io.Writer) bool {
	start := time.Now()
	degrees, label, err := lhm.Read(ctx, client, url)
	took := time.Since(start)
	if err == nil {
		writef(out, "cpu  %.1f °C  %s, LibreHardwareMonitor  (%.1f ms)\n", degrees, label, float64(took.Microseconds())/1000)
		return true
	}
	have := lhm.ProbeHost()
	writef(out, "cpu  none: %v\n     PawnIO installed: %v, LibreHardwareMonitor running: %v, startup task: %v\n",
		err, have.PawnIO, have.Running, have.Task)
	return errors.Is(err, lhm.ErrUnreachable) || errors.Is(err, lhm.ErrNoSensor) || errors.Is(err, lhm.ErrAuth)
}

// figure is a reading, or a dash where the source gave none.
func figure(v float64, ok bool, format string) string {
	if !ok {
		return "–"
	}
	return fmt.Sprintf(format, v)
}
