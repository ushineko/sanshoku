package sony

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

A controller that is asleep has no interface to find, and one that sends
nothing in time is no reading and passes; a reading that comes back is
checked.
*/
func TestLiveDualSenseReadingsArePlausible(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	found, err := Driver{}.Find(ctx)
	if errors.Is(err, sanshoku.ErrAbsent) {
		t.Skip("no DualSense on this machine")
	}
	require.NoError(t, err)

	for _, c := range found {
		dev, err := c.Open(ctx)
		if errors.Is(err, sanshoku.ErrUnsupported) {
			continue
		}
		require.NoError(t, err, "%s did not open; see docs/udev.md", c.Path)
		got, err := dev.(battery.Source).Batteries(ctx)
		assert.NoError(t, err, c.Path)
		for _, b := range got {
			assert.Equal(t, battery.KindGamepad, b.Kind)
			assert.True(t, b.HasLevel)
			assert.GreaterOrEqual(t, b.Level, 5)
			assert.LessOrEqual(t, b.Level, 100)
		}
		require.NoError(t, dev.Close())
	}
}
