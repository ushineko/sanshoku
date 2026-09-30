package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// toolTimeout bounds one cross-checking tool. `solaar show` took 3.5 s on
// the machine hayami measured it on.
const toolTimeout = 20 * time.Second

/*
The tolerances against liquidctl, from hotaru's live test.

Compared with tolerance because these are live numbers: a pump drifts by a few
rpm between two readings a fraction of a second apart, so an exact comparison
tests the weather. What is checked is that both implementations read the same
fields of the same report; a byte offset taken off the wrong field is wrong by
hundreds, not by five.
*/
const (
	coolantDelta   = 1.0  // degrees, absolute
	pumpEpsilon    = 0.10 // relative
	fanEpsilon     = 0.15 // relative
	liquidctlMatch = "kraken"
)

// verify takes every reading and cross-checks it against a tool that is on
// PATH. A missing tool is "not checked", not a failure. Exit 1 on any
// disagreement.
func verify(ctx context.Context, args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(errOut)
	driver := fs.String("driver", "", "restrict to one driver")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	ds, err := drivers(*driver)
	if err != nil {
		writeln(errOut, err)
		return 2
	}
	reports, scanErr := readAll(ctx, ds)
	if scanErr != nil {
		writef(errOut, "scan: %v\n", scanErr)
	}

	disagree := verifyCooling(ctx, out, reports)
	disagree = verifyBatteries(ctx, out, reports) || disagree
	disagree = verifyHeadsets(ctx, out, reports) || disagree

	if disagree {
		return 1
	}
	return 0
}

// verifyCooling compares every cooling reading with liquidctl's.
func verifyCooling(ctx context.Context, out io.Writer, reports []deviceReport) bool {
	var readings []deviceReport
	for _, r := range reports {
		if r.Cooling != nil && r.Cooling.Error == "" {
			readings = append(readings, r)
		}
	}
	if _, err := exec.LookPath("liquidctl"); err != nil {
		writeln(out, "liquidctl: not on PATH, cooling not checked")
		return false
	}
	if len(readings) == 0 {
		writeln(out, "liquidctl: on PATH; no cooling reading to compare")
		return false
	}
	theirs, err := liquidctlStatus(ctx)
	if err != nil {
		writef(out, "liquidctl: %v; cooling not checked\n", err)
		return false
	}
	disagree := false
	for _, r := range readings {
		s := r.Cooling.status
		checks := []struct {
			what   string
			mine   float64
			key    string
			has    bool
			within func(a, b float64) bool
		}{
			{"coolant", s.Coolant, "liquid temperature", true, func(a, b float64) bool { return math.Abs(a-b) <= coolantDelta }},
			{"pump", float64(s.PumpRPM), "pump speed", s.HasPump, relative(pumpEpsilon)},
			{"fan", float64(s.FanRPM), "fan speed", s.HasFan, relative(fanEpsilon)},
		}
		for _, c := range checks {
			if !c.has {
				continue
			}
			v, ok := theirs[c.key]
			if !ok {
				writef(out, "%s %s: liquidctl did not report %q, not checked\n", r.Name, c.what, c.key)
				continue
			}
			verdict := "agree"
			if !c.within(c.mine, v) {
				verdict, disagree = "DISAGREE", true
			}
			writef(out, "%s %s: %s (ours %g, liquidctl %g)\n", r.Name, c.what, verdict, c.mine, v)
		}
	}
	return disagree
}

func relative(eps float64) func(a, b float64) bool {
	return func(a, b float64) bool {
		if b == 0 {
			return a == 0
		}
		return math.Abs(a-b)/math.Abs(b) <= eps
	}
}

// liquidctlStatus runs `liquidctl --json --match kraken status` and returns
// the first matching device's values by lower-case key. The device's bus
// address is in the output and is not kept.
func liquidctlStatus(ctx context.Context) (map[string]float64, error) {
	stdout, err := runTool(ctx, "liquidctl", "--json", "--match", liquidctlMatch, "status")
	if err != nil {
		return nil, err
	}
	var devices []struct {
		Status []struct {
			Key   string `json:"key"`
			Value any    `json:"value"`
		} `json:"status"`
	}
	if err := json.Unmarshal(stdout, &devices); err != nil {
		return nil, fmt.Errorf("reading its output: %w", err)
	}
	if len(devices) == 0 {
		return nil, errors.New("no device matched")
	}
	values := map[string]float64{}
	for _, s := range devices[0].Status {
		if v, ok := s.Value.(float64); ok {
			values[strings.ToLower(s.Key)] = v
		}
	}
	return values, nil
}

// verifyBatteries compares every battery level with solaar's.
func verifyBatteries(ctx context.Context, out io.Writer, reports []deviceReport) bool {
	type level struct {
		name  string
		level int
	}
	var mine []level
	for _, r := range reports {
		if r.Driver != "logitech" || r.Batteries == nil {
			continue
		}
		for _, b := range r.Batteries.Readings {
			if b.hasLevel {
				mine = append(mine, level{b.Name, b.rawLevel})
			}
		}
	}
	if _, err := exec.LookPath("solaar"); err != nil {
		writeln(out, "solaar: not on PATH, HID++ levels not checked")
		return false
	}
	if len(mine) == 0 {
		writeln(out, "solaar: on PATH; no HID++ level to compare")
		return false
	}
	stdout, err := runTool(ctx, "solaar", "show")
	if err != nil {
		writef(out, "solaar: %v; HID++ levels not checked\n", err)
		return false
	}
	theirs := solaarLevels(stdout)
	disagree := false
	for _, m := range mine {
		v, ok := theirs[strings.ToLower(m.name)]
		if !ok {
			writef(out, "%s battery: solaar did not report it, not checked\n", m.name)
			continue
		}
		verdict := "agree"
		if v != m.level {
			verdict, disagree = "DISAGREE", true
		}
		writef(out, "%s battery: %s (ours %d%%, solaar %d%%)\n", m.name, verdict, m.level, v)
	}
	return disagree
}

/*
headsetStates is how headsetcontrol's battery status reads as a battery.State,
for the two it reports with a level. BATTERY_UNAVAILABLE is a headset that is
off, which is a reading with no level here and is not compared.
*/
var headsetStates = map[string]string{
	"BATTERY_CHARGING":  "charging",
	"BATTERY_AVAILABLE": "discharging",
}

// verifyHeadsets compares every SteelSeries headset level with
// `headsetcontrol -o json`: the level exactly, since both read the same byte
// through the same arithmetic, and the status by headsetStates. Full is ours
// alone; headsetcontrol calls it charging.
func verifyHeadsets(ctx context.Context, out io.Writer, reports []deviceReport) bool {
	type reading struct {
		name, product string
		b             batteryJSON
	}
	var mine []reading
	for _, r := range reports {
		if r.Driver != "steelseries" || r.Batteries == nil {
			continue
		}
		for _, b := range r.Batteries.Readings {
			if b.Kind == "headset" {
				mine = append(mine, reading{r.Name, "0x" + r.Product, b})
			}
		}
	}
	if _, err := exec.LookPath("headsetcontrol"); err != nil {
		writeln(out, "headsetcontrol: not on PATH, headsets not checked")
		return false
	}
	if len(mine) == 0 {
		writeln(out, "headsetcontrol: on PATH; no headset reading to compare")
		return false
	}
	stdout, err := runTool(ctx, "headsetcontrol", "-o", "json")
	if err != nil {
		writef(out, "headsetcontrol: %v; headsets not checked\n", err)
		return false
	}
	var theirs struct {
		Devices []struct {
			Product string `json:"id_product"`
			Battery *struct {
				Status string `json:"status"`
				Level  int    `json:"level"`
			} `json:"battery"`
		} `json:"devices"`
	}
	if err := json.Unmarshal(stdout, &theirs); err != nil {
		writef(out, "headsetcontrol: reading its output: %v; headsets not checked\n", err)
		return false
	}
	disagree := false
	for _, m := range mine {
		if !m.b.hasLevel {
			writef(out, "%s battery: headset off, no level; not compared\n", m.name)
			continue
		}
		compared := false
		for _, d := range theirs.Devices {
			if !strings.EqualFold(d.Product, m.product) || d.Battery == nil {
				continue
			}
			compared = true
			state, known := headsetStates[d.Battery.Status]
			if m.b.State == "full" {
				m.b.State = "charging"
			}
			verdict := "agree"
			if !known || d.Battery.Level != m.b.rawLevel || state != m.b.State {
				verdict, disagree = "DISAGREE", true
			}
			writef(out, "%s battery: %s (ours %d%% %s, headsetcontrol %d%% %s)\n",
				m.name, verdict, m.b.rawLevel, m.b.State, d.Battery.Level, d.Battery.Status)
			break
		}
		if !compared {
			writef(out, "%s battery: headsetcontrol did not report it, not checked\n", m.name)
		}
	}
	return disagree
}

var (
	// solaarDevice is a paired device's heading in `solaar show`: "  1: G502 X PLUS".
	solaarDevice = regexp.MustCompile(`^  \d+: (.+)$`)
	// solaarTop is an unindented heading: a receiver or a wired device.
	solaarTop     = regexp.MustCompile(`^(\S.*)$`)
	solaarBattery = regexp.MustCompile(`Battery: (\d+)%`)
)

// solaarLevels reads the first battery percentage under each device heading
// of `solaar show`, keyed by the lower-case device name.
func solaarLevels(report []byte) map[string]int {
	levels := map[string]int{}
	current := ""
	sc := bufio.NewScanner(bytes.NewReader(report))
	for sc.Scan() {
		line := sc.Text()
		if m := solaarDevice.FindStringSubmatch(line); m != nil {
			current = strings.ToLower(strings.TrimSpace(m[1]))
			continue
		}
		if m := solaarTop.FindStringSubmatch(line); m != nil {
			current = strings.ToLower(strings.TrimSpace(m[1]))
			continue
		}
		if m := solaarBattery.FindStringSubmatch(line); m != nil && current != "" {
			if _, seen := levels[current]; !seen {
				if v, err := strconv.Atoi(m[1]); err == nil {
					levels[current] = v
				}
			}
		}
	}
	return levels
}

// runTool runs a cross-checking tool with a bound, and returns its stdout.
// Arguments are separate, never a shell string.
func runTool(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, toolTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // every caller passes a constant tool name and constant arguments
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	if err != nil {
		first, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n")
		return nil, fmt.Errorf("%s failed: %w: %s", name, err, first)
	}
	return stdout, nil
}
