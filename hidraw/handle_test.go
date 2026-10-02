//go:build linux

// Linux only: a pipe stands in for the node, and only a Linux pipe, like a
// hidraw node, takes a read deadline. On Windows os.Pipe refuses one.

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
An exchange with a device that never answers ends at the deadline, and says
whose deadline it was.

The wrong hidraw node of a device never answers, and a read that blocks forever
is a program that hangs rather than one that reports nothing there. A pipe
stands in for the node: the request written to it comes back as the only
report, which matches nothing, and then there is silence.

Running out of time is ErrSilent whoever set the deadline, and the error also
answers to context.DeadlineExceeded. Cancellation is not silence and comes
back as context.Canceled.
*/
func TestExchangeWithSilenceEndsAtTheDeadline(t *testing.T) {
	pipe := func(t *testing.T) *Handle {
		t.Helper()
		r, w, err := os.Pipe()
		require.NoError(t, err)
		h := &Handle{r: r, w: w, name: "pipe"}
		t.Cleanup(func() { _ = h.Close() })
		return h
	}
	never := func([]byte) bool { return false }
	req := []byte{0x10, 0xff, 0x00, 0x01}

	t.Run("the caller's deadline", func(t *testing.T) {
		const deadline = 100 * time.Millisecond
		ctx, cancel := context.WithTimeout(context.Background(), deadline)
		defer cancel()

		start := time.Now()
		reply, err := Exchange(ctx, pipe(t), req, never, 64)
		elapsed := time.Since(start)

		require.ErrorIs(t, err, ErrSilent)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Nil(t, reply)
		assert.Less(t, elapsed, deadline+100*time.Millisecond, "the exchange outlived its deadline")
	})

	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		reply, err := Exchange(ctx, pipe(t), req, never, 64)

		require.ErrorIs(t, err, context.Canceled)
		assert.NotErrorIs(t, err, ErrSilent, "giving up was reported as the device's silence")
		assert.Nil(t, reply)
	})
}
