//go:build !linux

package l2cap

import (
	"context"
	"errors"
	"fmt"
)

// Conn is an open L2CAP channel. Off Linux there is no socket family to open
// one with, so none is ever returned.
type Conn struct{}

// Dial reports [errors.ErrUnsupported]: L2CAP sockets are a Linux interface.
func Dial(_ context.Context, _ [6]byte, psm uint16) (*Conn, error) {
	return nil, fmt.Errorf("dialling l2cap port %#04x: %w", psm, errors.ErrUnsupported)
}

// Send reports [errors.ErrUnsupported].
func (c *Conn) Send([]byte) error {
	return fmt.Errorf("sending on an l2cap channel: %w", errors.ErrUnsupported)
}

// Receive reports [errors.ErrUnsupported].
func (c *Conn) Receive(context.Context) ([]byte, error) {
	return nil, fmt.Errorf("waiting on an l2cap channel: %w", errors.ErrUnsupported)
}

// Close has nothing to close.
func (c *Conn) Close() error { return nil }
