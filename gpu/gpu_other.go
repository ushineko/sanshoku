//go:build !windows

package gpu

import (
	"context"
	"errors"
	"fmt"
)

/*
Reader reads the card, keeping what it needs between reads. The zero value is
ready to use. Off Windows there is no D3DKMT and no counter set, so it reads
nothing.
*/
type Reader struct{}

// Read reports [errors.ErrUnsupported]: D3DKMT and the GPU Engine counters
// are Windows interfaces.
func (*Reader) Read(context.Context) (Card, error) {
	return Card{}, fmt.Errorf("reading the graphics card: %w", errors.ErrUnsupported)
}

// Close has nothing to release.
func (*Reader) Close() error { return nil }
