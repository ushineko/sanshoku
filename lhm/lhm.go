package lhm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// URL is where LibreHardwareMonitor's web server serves its sensor tree when
// it is left on its default port.
const URL = "http://127.0.0.1:8085/data.json"

// Timeout bounds one request made with a client that has no timeout of its
// own. The server answers a loopback request in milliseconds when it answers
// at all, and a poll must not wait on one that does not.
const Timeout = time.Second

// maxBody caps what is read of an answer. The tree on a desk with two
// drives, three memory modules and a graphics card is about 60 KiB; a body
// many times that is not the tree, and is not worth holding.
const maxBody = 8 << 20

/*
CPULabels are the processor temperatures looked for, in order: the first a
machine has is the one read. By label, never by index, for the reason hwmon
sensors are: the sensors' numbers in LibreHardwareMonitor's identifiers are
the order it found them in, and a version that finds one more moves every
number after it.

The order is hwmon.CPU's, for the reason it gives. Tdie first, because Tctl
on some Ryzen parts carries an offset the fan curve wants and a person does
not; where LibreHardwareMonitor knows no offset it publishes one sensor named
for both, which is next; Tctl alone after that. Intel's package temperature
last, under its own name.
*/
var CPULabels = []string{"Core (Tdie)", "Core (Tctl/Tdie)", "Core (Tctl)", "CPU Package"}

// cpuPrefixes are the identifier prefixes of LibreHardwareMonitor's
// processor hardware. A graphics card has a "GPU Core" temperature too, and
// matching a label alone would read it.
var cpuPrefixes = []string{"/amdcpu/", "/intelcpu/"}

/*
Node is one node of LibreHardwareMonitor's sensor tree, as data.json carries
it: hardware, a group of sensors, or a sensor.

**Value and RawValue are text**, "51.5 °C", formatted by the server. Both are
read through Text, which takes a number too, because the format has changed
between releases and is not documented. SensorID is not unique: one measured
tree has "/gpu-nvidia/0/load/3" twice. Nothing here keys on it.
*/
type Node struct {
	Text       string `json:"Text"`
	SensorID   string `json:"SensorId"`
	Type       string `json:"Type"`
	Value      Text   `json:"Value"`
	RawValue   Text   `json:"RawValue"`
	HardwareID string `json:"HardwareId"`
	Children   []Node `json:"Children"`
}

// Text is a field that is text in every answer measured and may be a number
// in another.
type Text string

// UnmarshalJSON takes a string, a number, or null.
func (t *Text) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*t = Text(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err == nil {
		*t = Text(n.String())
		return nil
	}
	*t = ""
	return nil
}

/*
ParseValue is the figure at the front of a value: "51.5 °C" is 51.5.

The decimal mark is whichever the server's locale writes, so a comma is read
as one too: "51,5 °C" is 51.5. LibreHardwareMonitor writes no thousands
separator in a temperature, so a comma is never one here.
*/
func ParseValue(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	end := 0
	for end < len(s) && (s[end] == '-' || s[end] == '.' || s[end] == ',' || (s[end] >= '0' && s[end] <= '9')) {
		end++
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(s[:end], ",", "."), 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// CPUTemperature is the processor's temperature in a tree, and the label it
// was read under, in CPULabels' order.
func CPUTemperature(root Node) (degrees float64, label string, ok bool) {
	found := map[string]float64{}
	var walk func(n Node)
	walk = func(n Node) {
		if n.Type == "Temperature" && cpuSensor(n.SensorID) {
			if _, seen := found[n.Text]; !seen {
				if v, ok := reading(n); ok {
					found[n.Text] = v
				}
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
	for _, label := range CPULabels {
		if v, ok := found[label]; ok {
			return v, label, true
		}
	}
	return 0, "", false
}

// reading is a sensor's figure: the raw value where it parses, the shown one
// where it does not.
func reading(n Node) (float64, bool) {
	if v, ok := ParseValue(string(n.RawValue)); ok {
		return v, true
	}
	return ParseValue(string(n.Value))
}

func cpuSensor(id string) bool {
	for _, p := range cpuPrefixes {
		if strings.HasPrefix(id, p) {
			return true
		}
	}
	return false
}

// The ways LibreHardwareMonitor can fail to give a temperature. A consumer
// tells them apart to say what a person should do about each; ProbeHost says
// which of the reasons for ErrUnreachable a machine has.
var (
	// ErrUnreachable is nothing answering at the address.
	ErrUnreachable = errors.New("LibreHardwareMonitor's web server is not answering")
	// ErrAuth is a server that wants a password. This package sends none.
	ErrAuth = errors.New("LibreHardwareMonitor's web server wants a password")
	// ErrNoSensor is a server that answered with no processor temperature,
	// which is what it serves when its PawnIO driver is not installed.
	ErrNoSensor = errors.New("LibreHardwareMonitor has no CPU temperature")
	// ErrTooLarge is an answer too large to be a sensor tree.
	ErrTooLarge = errors.New("the answer is not a sensor tree")
)

/*
Fetch reads the sensor tree at url, within the client's timeout, or Timeout
for a client that has none.

It only ever asks for the tree: the same server takes requests that set a
fan's speed, and nothing here makes one. The address is the caller's. This
package does not restrict it to loopback, because LibreHardwareMonitor's port
is a setting and a person may point a program at another machine's; a
consumer that wants loopback only passes URL.

A cancelled context is returned as the context's error. Everything else that
stops an answer arriving is ErrUnreachable.
*/
func Fetch(ctx context.Context, client *http.Client, url string) (Node, error) {
	if client.Timeout == 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, Timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Node{}, fmt.Errorf("asking %s: %w", url, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return Node{}, fmt.Errorf("asking %s: %w", url, ctx.Err())
		}
		return Node{}, fmt.Errorf("%w at %s: %w", ErrUnreachable, url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return Node{}, fmt.Errorf("%w at %s", ErrAuth, url)
	case resp.StatusCode != http.StatusOK:
		return Node{}, fmt.Errorf("%s answered %s", url, resp.Status)
	}

	// An answer that says it is over the cap is refused before a byte of it is
	// read; one that does not say (chunked) is read up to the cap and no
	// further.
	if resp.ContentLength > maxBody {
		return Node{}, tooLarge(url)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return Node{}, fmt.Errorf("reading %s: %w", url, err)
	}
	if len(body) > maxBody {
		return Node{}, tooLarge(url)
	}
	var root Node
	if err := json.Unmarshal(body, &root); err != nil {
		return Node{}, fmt.Errorf("%s did not answer with a sensor tree: %w", url, err)
	}
	return root, nil
}

// tooLarge is the error for an answer over maxBody.
func tooLarge(url string) error {
	return fmt.Errorf("%s answered more than %d MiB: %w", url, maxBody>>20, ErrTooLarge)
}

/*
Read is the processor's temperature from the server at url, and the label it
was read under: Fetch, then CPUTemperature. A tree with no processor
temperature is ErrNoSensor.
*/
func Read(ctx context.Context, client *http.Client, url string) (degrees float64, label string, err error) {
	root, err := Fetch(ctx, client, url)
	if err != nil {
		return 0, "", err
	}
	degrees, label, ok := CPUTemperature(root)
	if !ok {
		return 0, "", fmt.Errorf("%w at %s", ErrNoSensor, url)
	}
	return degrees, label, nil
}

/*
Host is what a machine has of LibreHardwareMonitor: the reason, when its
server does not answer, that tells one remedy from another.

A LibreHardwareMonitor that is running has its web server off. One that is
not running was closed with its startup task in place, or has no startup
task, or is not installed, which a machine without PawnIO says. Closing its
window quits it unless its Minimize On Close option is on, which is the usual
way it stops.
*/
type Host struct {
	// PawnIO is whether the PawnIO driver's service is installed.
	PawnIO bool
	// Running is whether a LibreHardwareMonitor process is running.
	Running bool
	// Task is whether LibreHardwareMonitor's startup task is registered: the
	// one its own Options > Run On Windows Startup makes.
	Task bool
}
