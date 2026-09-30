package bluez

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/battery"
)

// driverName is the driver's name, as Name returns it and the support table keys it.
const driverName = "bluez"

// lister is how the driver asks what is connected: Devices, or a test's fixed
// list, so no unit test touches the system bus.
type lister func(context.Context) ([]Device, error)

/*
Driver reads the battery BlueZ already has for a connected device, from
org.bluez.Battery1. Nothing is sent to the device; BlueZ is asked over the
system bus.

Every connected device with a Battery1 level is a candidate, whatever kind of
device it is, with no per-device code — except an Apple audio device, which
belongs to the apple driver: that one reads per-ear and case levels over the
accessory protocol and falls back to Battery1 itself, so listing it here as
well would report the same earbuds twice.

The zero value is ready to use.
*/
type Driver struct{}

// Name is "bluez".
func (Driver) Name() string { return driverName }

/*
Find returns one candidate per connected device that reports a Battery1 level
and is not Apple audio. ErrNoBlueZ, which wraps sanshoku.ErrUnavailable, when BlueZ
is not answering; sanshoku.ErrAbsent when nothing connected has a level. Find
opens nothing.
*/
func (Driver) Find(ctx context.Context) ([]sanshoku.Candidate, error) {
	return find(ctx, Devices)
}

// find is Find over a lister.
func find(ctx context.Context, list lister) ([]sanshoku.Candidate, error) {
	devices, err := list(ctx)
	if err != nil {
		return nil, err
	}
	var out []sanshoku.Candidate
	for _, d := range devices {
		if !d.HasLevel || (d.Apple && d.Audio) {
			continue
		}
		out = append(out, candidate(d, list))
	}
	if len(out) == 0 {
		return nil, sanshoku.ErrAbsent
	}
	return out, nil
}

// candidate is one listed device as a candidate. Opening it opens nothing: the
// device is BlueZ's and is asked through the bus on every read.
func candidate(d Device, list lister) sanshoku.Candidate {
	id := identity(d)
	return sanshoku.Candidate{
		Identity: id,
		Driver:   driverName,
		Open: func(context.Context) (sanshoku.Device, error) {
			return &device{id: id, list: list}, nil
		},
	}
}

/*
identity is what a listed device is called and where it is.

The address goes in Phys, which sanshoku.Identity keeps for matching and
never prints. Path is the D-Bus object path, which carries the address too; the
bench masks it.
*/
func identity(d Device) sanshoku.Identity {
	return sanshoku.Identity{
		Vendor: d.Vendor, Product: d.Product, Bus: sanshoku.BusBluetooth,
		Name: d.Name, Phys: d.Address, Path: d.Path,
	}
}

// device is one connected device's Battery1.
type device struct {
	id   sanshoku.Identity
	list lister

	mu     sync.Mutex
	closed bool
}

// Identity is the device's.
func (d *device) Identity() sanshoku.Identity { return d.id }

// Close forgets the device. There is nothing open to close.
func (d *device) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	return nil
}

/*
Batteries re-reads Battery1.Percentage for the device's object path.

A device that is still connected and has, for now, no level is no reading and
no error. A device that is no longer connected is sanshoku.ErrGone. The error
names neither the device nor its address: an alias may be a name somebody
chose, and the repository and its bug tracker are public.
*/
func (d *device) Batteries(ctx context.Context) ([]battery.Battery, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, fmt.Errorf("reading a bluetooth battery: %w", os.ErrClosed)
	}
	now, err := d.list(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading a bluetooth battery: %w", err)
	}
	for _, n := range now {
		if n.Path != d.id.Path {
			continue
		}
		if !n.HasLevel {
			return nil, nil
		}
		return []battery.Battery{{Name: n.Name, Level: n.Level, HasLevel: true, Kind: n.Kind}}, nil
	}
	return nil, fmt.Errorf("reading a bluetooth battery: the device is no longer connected: %w", sanshoku.ErrGone)
}
