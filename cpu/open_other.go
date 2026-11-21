//go:build !windows

package cpu

import (
	"errors"

	"github.com/ushineko/sanshoku/pawnio"
)

var errClosed = errors.New("the reader is closed")

// Open is pawnio.ErrNoPawnIO: on Linux package hwmon reads the processor.
func Open() (*Reader, error) { return nil, pawnio.ErrNoPawnIO }
