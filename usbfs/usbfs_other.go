//go:build !linux

package usbfs

import (
	"context"
	"errors"
	"fmt"
)

// Interface is a claimed interface on a USB device. Off Linux there is no
// usbfs to claim one through, so none is ever returned.
type Interface struct{}

// Open reports [errors.ErrUnsupported]: usbfs is a Linux interface.
func Open(path string, iface int) (*Interface, error) {
	return nil, fmt.Errorf("claiming interface %d of %s: %w", iface, path, errors.ErrUnsupported)
}

// Bulk reports [errors.ErrUnsupported].
func (u *Interface) Bulk(context.Context, byte, []byte) error {
	return fmt.Errorf("usb bulk transfer: %w", errors.ErrUnsupported)
}

// Close has nothing to release.
func (u *Interface) Close() error { return nil }
