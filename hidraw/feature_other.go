//go:build !linux

package hidraw

import (
	"context"
	"errors"
	"fmt"
)

// SetFeature reports [errors.ErrUnsupported]: the feature-report ioctls are
// hidraw's, and hidraw is a Linux interface.
func (h *Handle) SetFeature(context.Context, []byte) error {
	return fmt.Errorf("hidraw feature report: %w", errors.ErrUnsupported)
}

// GetFeature reports [errors.ErrUnsupported], as SetFeature does.
func (h *Handle) GetFeature(context.Context, []byte) error {
	return fmt.Errorf("hidraw feature report: %w", errors.ErrUnsupported)
}
