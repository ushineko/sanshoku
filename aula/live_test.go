package aula

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

Every other test in this package drives a transport the test wrote, and a
decoder passing on captured bytes is not evidence that the question reaches
the receiver or that the reply comes back on the collection read. A keyboard
asleep answers nothing, which is no reading and passes; a reading that comes
back is checked.
*/
func TestLiveAulaReadingsArePlausible(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	found, err := Driver{}.Find(ctx)
	if errors.Is(err, sanshoku.ErrAbsent) {
		t.Skip("no AULA receiver on this machine")
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
			assert.Equal(t, "F75", b.Name)
			assert.Equal(t, battery.KindKeyboard, b.Kind)
			if b.HasLevel {
				assert.GreaterOrEqual(t, b.Level, 1)
				assert.LessOrEqual(t, b.Level, 100)
			} else {
				assert.Equal(t, battery.Charging, b.State, "a reading with no level that is not the cable's")
			}
		}
		require.NoError(t, dev.Close())
	}
}
