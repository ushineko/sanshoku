package steelseries

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/battery"
)

/*
The test that touches the real hardware, and skips where there is none.

A product the allow-list does not name is refused at Open and is not a
failure here: that is the driver doing its job. hayami had no live test for
this driver; the bench is the oracle.

The Arctis Nova Pro's base station, when it is here and its headset is on, is
also checked against headsetcontrol when that is on PATH: the level is exact,
since both read the same byte through the same arithmetic.
*/
func TestLiveSteelSeriesReadingsArePlausible(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	found, err := Driver{}.Find(ctx)
	if errors.Is(err, sanshoku.ErrAbsent) {
		t.Skip("no SteelSeries control node on this machine")
	}
	require.NoError(t, err)

	opened := 0
	for _, c := range found {
		dev, err := c.Open(ctx)
		if errors.Is(err, sanshoku.ErrUnsupported) {
			continue
		}
		require.NoError(t, err, "%s did not open; see docs/udev.md", c.Path)
		opened++
		got, err := dev.(battery.Source).Batteries(ctx)
		assert.NoError(t, err, c.Path)
		for _, b := range got {
			assert.NotEmpty(t, b.Name)
			assert.GreaterOrEqual(t, b.Level, 0)
			assert.LessOrEqual(t, b.Level, 100)
			if products[c.Product] == novaPro && b.HasLevel {
				checkAgainstHeadsetcontrol(ctx, t, c.Product, b.Level)
			}
		}
		require.NoError(t, dev.Close())
	}
	if opened == 0 {
		t.Skip("every SteelSeries node here is a product this driver does not speak to")
	}
}

// checkAgainstHeadsetcontrol compares a Nova Pro level with what
// `headsetcontrol -o json` reports for the same product, when it is on PATH.
func checkAgainstHeadsetcontrol(ctx context.Context, t *testing.T, product uint16, level int) {
	t.Helper()
	if _, err := exec.LookPath("headsetcontrol"); err != nil {
		t.Log("headsetcontrol is not on PATH; the level is not compared")
		return
	}
	stdout, err := exec.CommandContext(ctx, "headsetcontrol", "-o", "json").Output()
	require.NoError(t, err)
	var report struct {
		Devices []struct {
			Product string `json:"id_product"`
			Battery *struct {
				Status string `json:"status"`
				Level  int    `json:"level"`
			} `json:"battery"`
		} `json:"devices"`
	}
	require.NoError(t, json.Unmarshal(stdout, &report))
	for _, d := range report.Devices {
		if d.Product == fmt.Sprintf("0x%04x", product) && d.Battery != nil && d.Battery.Status != "BATTERY_UNAVAILABLE" {
			assert.Equal(t, d.Battery.Level, level, "headsetcontrol's level")
			return
		}
	}
	t.Log("headsetcontrol reported no level for the base station; not compared")
}
