package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/all"
	"github.com/ushineko/sanshoku/aula"
	"github.com/ushineko/sanshoku/lighting"
)

/*
onboard reads the firmware effect of every device that runs one, and with
--effect, --colour or --restore and --yes writes it. Every write saves both
blocks first, to a file it names, so the write can be undone with --restore.
--effect and --colour together set the effect first and then its colour,
which is how a consumer puts both back after a power cycle. Spec 021 R3.
*/
func onboard(ctx context.Context, args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("onboard", flag.ContinueOnError)
	fs.SetOutput(errOut)
	yes := fs.Bool("yes", false, "allow writing to the device's storage")
	effect := fs.String("effect", "", "run this firmware effect, by name")
	colour := fs.String("colour", "", "make the running effect this colour, RRGGBB")
	save := fs.String("save", "", "save the device's blocks to this file")
	restore := fs.String("restore", "", "write the device's blocks back from this saved file")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	writes := *effect != "" || *colour != "" || *restore != ""
	if writes && !*yes {
		writeln(errOut, "onboard --effect, --colour and --restore write the device's flash; pass --yes to allow it")
		return 2
	}
	if *restore != "" && (*effect != "" || *colour != "") {
		writeln(errOut, "onboard: --restore goes alone")
		return 2
	}
	job := onboardJob{save: *save, restore: *restore, effect: *effect, colour: *colour != ""}
	if job.colour {
		c, err := parseColour(*colour)
		if err != nil {
			writef(errOut, "--colour: %v\n", err)
			return 2
		}
		job.want = lighting.Colour{R: c[0], G: c[1], B: c[2]}
	}

	found, scanErr := sanshoku.Scan(ctx, all.Drivers()...)
	if scanErr != nil {
		writef(errOut, "scan: %v\n", scanErr)
	}
	code, devices := 0, 0
	for _, c := range found {
		dev, err := open(ctx, c)
		if err != nil {
			continue
		}
		if ob, ok := dev.(lighting.Onboard); ok {
			devices++
			writef(out, "%s  %s  %s\n", c.Driver, c.Identity, c.Path)
			if !onboardOne(ctx, out, dev, ob, job) {
				code = 1
			}
		}
		if err := dev.Close(); err != nil {
			writef(out, "  close: %v\n", err)
			code = 1
		}
	}
	if devices == 0 {
		writeln(out, "no device with a firmware effect found")
	}
	return code
}

// onboardJob is what the onboard verb was asked to do to each device.
type onboardJob struct {
	save, restore, effect string
	colour                bool
	want                  lighting.Colour
}

// writes reports whether the job writes the device.
func (j onboardJob) writes() bool { return j.colour || j.effect != "" || j.restore != "" }

// onboardOne does the job on one device and prints the effect before and
// after any write.
func onboardOne(ctx context.Context, out io.Writer, dev sanshoku.Device, ob lighting.Onboard, job onboardJob) bool {
	if !printEffect(ctx, out, "", ob) {
		return false
	}
	if !job.writes() && job.save == "" {
		writef(out, "  effects: %s\n", strings.Join(ob.Effects(), ", "))
		return true
	}
	saver, canSave := dev.(aula.Saver)
	if !canSave {
		writeln(out, "  this device cannot be saved, so nothing is written")
		return false
	}
	if !saveTo(ctx, out, saver, job.save) {
		return false
	}
	switch {
	case job.restore != "":
		if !restoreFrom(ctx, out, saver, job.restore) {
			return false
		}
	case job.writes():
		if job.effect != "" && !timed(out, "set effect "+fmt.Sprintf("%q", job.effect), func() (bool, error) {
			return ob.SetEffect(ctx, job.effect)
		}) {
			return false
		}
		if job.colour && !timed(out, fmt.Sprintf("set colour #%02x%02x%02x", job.want.R, job.want.G, job.want.B), func() (bool, error) {
			return ob.SetColour(ctx, job.want)
		}) {
			return false
		}
	default:
		return true
	}
	return printEffect(ctx, out, "now ", ob)
}

// saveTo saves both blocks to path, or to a file named for the time in the
// temporary directory, and says where.
func saveTo(ctx context.Context, out io.Writer, saver aula.Saver, path string) bool {
	if path == "" {
		path = filepath.Join(os.TempDir(), time.Now().Format("sanshoku-onboard-20060102-150405.bin"))
	}
	start := time.Now()
	saved, err := saver.Save(ctx)
	if err == nil {
		err = os.WriteFile(path, saved, 0o600)
	}
	if err != nil {
		writef(out, "  save: %v\n", err)
		return false
	}
	writef(out, "  saved %d bytes to %s in %s\n", len(saved), path, time.Since(start).Round(time.Millisecond))
	return true
}

// restoreFrom writes a saved file back.
func restoreFrom(ctx context.Context, out io.Writer, saver aula.Saver, path string) bool {
	saved, err := os.ReadFile(path)
	if err != nil {
		writef(out, "  restore: %v\n", err)
		return false
	}
	start := time.Now()
	if err := saver.Restore(ctx, saved); err != nil {
		writef(out, "  restore: %v\n", err)
		return false
	}
	writef(out, "  restored from %s in %s\n", path, time.Since(start).Round(time.Millisecond))
	return true
}

// timed runs one write and prints whether it wrote and how long it took.
func timed(out io.Writer, what string, write func() (bool, error)) bool {
	start := time.Now()
	wrote, err := write()
	took := time.Since(start).Round(time.Millisecond)
	if err != nil {
		writef(out, "  %s: %v (wrote %t, %s)\n", what, err, wrote, took)
		return false
	}
	writef(out, "  %s: wrote %t, %s\n", what, wrote, took)
	return true
}

// printEffect prints the running effect and its colour.
func printEffect(ctx context.Context, out io.Writer, prefix string, ob lighting.Onboard) bool {
	e, err := ob.Effect(ctx)
	if err != nil {
		writef(out, "  %seffect: %v\n", prefix, err)
		return false
	}
	writef(out, "  %seffect %q, %s\n", prefix, e.Name, colourWords(e))
	return true
}

// colourWords is an effect's colour as the bench prints it.
func colourWords(e lighting.Effect) string {
	switch {
	case !e.Coloured:
		return "no colour"
	case e.Colour == nil:
		return "colourful"
	default:
		return fmt.Sprintf("colour #%02x%02x%02x", e.Colour.R, e.Colour.G, e.Colour.B)
	}
}
