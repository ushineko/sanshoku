package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/all"
	"github.com/ushineko/sanshoku/battery"
	"github.com/ushineko/sanshoku/cooling"
	"github.com/ushineko/sanshoku/hwmon"
	"github.com/ushineko/sanshoku/support"
)

// readTimeout bounds one capability call.
const readTimeout = 10 * time.Second

// deviceReport is one device's line of `read --json`.
type deviceReport struct {
	Driver       string       `json:"driver"`
	Name         string       `json:"name"`
	Vendor       string       `json:"vendor"`
	Product      string       `json:"product"`
	Bus          string       `json:"bus"`
	Path         string       `json:"path"`
	Tier         string       `json:"tier"`
	Capabilities []string     `json:"capabilities"`
	Batteries    *batteryRead `json:"batteries,omitempty"`
	Cooling      *coolingRead `json:"cooling,omitempty"`
	Errors       []string     `json:"errors,omitempty"`
	expected     bool
	candidate    sanshoku.Candidate
}

type batteryRead struct {
	ElapsedMS float64       `json:"elapsed_ms"`
	Readings  []batteryJSON `json:"readings"`
	Error     string        `json:"error,omitempty"`
}

type batteryJSON struct {
	Name     string     `json:"name"`
	Level    *int       `json:"level,omitempty"`
	Band     string     `json:"band,omitempty"`
	State    string     `json:"state"`
	Kind     string     `json:"kind"`
	Cells    []cellJSON `json:"cells,omitempty"`
	hasLevel bool
	rawLevel int
}

type cellJSON struct {
	Cell     string `json:"cell"`
	Level    int    `json:"level"`
	Charging bool   `json:"charging"`
}

type coolingRead struct {
	ElapsedMS float64 `json:"elapsed_ms"`
	Coolant   float64 `json:"coolant_c"`
	PumpRPM   *int    `json:"pump_rpm,omitempty"`
	PumpDuty  *int    `json:"pump_duty,omitempty"`
	FanRPM    *int    `json:"fan_rpm,omitempty"`
	FanDuty   *int    `json:"fan_duty,omitempty"`
	Error     string  `json:"error,omitempty"`
	status    cooling.Status
}

// sensorReport is one of the hwmon sensor tables read through hwmon.First,
// which is how both consumers read a temperature.
type sensorReport struct {
	Sensor    string   `json:"sensor"`
	Chip      string   `json:"chip,omitempty"`
	Label     string   `json:"label,omitempty"`
	Celsius   *float64 `json:"celsius,omitempty"`
	ElapsedMS float64  `json:"elapsed_ms"`
	Error     string   `json:"error,omitempty"`
}

// readAll opens every candidate the drivers find and takes every read-only
// reading. The error is the scan's; per-device errors are in the reports.
func readAll(ctx context.Context, ds []sanshoku.Driver) ([]deviceReport, error) {
	found, scanErr := sanshoku.Scan(ctx, ds...)
	entries := all.Support()
	reports := make([]deviceReport, 0, len(found))
	for _, c := range found {
		entry, known := entryFor(entries, c)
		r := deviceReport{
			Driver:       c.Driver,
			Name:         c.Name,
			Vendor:       fmt.Sprintf("%04x", c.Vendor),
			Product:      fmt.Sprintf("%04x", c.Product),
			Bus:          c.Bus.String(),
			Path:         c.Path,
			Tier:         tierWords(entry, known),
			expected:     known && entry.Tier == support.Expected,
			Capabilities: []string{},
			candidate:    c,
		}
		dev, err := open(ctx, c)
		if err != nil {
			msg := err.Error()
			if sanshoku.IsPermission(err) {
				msg += "; " + permissionHint(c.Identity)
			}
			r.Errors = append(r.Errors, msg)
			reports = append(reports, r)
			continue
		}
		r.Capabilities = append([]string{}, sanshoku.Capabilities(dev)...)
		if src, ok := dev.(battery.Source); ok {
			r.Batteries = readBatteries(ctx, src)
		}
		if src, ok := dev.(cooling.Source); ok {
			r.Cooling = readCooling(ctx, src)
		}
		if err := dev.Close(); err != nil {
			r.Errors = append(r.Errors, err.Error())
		}
		reports = append(reports, r)
	}
	return reports, scanErr
}

func readBatteries(ctx context.Context, src battery.Source) *batteryRead {
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	start := time.Now()
	got, err := src.Batteries(ctx)
	out := &batteryRead{ElapsedMS: ms(time.Since(start))}
	if err != nil {
		out.Error = err.Error()
	}
	for _, b := range got {
		j := batteryJSON{Name: b.Name, State: b.State.String(), Kind: b.Kind.String(), hasLevel: b.HasLevel, rawLevel: b.Level}
		if b.HasLevel {
			level := b.Level
			j.Level = &level
		}
		if b.HasBand {
			j.Band = b.Band.String()
		}
		for _, c := range b.Cells {
			j.Cells = append(j.Cells, cellJSON{Cell: c.Cell.String(), Level: c.Level, Charging: c.Charging})
		}
		out.Readings = append(out.Readings, j)
	}
	return out
}

func readCooling(ctx context.Context, src cooling.Source) *coolingRead {
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	start := time.Now()
	s, err := src.Status(ctx)
	out := &coolingRead{ElapsedMS: ms(time.Since(start)), status: s}
	if err != nil {
		out.Error = err.Error()
		return out
	}
	out.Coolant = s.Coolant
	if s.HasPump {
		out.PumpRPM, out.PumpDuty = &s.PumpRPM, &s.PumpDuty
	}
	if s.HasFan {
		out.FanRPM, out.FanDuty = &s.FanRPM, &s.FanDuty
	}
	return out
}

// readSensors reads the CPU and GPU sensor tables.
func readSensors() []sensorReport {
	tables := []struct {
		name    string
		sensors []hwmon.Sensor
	}{{"cpu", hwmon.CPU}, {"gpu", hwmon.GPU}}
	out := make([]sensorReport, 0, len(tables))
	for _, t := range tables {
		start := time.Now()
		s, v, err := hwmon.First(hwmon.Root, t.sensors)
		r := sensorReport{Sensor: t.name, ElapsedMS: ms(time.Since(start))}
		if err != nil {
			r.Error = err.Error()
		} else {
			r.Chip, r.Label, r.Celsius = s.Chip, s.Label, &v
		}
		out = append(out, r)
	}
	return out
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// read opens every device and prints every read-only reading, once or on an
// interval. Exit 1 when a driver failed or a reading returned an error.
func read(ctx context.Context, args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("read", flag.ContinueOnError)
	fs.SetOutput(errOut)
	asJSON := fs.Bool("json", false, "print one JSON object per device")
	driver := fs.String("driver", "", "restrict to one driver")
	repeat := fs.Int("repeat", 1, "read this many times")
	interval := fs.Duration("interval", 2*time.Second, "wait between repeats")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	ds, err := drivers(*driver)
	if err != nil {
		writeln(errOut, err)
		return 2
	}

	code := 0
	for i := range max(*repeat, 1) {
		if i > 0 {
			select {
			case <-ctx.Done():
				return code
			case <-time.After(*interval):
			}
			if !*asJSON {
				writeln(out)
			}
		}
		reports, scanErr := readAll(ctx, ds)
		var sensors []sensorReport
		if *driver == "" || *driver == "hwmon" {
			sensors = readSensors()
		}
		if *asJSON {
			printJSON(out, reports, sensors)
		} else {
			printText(out, reports, sensors)
		}
		if scanErr != nil {
			writef(errOut, "scan: %v\n", scanErr)
			code = 1
		}
		for _, r := range reports {
			if len(r.Errors) > 0 || (r.Batteries != nil && r.Batteries.Error != "") || (r.Cooling != nil && r.Cooling.Error != "") {
				code = 1
			}
		}
	}
	return code
}

func printJSON(out io.Writer, reports []deviceReport, sensors []sensorReport) {
	enc := json.NewEncoder(out)
	for _, r := range reports {
		_ = enc.Encode(r)
	}
	for _, s := range sensors {
		_ = enc.Encode(s)
	}
}

func printText(out io.Writer, reports []deviceReport, sensors []sensorReport) {
	idle := 0
	for _, r := range reports {
		if r.Batteries == nil && r.Cooling == nil && len(r.Errors) == 0 {
			idle++
			continue
		}
		writef(out, "%s  %s  %s  [%s]\n", r.Driver, r.candidate.Identity, r.Path, r.Tier)
		for _, e := range r.Errors {
			writef(out, "  error: %s\n", e)
		}
		if b := r.Batteries; b != nil {
			if b.Error != "" {
				writef(out, "  battery: error after %.1f ms: %s\n", b.ElapsedMS, b.Error)
			}
			if len(b.Readings) == 0 && b.Error == "" {
				writef(out, "  battery: no reading (%.1f ms)\n", b.ElapsedMS)
			}
			for _, j := range b.Readings {
				writef(out, "  battery: %s  %s  %s  %s (%.1f ms)\n", j.Name, level(j), j.State, j.Kind, b.ElapsedMS)
				for _, c := range j.Cells {
					charging := ""
					if c.Charging {
						charging = " charging"
					}
					writef(out, "    %s %d%%%s\n", c.Cell, c.Level, charging)
				}
			}
		}
		if c := r.Cooling; c != nil {
			if c.Error != "" {
				writef(out, "  cooling: error after %.1f ms: %s\n", c.ElapsedMS, c.Error)
			} else {
				writef(out, "  cooling: coolant %.1f °C  pump %s  fan %s (%.1f ms)\n",
					c.Coolant, speed(c.status.HasPump, c.status.PumpRPM, c.status.PumpDuty),
					speed(c.status.HasFan, c.status.FanRPM, c.status.FanDuty), c.ElapsedMS)
			}
		}
		if r.expected {
			writef(out, "  %s\n", reportHint(r.candidate))
		}
	}
	if idle > 0 {
		names := make([]string, 0, idle)
		for _, r := range reports {
			if r.Batteries == nil && r.Cooling == nil && len(r.Errors) == 0 {
				names = append(names, r.Name)
			}
		}
		writef(out, "%d opened with no readable capability: %s\n", idle, strings.Join(names, ", "))
	}
	for _, s := range sensors {
		if s.Error != "" {
			writef(out, "sensor %s: %s (%.1f ms)\n", s.Sensor, s.Error, s.ElapsedMS)
			continue
		}
		writef(out, "sensor %s: %s  %.1f °C (%.1f ms)\n", s.Sensor, hwmon.Sensor{Chip: s.Chip, Label: s.Label}, *s.Celsius, s.ElapsedMS)
	}
}

func level(j batteryJSON) string {
	switch {
	case j.hasLevel:
		return fmt.Sprintf("%d%%", j.rawLevel)
	case j.Band != "":
		return j.Band
	default:
		return "no level"
	}
}

func speed(has bool, rpm, duty int) string {
	if !has {
		return "not reported"
	}
	return fmt.Sprintf("%d rpm %d%%", rpm, duty)
}
