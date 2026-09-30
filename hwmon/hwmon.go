package hwmon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ushineko/sanshoku"
)

// Root is where the kernel puts its sensors. A variable so a test can point
// it at a tree it wrote itself.
var Root = "/sys/class/hwmon"

// ErrNoSensor is a sensor this machine does not have. It wraps
// sanshoku.ErrAbsent: a reading that is absent, not a failure, and a program
// on a machine with a different processor should show the rest.
var ErrNoSensor error = absent("no such sensor on this machine")

// absent is an error that is sanshoku.ErrAbsent underneath without saying
// "absent" in its message.
type absent string

func (a absent) Error() string { return string(a) }
func (absent) Unwrap() error   { return sanshoku.ErrAbsent }

/*
Sensor is one labelled temperature the kernel already exposes.

**Read by label, never by hwmon index.** `coretemp` was hwmon10 when hayami was
written and will be something else after a reboot; a program that remembers the
number reports another chip's temperature rather than failing, which is the
worst way to be wrong.
*/
type Sensor struct {
	// Chip is the hwmon name: "coretemp", "k10temp", "amdgpu".
	Chip string

	// Label is the sensor within it: "Package id 0". Empty means the chip's
	// first temperature, for a chip that labels nothing: nouveau exposes
	// temp1_input and nothing else.
	Label string
}

// String is chip/label, or the chip alone for an unlabelled sensor: the form
// an error names a sensor in.
func (s Sensor) String() string {
	if s.Label == "" {
		return s.Chip
	}
	return s.Chip + "/" + s.Label
}

/*
CPU are the processor temperatures this module knows, most wanted first.

**One constant was one vendor.** hayami named `coretemp`, which is Intel's
driver, so a Ryzen read nothing at all and said "no coretemp/Package id 0":
precise about what it looked for and silent about only ever looking for one
thing (hayami issue #72).

The order is not alphabetical and not arbitrary:

  - `coretemp` / `Package id 0` is Intel's package temperature.
  - `k10temp` / `Tdie` is AMD's die temperature, where a chip exposes it.
  - `k10temp` / `Tctl` is the fallback, and **it is not a temperature.** It is
    a control value carrying a per-model offset the firmware uses for fan
    curves, and on some chips it reads several degrees above the die. It is
    last among AMD's because it is the one to take when there is nothing
    better; a 2600X exposes only this. Do not "tidy" it above Tdie.
  - `zenpower` / `Tdie` is the out-of-tree AMD driver some people run instead
    of k10temp, which exposes a die temperature where k10temp gives only Tctl.

The first that reads wins, so a machine with both k10temp and zenpower loaded
gets the in-tree one, which is the one its fan curves are built on.
*/
var CPU = []Sensor{
	{Chip: "coretemp", Label: "Package id 0"},
	{Chip: "k10temp", Label: "Tdie"},
	{Chip: "k10temp", Label: "Tctl"},
	{Chip: "zenpower", Label: "Tdie"},
}

/*
GPU are the kernel's ways of knowing a graphics card's temperature, in the
order they are tried.

AMD labels its sensors; nouveau exposes one unlabelled temperature. NVIDIA's
proprietary driver registers no hwmon at all, so a machine running it reads
nothing here; hotaru falls back to nvidia-smi, which stays in hotaru because
NVML is a vendor library and not a kernel node.
*/
var GPU = []Sensor{
	{Chip: "amdgpu", Label: "edge"},
	{Chip: "amdgpu"},
	{Chip: "nouveau"},
}

/*
First returns the first of sensors that reads under root, and its value in
degrees.

A machine with none of them gets an error naming every sensor looked for, not
just the last: "no coretemp/Package id 0" on an AMD box sent somebody looking
for an Intel driver that was never going to be there.
*/
func First(root string, sensors []Sensor) (Sensor, float64, error) {
	names := make([]string, 0, len(sensors))
	for _, s := range sensors {
		if v, err := s.Read(root); err == nil {
			return s, v, nil
		}
		names = append(names, s.String())
	}
	return Sensor{}, 0, fmt.Errorf("%w: looked for %s", ErrNoSensor, strings.Join(names, ", "))
}

// Read is the sensor's temperature in degrees under root: the chip found by
// its name, then the sensor by its label.
func (s Sensor) Read(root string) (float64, error) {
	dirs, err := os.ReadDir(root)
	if err != nil {
		return 0, ErrNoSensor
	}
	for _, d := range dirs {
		dir := filepath.Join(root, d.Name())
		name, err := text(filepath.Join(dir, "name"))
		if err != nil || name != s.Chip {
			continue
		}
		if v, err := s.readChip(dir); err == nil {
			return v, nil
		}
	}
	return 0, ErrNoSensor
}

// readChip finds the labelled temperature inside one chip's directory.
func (s Sensor) readChip(dir string) (float64, error) {
	if s.Label == "" {
		return milli(filepath.Join(dir, "temp1_input"))
	}
	labels, err := filepath.Glob(filepath.Join(dir, "temp*_label"))
	if err != nil {
		return 0, ErrNoSensor
	}
	for _, path := range labels {
		got, err := text(path)
		if err != nil || got != s.Label {
			continue
		}
		return milli(strings.TrimSuffix(path, "_label") + "_input")
	}
	return 0, ErrNoSensor
}

// text is a one-line sysfs file, trimmed.
func text(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err //nolint:wrapcheck // every caller turns a failure into ErrNoSensor
	}
	return strings.TrimSpace(string(b)), nil
}

// milli reads a sysfs temperature, which the kernel writes in thousandths.
func milli(path string) (float64, error) {
	s, err := text(path)
	if err != nil {
		return 0, ErrNoSensor
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, ErrNoSensor
	}
	return v / 1000, nil
}

// chips lists the hwmon directories under root with their names. A root
// that does not exist is no chips.
func chips(root string) ([]chip, error) {
	dirs, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing hwmon chips: %w", err)
	}
	var out []chip
	for _, d := range dirs {
		dir := filepath.Join(root, d.Name())
		name, err := text(filepath.Join(dir, "name"))
		if err != nil || name == "" {
			continue
		}
		out = append(out, chip{name: name, dir: dir})
	}
	return out, nil
}

type chip struct{ name, dir string }
