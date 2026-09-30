package bluez

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
The test that asks the real BlueZ, and skips where there is none or where
nothing connected reports a level.

hayami never ran its Battery1 path against a real device (hayami issue #24);
this and the bench are where it is first checked.

**No assertion message names a device or carries an address.** The live tests
read whatever is connected, which is often a device named after its owner, and
a failure that printed it would put it in a CI log. Messages use the index;
and every error this test sees is checked for an address and a name before it
is allowed near an assertion message.
*/
func TestLiveBatteryOneReadingsArePlausible(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	found, err := Driver{}.Find(ctx)
	if errors.Is(err, sanshoku.ErrAbsent) {
		t.Skip("bluez is not answering, or nothing connected reports a Battery1 level")
	}
	require.NoError(t, err)

	for i, c := range found {
		dev, err := c.Open(ctx)
		require.NoError(t, err)
		got, err := dev.(battery.Source).Batteries(ctx)
		if err != nil {
			assert.NotRegexp(t, anAddress, err.Error(), "device %d: an error carried an address", i)
			assert.NotContains(t, err.Error(), c.Name, "device %d: an error named the device", i)
			assert.Fail(t, "a Battery1 read failed", "device %d", i)
		}
		for _, b := range got {
			assert.True(t, b.HasLevel, "device %d", i)
			assert.GreaterOrEqual(t, b.Level, 0, "device %d", i)
			assert.LessOrEqual(t, b.Level, 100, "device %d", i)
			assert.NotEmpty(t, b.Name, "device %d reported no name", i)
		}
		require.NoError(t, dev.Close())
	}
}
