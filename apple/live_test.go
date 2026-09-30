package apple

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
The AirPods on this machine, read over the accessory protocol, and skipped
where BlueZ is not answering or no Apple audio device is connected.

This is the test that matters for this driver. Every other test in the package
drives a channel the test wrote, and the two findings that made hayami's
reader work — the address byte order and EINTR — were both invisible to all of
them. Each produced a working decoder talking to nothing.

**No assertion message names a device or carries an address.** The device is
often named after its owner, and a failure that printed it would put it in a
CI log. Messages use the index; any error is checked for an address and the
name before it is reported.
*/
func TestLiveAirPodsReadingIsPlausible(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	found, err := Driver{}.Find(ctx)
	if errors.Is(err, sanshoku.ErrAbsent) || errors.Is(err, sanshoku.ErrUnavailable) {
		t.Skip("bluez is not answering, or no Apple audio device is connected")
	}
	require.NoError(t, err)

	for i, c := range found {
		dev, err := c.Open(ctx)
		require.NoError(t, err)
		got, err := dev.(battery.Source).Batteries(ctx)
		require.NoError(t, dev.Close())
		if err != nil {
			assert.NotRegexp(t, anAddress, err.Error(), "device %d: an error carried an address", i)
			assert.NotContains(t, err.Error(), c.Name, "device %d: an error named the device", i)
			t.Skipf("device %d did not answer the accessory protocol and has no Battery1 level", i)
		}
		for _, b := range got {
			assert.True(t, b.HasLevel, "device %d", i)
			assert.GreaterOrEqual(t, b.Level, 0, "device %d", i)
			assert.LessOrEqual(t, b.Level, 100, "device %d", i)
			for _, cell := range b.Cells {
				assert.GreaterOrEqual(t, cell.Level, 0, "device %d cell %s", i, cell.Cell)
				assert.LessOrEqual(t, cell.Level, 100, "device %d cell %s", i, cell.Cell)
			}
			// The cells are ordered for reading, whatever order the firmware
			// sent them in. The pair hayami measured reports right first.
			for j := 1; j < len(b.Cells); j++ {
				assert.Less(t, b.Cells[j-1].Cell, b.Cells[j].Cell, "device %d: cells out of order", i)
			}
		}
	}
}
