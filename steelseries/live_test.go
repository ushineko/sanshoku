package steelseries

import (
	"context"
	"errors"
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
		}
		require.NoError(t, dev.Close())
	}
	if opened == 0 {
		t.Skip("every SteelSeries node here is a product this driver does not speak to")
	}
}
