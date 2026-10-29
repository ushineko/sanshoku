package powersupply

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
The test that reads the real kernel, and skips where it has no device
battery.

A reading that comes back is checked: a level in range or a band, never both.
*/
func TestLiveKernelBatteriesArePlausible(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	found, err := Driver{}.Find(ctx)
	if errors.Is(err, sanshoku.ErrAbsent) {
		t.Skip("no device battery the kernel reads on this machine")
	}
	require.NoError(t, err)

	for _, c := range found {
		dev, err := c.Open(ctx)
		require.NoError(t, err)
		got, err := dev.(battery.Source).Batteries(ctx)
		require.NoError(t, err)
		for _, b := range got {
			assert.NotEqual(t, b.HasLevel, b.HasBand, "%s: a level or a band", c.Name)
			if b.HasLevel {
				assert.GreaterOrEqual(t, b.Level, 0)
				assert.LessOrEqual(t, b.Level, 100)
			}
		}
		require.NoError(t, dev.Close())
	}
}
