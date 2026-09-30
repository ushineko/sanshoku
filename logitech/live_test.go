package logitech

import (
	"context"
	"errors"
	"os/exec"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/battery"
)

/*
The test that touches the real hardware, and skips where there is none.

Every other test in this package drives a transport the test wrote, and a
decoder passing on recorded bytes is not evidence that the exchange works: a
request that is well formed and never answered, a node chosen that does not
speak, a permission that is not there each pass every unit test and read
nothing on the desk.
*/
func TestLiveLogitechReadingsArePlausibleAndAgreeWithSolaar(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	found, err := Driver{}.Find(ctx)
	if errors.Is(err, sanshoku.ErrAbsent) {
		t.Skip("no Logitech HID++ node on this machine")
	}
	require.NoError(t, err)

	var readings []battery.Battery
	for _, c := range found {
		dev, err := c.Open(ctx)
		require.NoError(t, err, "%s did not open; see docs/udev.md", c.Path)
		got, err := dev.(battery.Source).Batteries(ctx)
		assert.NoError(t, err, c.Path)
		readings = append(readings, got...)
		require.NoError(t, dev.Close())
	}
	for _, b := range readings {
		assert.NotEmpty(t, b.Name)
		if b.HasLevel {
			assert.GreaterOrEqual(t, b.Level, 0)
			assert.LessOrEqual(t, b.Level, 100)
		}
	}

	if len(readings) == 0 {
		t.Skip("a HID++ node is here but no device on it answered")
	}
	if _, err := exec.LookPath("solaar"); err != nil {
		t.Skip("solaar is not installed, so there is nothing to agree with")
	}
	out, err := exec.CommandContext(ctx, "solaar", "show").CombinedOutput()
	require.NoError(t, err, "solaar show failed")
	levels := solaarLevels(string(out))
	if len(levels) == 0 {
		t.Skip("solaar reported no battery to compare against")
	}
	for _, b := range readings {
		if b.HasLevel {
			assert.Contains(t, levels, b.Level, "read %d %% over HID++; solaar reported %v", b.Level, levels)
		}
	}
}

// solaarBattery is solaar's battery line: "Battery: 86%, BatteryStatus.DISCHARGING."
var solaarBattery = regexp.MustCompile(`Battery: (\d+)%`)

// solaarLevels pulls the percentages out of `solaar show`.
func solaarLevels(out string) []int {
	var levels []int
	for _, m := range solaarBattery.FindAllStringSubmatch(out, -1) {
		if n, err := strconv.Atoi(m[1]); err == nil {
			levels = append(levels, n)
		}
	}
	return levels
}
