/*
Package i2c is a device on an I2C (SMBus) adapter: the bus a graphics card's
lighting controller sits on (spec 029).

Linux only: an adapter is /dev/i2c-N, through the kernel's i2c-dev module. On
other systems Adapters finds none.
*/
package i2c

import (
	"errors"
	"fmt"
)

/*
Adapter is one I2C adapter, and the PCI device it belongs to where it belongs
to one: a graphics card's adapters are its.
*/
type Adapter struct {
	// Path is the adapter's node, /dev/i2c-N.
	Path string
	// Name is the adapter's own name ("NVIDIA i2c adapter 1 at 1:00.0").
	Name string
	// The PCI device's IDs, zero where the adapter is not a PCI device's.
	Vendor, Device, SubVendor, SubDevice uint16
}

// Bus is a device at one address on an adapter.
type Bus interface {
	// WriteByteData writes one byte to a register (SMBus write byte data).
	WriteByteData(register, value byte) error
	// WriteBlockData writes up to 32 bytes from a register on (I2C block
	// write).
	WriteBlockData(register byte, data []byte) error
	Close() error
}

// ErrBlock is a block longer than an I2C block write takes.
var ErrBlock = errors.New("an I2C block is at most 32 bytes")

func checkBlock(data []byte) error {
	if len(data) > 32 {
		return fmt.Errorf("%d bytes: %w", len(data), ErrBlock)
	}
	return nil
}
