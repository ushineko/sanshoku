package evga

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/i2c"
)

// driverName is the driver's name, as Name returns it and the support table keys it.
const driverName = "evga"

// address is the lighting controller's address on the card's adapter
// (OpenRGB's detector, for every card it lists).
const address = 0x2D

// adapterName is the start of the name of the adapter the controller is on:
// NVIDIA's first (OpenRGB's port 1).
const adapterName = "NVIDIA i2c adapter 1 "

// card is a graphics card on the allow-list, by its PCI IDs.
type card struct{ vendor, device, subVendor, subDevice uint16 }

// The controller's registers, from OpenRGB 1.1's EVGA GPU v3 controller.
const (
	regFirmware = 0xB1
	regUnlock   = 0xB2
	regModes    = 0xC0
	regStatic   = 0xC1 // + the zone
	regSave     = 0x90 // never written: it stores the lighting in the card

	modeStatic = 0x01
	// keep is the byte that leaves a field of the modes register as it is.
	keep = 0xFF
)

// unlock is what the card needs written before it takes a command, as
// OpenRGB sends it (its comment: from a capture of EVGA's Precision X1).
var unlock = []byte{0x04, 0xC6, 0xEB, 0xEA, 0x15}

// zoneNames are the controller's four zones, as OpenRGB names them.
var zoneNames = [4]string{"Front Logo", "End plate Logo", "Back Logo", "Addressable Header"}

/*
Driver finds and opens EVGA Ampere graphics cards' lighting controllers.

The zero value is ready to use.
*/
type Driver struct{}

// Name is "evga".
func (Driver) Name() string { return driverName }

// Seams for tests: where the adapters are listed, and how one is opened.
var (
	listAdapters = i2c.Adapters
	openBus      = i2c.Open
)

/*
Find returns one candidate per NVIDIA adapter 1 on a card the allow-list
names. Matched by the card's PCI IDs; nothing is opened. ErrAbsent when there
is none.
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
			d := &device{id: id, bus: bus}
			if err := d.start(); err != nil {
				_ = bus.Close()
				return nil, err
			}
			return d, nil
		},
	}
}

// device is one open lighting controller.
type device struct {
	id sanshoku.Identity

	mu  sync.Mutex
	bus i2c.Bus
	// firmware is the controller's version ("1.00.12").
	firmware string
	// zones are the zones the card has lights on, in order.
	zones []zone
	// found is the modes register as it was at open, and whether this
	// handle has changed it: what Release puts back.
	found   []byte
	claimed bool
}

// zone is one of the controller's zones: its index, what it shows and
// whether the card has it, and its static register as it was at open.
type zone struct {
	index int
	held  colour
	sent  bool
	found []byte
}

type colour struct{ r, g, b uint8 }

func (d *device) Identity() sanshoku.Identity { return d.id }

// errNoAnswer is a controller that did not answer a read as expected.
var errNoAnswer = errors.New("the controller did not answer")

/*
start reads the firmware version, which is how OpenRGB tells the controller
is there, then the modes register (each zone's mode and LED count) and each
lit zone's static colour, so that Release can put them back. Nothing is
written. The caller has not shared the device.
*/
func (d *device) start() error {
	fw, err := d.bus.ReadBlockData(regFirmware, 6)
	if err != nil || len(fw) != 6 {
		return fmt.Errorf("%s: its firmware version: %w", d.id, errors.Join(errNoAnswer, err))
	}
	d.firmware = fmt.Sprintf("1.%02d.%02d", fw[4], fw[5])
	modes, err := d.bus.ReadBlockData(regModes, 10)
	if err != nil || len(modes) != 10 {
		return fmt.Errorf("%s: its zones: %w", d.id, errors.Join(errNoAnswer, err))
	}
	d.found = modes
	for i := range zoneNames {
		if modes[5+i] == 0 {
			continue
		}
		static, err := d.bus.ReadBlockData(regStatic+byte(i), 5) //nolint:gosec // i < 4
		if err != nil || len(static) != 5 {
			return fmt.Errorf("%s: zone %d's colour: %w", d.id, i+1, errors.Join(errNoAnswer, err))
		}
		d.zones = append(d.zones, zone{index: i, found: static})
	}
	if len(d.zones) == 0 {
		return fmt.Errorf("%s: no zone has lights: %w", d.id, sanshoku.ErrUnsupported)
	}
	return nil
}

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

/*
write writes one block. It refuses the save register, which stores the
lighting in the card (spec 041 R1.4). The caller holds mu.
*/
func (d *device) write(register byte, data []byte) error {
	if d.bus == nil {
		return fmt.Errorf("%s: %w", d.id.Path, os.ErrClosed)
	}
	if register == regSave {
		return fmt.Errorf("%s: register %02x stores the lighting in the card, and is never written", d.id, register)
	}
	if err := d.bus.WriteBlockData(register, data); err != nil {
		return fmt.Errorf("%s: %w", d.id, err)
	}
	return nil
}
