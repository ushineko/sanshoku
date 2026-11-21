//go:build !linux && !windows

package i2c

import (
	"errors"
	"fmt"
)

// ErrNoI2C is a system this package has no I2C for.
var ErrNoI2C = errors.New("no I2C on this system")

// Adapters finds none: only Linux has i2c-dev.
func Adapters() ([]Adapter, error) { return nil, nil }

// Open is ErrNoI2C.
func Open(path string, _ uint8) (Bus, error) {
	return nil, fmt.Errorf("%s: %w", path, ErrNoI2C)
}
