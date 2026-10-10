//go:build !windows

package bluez

import "context"

// windowsDevices is the Windows reader's place off Windows, where Devices
// never calls it.
func windowsDevices(context.Context) ([]Device, error) { return nil, ErrNoBlueZ }
