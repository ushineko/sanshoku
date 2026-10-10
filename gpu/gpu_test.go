package gpu_test

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/gpu"
)

// A machine with no card is absent, not a failure: the error is one a program
// passes over.
func TestNoCardIsAbsent(t *testing.T) {
	require.ErrorIs(t, gpu.ErrNoCard, sanshoku.ErrAbsent)
	assert.NotContains(t, gpu.ErrNoCard.Error(), "absent")
}

/*
A read answers on whatever this machine is, and says which. On Windows that is
a card or ErrNoCard (a CI runner has the Basic Render Driver and nothing
else); elsewhere it is unsupported. What a card read is logged rather than
asserted, since the suite does not require one.
*/
func TestAReadAnswersOnAnyMachine(t *testing.T) {
	var r gpu.Reader
	t.Cleanup(func() { require.NoError(t, r.Close()) })

	card, err := r.Read(t.Context())
	switch {
	case runtime.GOOS != "windows":
		require.ErrorIs(t, err, errors.ErrUnsupported)
	case err != nil:
		require.ErrorIs(t, err, gpu.ErrNoCard)
	default:
		t.Logf("card: %q, temperature %v (%.1f), load %v (%.0f %%)",
			card.Name, card.HasTemperature, card.Temperature, card.HasLoad, card.Load)
		if card.HasLoad {
			assert.GreaterOrEqual(t, card.Load, 0.0)
			assert.LessOrEqual(t, card.Load, 100.0)
		}
	}
}

// A read whose context is already over does not wait out the warm-up, and a
// closed reader reads again.
func TestACancelledReadReturnsAtOnceAndACloseIsNotTheEnd(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the warm-up and the query are Windows'")
	}
	var r gpu.Reader
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	start := time.Now()
	_, err := r.Read(ctx)
	require.ErrorIs(t, err, context.Canceled)
	assert.Less(t, time.Since(start), gpu.Warmup)

	require.NoError(t, r.Close())
	_, err = r.Read(t.Context())
	if err != nil {
		require.ErrorIs(t, err, gpu.ErrNoCard)
	}
	require.NoError(t, r.Close())
}
