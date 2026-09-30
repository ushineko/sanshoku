package razer

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

Every other test in this package drives a transport the test wrote. A decoder
passing on built bytes is not evidence that the ioctls reach the device, that
the node chosen is the one that answers, or that the settle time is long
enough. hayami had no live test for this driver; the bench is the oracle.
*/
func TestLiveRazerReadingsArePlausible(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	found, err := Driver{}.Find(ctx)
	if errors.Is(err, sanshoku.ErrAbsent) {
		t.Skip("no Razer control node on this machine")
	}
	require.NoError(t, err)

	for _, c := range found {
		dev, err := c.Open(ctx)
		require.NoError(t, err, "%s did not open; see docs/udev.md", c.Path)
		got, err := dev.(battery.Source).Batteries(ctx)
		assert.NoError(t, err, c.Path)
		for _, b := range got {
			assert.NotEmpty(t, b.Name)
			assert.True(t, b.HasLevel)
			assert.GreaterOrEqual(t, b.Level, 0)
			assert.LessOrEqual(t, b.Level, 100)
		}
		require.NoError(t, dev.Close())
	}
}
