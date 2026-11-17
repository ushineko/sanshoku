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

/*
cards is the allow-list: every card OpenRGB pairs with this controller (its
"v2") at this address, each named as OpenRGB names it, with the effects it
has (spec 033). Matched on all four IDs, because a subsystem ID repeats across
cards. The RTX 4090 Suprim Liquid X is tested; the rest are expected.
*/
var cards = map[card]cardInfo{
	{0x10DE, 0x2882, 0x1462, 0x5160}: {"MSI GeForce RTX 4060 Gaming X", familyV2},
	{0x10DE, 0x2882, 0x1462, 0x5164}: {"MSI GeForce RTX 4060 Gaming X NV Edition", familyV2},
	{0x10DE, 0x2803, 0x1462, 0x5152}: {"MSI GeForce RTX 4060 Ti Gaming X", familyV2},
	{0x10DE, 0x2805, 0x1462, 0x5172}: {"MSI GeForce RTX 4060 Ti 16GB Gaming X", familyV2},
	{0x10DE, 0x2805, 0x1462, 0x5171}: {"MSI GeForce RTX 4060 Ti 16GB Gaming X Slim White", familyNoStack},
	{0x10DE, 0x2786, 0x1462, 0x5136}: {"MSI GeForce RTX 4070 Gaming X Trio", familyV2},
	{0x10DE, 0x2786, 0x1462, 0x513F}: {"MSI GeForce RTX 4070 Gaming X Slim", familyV2},
	{0x10DE, 0x2786, 0x1462, 0xE131}: {"MSI GeForce RTX 4070 Gaming X Slim White", familyV2},
	{0x10DE, 0x2783, 0x1462, 0x513F}: {"MSI GeForce RTX 4070 SUPER Gaming X Slim", familyNoStack},
	{0x10DE, 0x2783, 0x1462, 0xE131}: {"MSI GeForce RTX 4070 SUPER Gaming X Slim White", familyV2},
	{0x10DE, 0x2783, 0x1462, 0xE134}: {"MSI GeForce RTX 4070 SUPER Gaming X Slim MLG", familyV2},
	{0x10DE, 0x2782, 0x1462, 0x5132}: {"MSI GeForce RTX 4070 Ti Gaming X Trio", familyV2},
	{0x10DE, 0x2782, 0x1462, 0x5139}: {"MSI GeForce RTX 4070 Ti Gaming X Trio White", familyV2},
	{0x10DE, 0x2782, 0x1462, 0xE132}: {"MSI GeForce RTX 4070 Ti Gaming X Slim White", familyV2},
	{0x10DE, 0x2782, 0x1462, 0x5133}: {"MSI GeForce RTX 4070 Ti Suprim X Trio", familyV2},
	{0x10DE, 0x2705, 0x1462, 0x5139}: {"MSI GeForce RTX 4070 Ti SUPER Gaming X Trio White", familyNoStack},
	{0x10DE, 0x2705, 0x1462, 0xE130}: {"MSI GeForce RTX 4070 Ti SUPER Gaming Slim", familyV2},
	{0x10DE, 0x2705, 0x1462, 0xE13D}: {"MSI GeForce RTX 4070 Ti SUPER Gaming X Slim", familyV2},
	{0x10DE, 0x2705, 0x1462, 0xE13E}: {"MSI GeForce RTX 4070 Ti SUPER Gaming White X Slim", familyV2},
	{0x10DE, 0x2705, 0x1462, 0xE132}: {"MSI GeForce RTX 4070 Ti SUPER Gaming White X Slim", familyV2},
	{0x10DE, 0x2704, 0x1462, 0x511A}: {"MSI GeForce RTX 4080 Gaming X Slim White", familyNoStack},
	{0x10DE, 0x2704, 0x1462, 0x5111}: {"MSI GeForce RTX 4080 Gaming X Trio", familyV2},
	{0x10DE, 0x2704, 0x1462, 0x5115}: {"MSI GeForce RTX 4080 Gaming X Trio White", familyV2},
	{0x10DE, 0x2704, 0x1462, 0x5110}: {"MSI GeForce RTX 4080 Suprim X", familyV2},
	{0x10DE, 0x2702, 0x1462, 0x5117}: {"MSI GeForce RTX 4080 SUPER Gaming X Slim", familyV2},
	{0x10DE, 0x2702, 0x1462, 0x511A}: {"MSI GeForce RTX 4080 SUPER Gaming X Slim White", familyNoStack},
	{0x10DE, 0x2702, 0x1462, 0x5110}: {"MSI GeForce RTX 4080 SUPER Suprim X", familyV2},
	{0x10DE, 0x2702, 0x1462, 0x5111}: {"MSI GeForce RTX 4080 SUPER Gaming X Trio", familyNoStack},
	{0x10DE, 0x2684, 0x1462, 0x5103}: {"MSI GeForce RTX 4090 Gaming X Trio", familyV2},
	{0x10DE, 0x2684, 0x1462, 0x510B}: {"MSI GeForce RTX 4090 Gaming X Slim", familyV2},
	{0x10DE, 0x2684, 0x1462, 0x5104}: {"MSI GeForce RTX 4090 Suprim Liquid X", familyV2},
	{0x10DE, 0x2684, 0x1462, 0x5102}: {"MSI GeForce RTX 4090 Suprim X", familyV2},
	{0x10DE, 0x2684, 0x1462, 0x5105}: {"MSI GeForce RTX 4090 Suprim X Classic", familyV2},
	{0x10DE, 0x2D04, 0x1462, 0x5354}: {"MSI GeForce RTX 5060 Ti Gaming OC", familyRTX50},
	{0x10DE, 0x2D05, 0x1462, 0x5372}: {"MSI GeForce RTX 5060 8G Gaming Trio OC", familyRTX50},
	{0x10DE, 0x2D04, 0x1462, 0x5353}: {"MSI GeForce RTX 5060 Ti 16G Gaming Trio OC", familyRTX50},
	{0x10DE, 0x2F04, 0x1462, 0x5322}: {"MSI GeForce RTX 5070 Gaming Trio", familyRTX50},
	{0x10DE, 0x2C05, 0x1462, 0x5315}: {"MSI GeForce RTX 5070 Ti Gaming Trio OC Plus", familyRTX50},
	{0x10DE, 0x2C05, 0x1462, 0x5314}: {"MSI GeForce RTX 5070 Ti VANGUARD SOC", familyRTX50},
	{0x10DE, 0x2C02, 0x1462, 0x5315}: {"MSI GeForce RTX 5080 Gaming Trio OC", familyRTX50},
	{0x10DE, 0x2C02, 0x1462, 0x5314}: {"MSI GeForce RTX 5080 VANGUARD OC SOC", familyRTX50},
	{0x10DE, 0x2C02, 0x1462, 0x5313}: {"MSI GeForce RTX 5080 SUPRIM SOC", familyRTX50},
	{0x10DE, 0x2C02, 0x1462, 0x5312}: {"MSI GeForce RTX 5080 SUPRIM LIQUID SOC", familyRTX50},
	{0x10DE, 0x2B85, 0x1462, 0x5303}: {"MSI GeForce RTX 5090 Gaming Trio OC", familyRTX50},
	{0x10DE, 0x2B85, 0x1462, 0x5302}: {"MSI GeForce RTX 5090 VANGUARD SOC", familyRTX50},
	{0x10DE, 0x2B85, 0x1462, 0x5301}: {"MSI GeForce RTX 5090 SUPRIM SOC", familyRTX50},
	{0x10DE, 0x2B85, 0x1462, 0x5300}: {"MSI GeForce RTX 5090 SUPRIM LIQUID SOC", familyRTX50},
}

// cardInfo is a card's name and which of the controller's effect tables it
// has.
type cardInfo struct {
	name   string
	family family
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
		info, ok := cards[card{a.Vendor, a.Device, a.SubVendor, a.SubDevice}]
		if !ok || !strings.HasPrefix(a.Name+" ", adapterName) {
			continue
		}
		out = append(out, candidate(a, info))
	}
	if len(out) == 0 {
		return nil, sanshoku.ErrAbsent
	}
	return out, nil
}

func candidate(a i2c.Adapter, info cardInfo) sanshoku.Candidate {
	id := sanshoku.Identity{
		Vendor: a.SubVendor, Product: a.SubDevice, Bus: sanshoku.BusI2C,
		Name: info.name, Phys: fmt.Sprintf("%s, address 0x%02x", a.Name, address), Path: a.Path,
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
			return &device{id: id, bus: bus, running: -1, fx: effectsFor(info.family)}, nil
		},
	}
}

// device is one open lighting controller.
type device struct {
	id sanshoku.Identity

	mu  sync.Mutex
	bus i2c.Bus
	// fx is this card's effects, by its family.
	fx []effect
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
