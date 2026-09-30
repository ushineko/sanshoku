package hidraw

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
An exchange with a device that never answers ends at the deadline, with the
context's error.

The wrong hidraw node of a device never answers, and a read that blocks forever
is a program that hangs rather than one that reports nothing there. A pipe
stands in for the node: the request written to it comes back as the only
report, which matches nothing, and then there is silence.
*/
func TestExchangeWithSilenceEndsAtTheDeadline(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	h := &Handle{r: r, w: w, name: "pipe"}
	t.Cleanup(func() { _ = h.Close() })

	const deadline = 100 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()

	start := time.Now()
	reply, err := Exchange(ctx, h, []byte{0x10, 0xff, 0x00, 0x01}, func([]byte) bool { return false }, 64)
	elapsed := time.Since(start)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Nil(t, reply)
	assert.Less(t, elapsed, deadline+100*time.Millisecond, "the exchange outlived its deadline")
}
