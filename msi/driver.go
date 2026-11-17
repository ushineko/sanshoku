package msi

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/i2c"
)

// driverName is the driver's name, as Name returns it and the support table keys it.
const driverName = "msi"

// address is the lighting controller's address on the card's adapter.
const address = 0x68

// adapterName is the start of the name of the adapter the controller is on:
// NVIDIA's first (OpenRGB's port 1).
const adapterName = "NVIDIA i2c adapter 1 "

// card is a graphics card on the allow-list, by its PCI IDs.
type card struct{ vendor, device, subVendor, subDevice uint16 }

// cards is the allow-list, and each card's name, as OpenRGB names it.
var cards = map[card]string{
	{0x10DE, 0x2684, 0x1462, 0x5104}: "MSI GeForce RTX 4090 Suprim Liquid X",
}

// settle is the pause after every register write (OpenRGB's 20 ms).
var settle = 20 * time.Millisecond

/*
Driver finds and opens MSI graphics cards' lighting controllers.

The zero value is ready to use.
*/
type Driver struct{}

// Name is "msi".
func (Driver) Name() string { return driverName }

// Seams for tests: where the adapters are listed, and how one is opened.
var (
	listAdapters = i2c.Adapters
	openBus      = i2c.Open
)

/*
Find returns one candidate per NVIDIA adapter 1 on a card the allow-list
names. Matched by the card's PCI IDs; nothing is read from the controller,
which answers no question. ErrAbsent when there is none.
*/
func (Driver) Find(context.Context) ([]sanshoku.Candidate, error) {
	adapters, err := listAdapters()
	if err != nil {
		return nil, err //nolint:wrapcheck // i2c names the directory
	}
	var out []sanshoku.Candidate
	for _, a := range adapters {
		name, ok := cards[card{a.Vendor, a.Device, a.SubVendor, a.SubDevice}]
		if !ok || !strings.HasPrefix(a.Name+" ", adapterName) {
			continue
		}
		out = append(out, candidate(a, name))
	}
	if len(out) == 0 {
		return nil, sanshoku.ErrAbsent
	}
	return out, nil
}

func candidate(a i2c.Adapter, name string) sanshoku.Candidate {
	id := sanshoku.Identity{
		Vendor: a.SubVendor, Product: a.SubDevice, Bus: sanshoku.BusI2C,
		Name: name, Phys: fmt.Sprintf("%s, address 0x%02x", a.Name, address), Path: a.Path,
	}
	return sanshoku.Candidate{
		Identity: id,
		Driver:   driverName,
		Key:      a.Path,
		Open: func(context.Context) (sanshoku.Device, error) {
			bus, err := openBus(a.Path, address)
			if err != nil {
				return nil, err //nolint:wrapcheck // i2c names the node
			}
			return &device{id: id, bus: bus, running: -1}, nil
		},
	}
}

// device is one open lighting controller.
type device struct {
	id sanshoku.Identity

	mu  sync.Mutex
	bus i2c.Bus
	// held is the static colour, and shown whether the card shows it.
	held  colour
	shown bool
	// running is the effect this handle set, -1 for none; colour the colour
	// it was last given.
	running int
	colour  *colour
	// given are the colours SetEffectWith was given, first to last, and
	// speed the speed register's value; nil and 0 are OpenRGB's (spec 032).
	given []colour
	speed byte
}

func (d *device) Identity() sanshoku.Identity { return d.id }

// Close closes the adapter and sends nothing: the card keeps its colour.
func (d *device) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.bus == nil {
		return nil
	}
	err := d.bus.Close()
	d.bus = nil
	return err //nolint:wrapcheck // the file names itself
}

// write writes one register and waits, as the controller needs. The caller
// holds mu.
func (d *device) write(register, value byte) error {
	if d.bus == nil {
		return fmt.Errorf("%s: %w", d.id.Path, os.ErrClosed)
	}
	if err := d.bus.WriteByteData(register, value); err != nil {
		return fmt.Errorf("%s: %w", d.id, err)
	}
	time.Sleep(settle)
	return nil
}

// block writes a colour block in B, G, R order and waits. The caller holds mu.
func (d *device) block(register byte, c colour) error {
	if d.bus == nil {
		return fmt.Errorf("%s: %w", d.id.Path, os.ErrClosed)
	}
	if err := d.bus.WriteBlockData(register, []byte{c.b, c.g, c.r}); err != nil {
		return fmt.Errorf("%s: %w", d.id, err)
	}
	time.Sleep(settle)
	return nil
}
