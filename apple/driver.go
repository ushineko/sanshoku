package apple

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/battery"
	"github.com/ushineko/sanshoku/bluez"
	"github.com/ushineko/sanshoku/l2cap"
)

// driverName is the driver's name, as Name returns it and the support table keys it.
const driverName = "apple"

// appleVendor is Apple's Bluetooth company identifier.
const appleVendor = 0x004C

/*
defaultTimeout bounds the dial, and then the exchange, each on its own.

A device that connects and never reports must not take the caller's poll loop
with it. The real exchange finishes well inside a second on the AirPods Pro
hayami measured (hayami AAPTimeout).
*/
const defaultTimeout = 6 * time.Second

// lister is how the driver asks what is connected: bluez.Devices, or a
// test's fixed list.
type lister func(context.Context) ([]bluez.Device, error)

// dialer opens the accessory-protocol channel: l2cap.Dial, or a test's fake.
type dialer func(ctx context.Context, addr [6]byte, psm uint16) (channel, error)

// dialL2CAP is l2cap.Dial as a dialer.
func dialL2CAP(ctx context.Context, addr [6]byte, psm uint16) (channel, error) {
	return l2cap.Dial(ctx, addr, psm)
}

/*
Driver reads Apple audio accessories (AirPods) over the accessory protocol on
an L2CAP channel, with per-ear and case levels, and falls back to the device's
BlueZ Battery1 level when the channel will not open or the device does not
report.

The zero value is ready to use. Timeout bounds the dial and then the exchange,
each; zero means six seconds.
*/
type Driver struct {
	Timeout time.Duration
}

// Name is "apple".
func (Driver) Name() string { return driverName }

/*
Find returns one candidate per connected BlueZ device that is Apple (Modalias
vendor 004C) and audio (an audio- icon or an audio sink profile). Nothing else
is ever dialled: opening an L2CAP channel to a keyboard is a thing to not do.
bluez.ErrNoBlueZ, which wraps sanshoku.ErrUnavailable, when BlueZ is not answering;
sanshoku.ErrAbsent when no such device is connected. Find opens nothing.
*/
func (d Driver) Find(ctx context.Context) ([]sanshoku.Candidate, error) {
	return find(ctx, d.timeout(), bluez.Devices, dialL2CAP)
}

func (d Driver) timeout() time.Duration {
	if d.Timeout <= 0 {
		return defaultTimeout
	}
	return d.Timeout
}

// find is Find over a lister and a dialer.
func find(ctx context.Context, timeout time.Duration, list lister, dial dialer) ([]sanshoku.Candidate, error) {
	devices, err := list(ctx)
	if err != nil {
		return nil, err
	}
	var out []sanshoku.Candidate
	for _, d := range devices {
		if !d.Apple || !d.Audio {
			continue
		}
		out = append(out, candidate(d, timeout, list, dial))
	}
	if len(out) == 0 {
		return nil, sanshoku.ErrAbsent
	}
	return out, nil
}

// candidate is one listed device as a candidate. Opening it dials nothing;
// the channel is opened for each reading and closed after it.
func candidate(d bluez.Device, timeout time.Duration, list lister, dial dialer) sanshoku.Candidate {
	id := sanshoku.Identity{
		Vendor: d.Vendor, Product: d.Product, Bus: sanshoku.BusBluetooth,
		Name: d.Name, Phys: d.Address, Path: d.Path,
	}
	return sanshoku.Candidate{
		Identity: id,
		Driver:   driverName,
		Open: func(context.Context) (sanshoku.Device, error) {
			return &device{id: id, kind: d.Kind, timeout: timeout, list: list, dial: dial}, nil
		},
	}
}

// device is one Apple audio accessory.
type device struct {
	id      sanshoku.Identity
	kind    battery.Kind
	timeout time.Duration
	list    lister
	dial    dialer

	mu     sync.Mutex
	closed bool
}

// Identity is the device's.
func (d *device) Identity() sanshoku.Identity { return d.id }

// Close forgets the device. No channel is held between readings.
func (d *device) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	return nil
}

/*
Batteries reads the device's battery, by whichever route it has.

The accessory protocol is tried first and its answer wins, because it is the
one with the cells in it: a device that reports both would otherwise be a
single percentage when it could say which ear is low. When it fails, BlueZ is
asked again and its Battery1 level, if it has one, is the reading. Better a
percentage than nothing.

A device that answers neither is no reading. Silence (ErrNoBatteryPacket) is
no error either; a refused channel is, because it is worth reporting once,
though it is what AirPods in a pocket do. A device BlueZ no longer lists is
sanshoku.ErrGone. No error names the device or its address.
*/
func (d *device) Batteries(ctx context.Context) ([]battery.Battery, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, fmt.Errorf("reading an apple accessory: %w", os.ErrClosed)
	}

	b, aapErr := d.readAccessory(ctx)
	if aapErr == nil {
		return []battery.Battery{b}, nil
	}
	if ctx.Err() != nil {
		return nil, fmt.Errorf("reading an apple accessory: %w", ctx.Err())
	}

	now, err := d.list(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading an apple accessory: %w", err)
	}
	for _, n := range now {
		if n.Path != d.id.Path {
			continue
		}
		if n.HasLevel {
			return []battery.Battery{{Name: n.Name, Level: n.Level, HasLevel: true, Kind: n.Kind}}, nil
		}
		if errors.Is(aapErr, ErrNoBatteryPacket) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading an apple accessory: %w", aapErr)
	}
	return nil, fmt.Errorf("reading an apple accessory: the device is no longer connected: %w", sanshoku.ErrGone)
}

// readAccessory runs the accessory protocol against the device: dial within
// the timeout, then the exchange within another.
func (d *device) readAccessory(ctx context.Context) (battery.Battery, error) {
	addr, err := l2cap.ParseAddress(d.id.Phys)
	if err != nil {
		return battery.Battery{}, err
	}

	dctx, cancel := context.WithTimeout(ctx, d.timeout)
	c, err := d.dial(dctx, addr, aapPSM)
	cancel()
	if err != nil {
		return battery.Battery{}, err
	}
	defer func() { _ = c.Close() }()

	xctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	cells, err := readAAP(xctx, c)
	if err != nil {
		return battery.Battery{}, err
	}
	b, err := batteryOf(d.id.Name, cells)
	if err != nil {
		return battery.Battery{}, err
	}
	b.Kind = d.kind
	return b, nil
}
