package corsairdram

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/i2c"
)

// driverName is the driver's name, as Name returns it and the support table keys it.
const driverName = "corsairdram"

// Registers, from OpenRGB 1.1's Corsair DRAM controller.
const (
	regIdentify1   = 0x43 // reads 1A, 1B or 1C on a module
	regIdentify2   = 0x44 // reads 01, 03 or 04 on a module
	regDeviceInfo  = 0x61 // selects the device-information block
	regBinaryStart = 0x21 // starts reading or writing the selected block
	regBinaryData  = 0x40 // reads the block's next byte
	regChecksum    = 0x42 // reads the block's CRC-8
	regFrame1      = 0x31 // a direct frame's first 32 bytes
	regFrame2      = 0x32 // the rest

	infoLen = 32
	// directProtocol is the first protocol version that takes direct frames.
	directProtocol = 4
)

// family is which of Corsair's two memory protocols a module speaks.
type family int

const (
	// familyDRAM is the device-information block and direct frames: the
	// Vengeance RGB Pro, Dominator and later (spec 042).
	familyDRAM family = iota
	// familyVengeance is the original Vengeance RGB: one colour, written
	// register by register (spec 046).
	familyVengeance
)

// The original Vengeance RGB's registers (OpenRGB 1.1's
// CorsairVengeanceController).
const (
	regVengeanceID    = 0xA0 // A0 to AF all read BA on a module
	vengeanceID       = 0xBA
	regVengeanceFade  = 0xA4 // the fade time, 0 for a still colour
	regVengeanceMode  = 0xA6 // 0 is a single colour
	regVengeanceRed   = 0xB0
	regVengeanceGreen = 0xB1
	regVengeanceBlue  = 0xB2
)

/*
writable are the only registers this driver writes, per family (spec 042
R1.5, spec 046): the device-information read and the direct frame, or the
original Vengeance's colour and its single-colour mode.
*/
var writable = map[family]map[byte]bool{
	familyDRAM: {regDeviceInfo: true, regBinaryStart: true, regFrame1: true, regFrame2: true},
	familyVengeance: {
		regVengeanceFade: true, regVengeanceMode: true,
		regVengeanceRed: true, regVengeanceGreen: true, regVengeanceBlue: true,
	},
}

// addresses are where a module answers: 18 to 1F on DDR5, 58 to 5F on DDR4,
// as OpenRGB's detector tries them.
var addresses = func() []uint8 {
	var out []uint8
	for a := uint8(0x58); a <= 0x5F; a++ {
		out = append(out, a)
	}
	for a := uint8(0x18); a <= 0x1F; a++ {
		out = append(out, a)
	}
	return out
}()

// pciID is a PCI vendor and device.
type pciID struct{ vendor, device uint16 }

/*
memoryBuses are the SMBus controllers OpenRGB 1.1 looks for memory on
(IF_DRAM_SMBUS in pci_ids.h). Another adapter is never probed.
*/
var memoryBuses = map[pciID]string{
	{0x1022, 0x790B}: "AMD FCH",
	{0x8086, 0x3A30}: "Intel ICH10",
	{0x8086, 0xA123}: "Intel Sunrise Point-H",
	{0x8086, 0x2085}: "Intel Skylake-X PCU",
	{0x8086, 0xA2A3}: "Intel Kaby Lake-H",
	{0x8086, 0xA323}: "Intel Cannon Lake-H",
	{0x8086, 0x06A3}: "Intel Comet Lake-H",
	{0x8086, 0xA3A3}: "Intel Comet Lake-V",
	{0x8086, 0x43A3}: "Intel Tiger Lake",
	{0x8086, 0x7AA3}: "Intel Alder Lake",
	{0x8086, 0x7A23}: "Intel Raptor Lake",
	{0x8086, 0x7F23}: "Intel Arrow Lake",
}

// product is a module family: its name, its LEDs, and whether a frame lists
// them last first.
type product struct {
	name    string
	leds    int
	reverse bool
}

/*
products are the module families OpenRGB 1.1 knows, by the product ID in the
device-information block (CorsairDRAMDevices.cpp; facts only). The Dominator
Platinum RGB DDR5 is tested; the rest are expected (spec 042).
*/
var products = map[uint16]product{
	0x0100: {"Corsair Vengeance RGB Pro DDR4", 10, false},
	0x0101: {"Corsair Vengeance RGB Pro DDR4", 10, false},
	0x0200: {"Corsair Dominator Platinum RGB DDR4", 12, true},
	0x0201: {"Corsair Dominator Platinum RGB DDR4", 12, true},
	0x0300: {"Corsair Vengeance RGB Pro SL DDR4", 10, false},
	0x0301: {"Corsair Vengeance RGB Pro SL DDR4", 10, false},
	0x0400: {"Corsair Vengeance RGB RS DDR4", 6, false},
	0x0401: {"Corsair Vengeance RGB RS DDR4", 6, false},
	0x0600: {"Corsair Dominator Platinum RGB DDR5", 12, true},
	0x0601: {"Corsair Dominator Platinum RGB DDR5", 12, true},
	0x0800: {"Corsair Dominator Titanium RGB DDR5", 12, true},
	0x0801: {"Corsair Dominator Titanium RGB DDR5", 12, true},
	0x0810: {"Corsair Dominator Titanium RGB DDR5", 12, true},
	0x0811: {"Corsair Dominator Titanium RGB DDR5", 12, true},
	0x0700: {"Corsair Vengeance RGB DDR5", 10, false},
	0x0701: {"Corsair Vengeance RGB DDR5", 10, false},
	0x0900: {"Corsair Vengeance RGB DDR5", 10, false},
	0x0901: {"Corsair Vengeance RGB DDR5", 10, false},
	0x0910: {"Corsair Vengeance RGB DDR5", 10, false},
	0x0911: {"Corsair Vengeance RGB DDR5", 10, false},
	0x0A00: {"Corsair Vengeance Shugo Series DDR5", 10, false},
	0x0A01: {"Corsair Vengeance Shugo Series DDR5", 10, false},
	0x0A10: {"Corsair Vengeance Shugo Series DDR5", 10, false},
	0x0A11: {"Corsair Vengeance Shugo Series DDR5", 10, false},
	0x0B00: {"Corsair Vengeance RGB RS DDR5", 6, false},
	0x0B01: {"Corsair Vengeance RGB RS DDR5", 6, false},
}

/*
Driver finds and opens Corsair RGB memory modules.

The zero value is ready to use.
*/
type Driver struct{}

// Name is "corsairdram".
func (Driver) Name() string { return driverName }

// Seams for tests: where the adapters are listed, how one is opened, and the
// pause between probing two addresses (OpenRGB's 10 ms).
var (
	listAdapters = i2c.Adapters
	openBus      = i2c.Open
	probePause   = 10 * time.Millisecond
)

/*
probed is what probing found, per adapter, for the life of the process.
Memory modules do not come and go while a machine runs, and a consumer that
looks for devices every few seconds should not read the memory bus each
time.
*/
var probed = struct {
	sync.Mutex
	found map[string][]sighting
}{found: map[string][]sighting{}}

// sighting is a module probing found: its address and family.
type sighting struct {
	addr   uint8
	family family
}

/*
Find returns one candidate per module on a memory bus OpenRGB names.

It is the one driver that reads to find: a module has no ID to match without
asking it. Each address is opened alone, which the kernel refuses for an
address one of its drivers owns (a DDR5 module's SPD hub), and asked its two
identification registers; only reads. Each adapter is probed once per
process. ErrAbsent when there is none.
*/
func (Driver) Find(context.Context) ([]sanshoku.Candidate, error) {
	adapters, err := listAdapters()
	if err != nil {
		return nil, err //nolint:wrapcheck // i2c names the directory
	}
	var out []sanshoku.Candidate
	for _, a := range adapters {
		if _, ok := memoryBuses[pciID{a.Vendor, a.Device}]; !ok {
			continue
		}
		for _, m := range probe(a.Path) {
			out = append(out, candidate(a, m))
		}
	}
	if len(out) == 0 {
		return nil, sanshoku.ErrAbsent
	}
	return out, nil
}

// probe is the modules on an adapter, probing the adapter the first time it
// is asked.
func probe(path string) []sighting {
	probed.Lock()
	defer probed.Unlock()
	if found, ok := probed.found[path]; ok {
		return found
	}
	var found []sighting
	for i, addr := range addresses {
		if i > 0 {
			time.Sleep(probePause)
		}
		if f, ok := identify(path, addr); ok {
			found = append(found, sighting{addr, f})
		}
	}
	probed.found[path] = found
	return found
}

/*
identify is whether a Corsair module answers at an address, and which family:
the newer modules by their two identification registers, the original
Vengeance RGB by sixteen registers that all read BA (OpenRGB's detectors).
Only reads.
*/
func identify(path string, addr uint8) (family, bool) {
	bus, err := openBus(path, addr)
	if err != nil {
		return 0, false
	}
	defer func() { _ = bus.Close() }()
	v1, err1 := bus.ReadByteData(regIdentify1)
	v2, err2 := bus.ReadByteData(regIdentify2)
	if err1 == nil && err2 == nil && (v1 == 0x1A || v1 == 0x1B || v1 == 0x1C) && (v2 == 0x01 || v2 == 0x03 || v2 == 0x04) {
		return familyDRAM, true
	}
	if err1 != nil && err2 != nil {
		return 0, false // nothing answers here
	}
	for r := byte(regVengeanceID); r < regVengeanceID+16; r++ {
		if v, err := bus.ReadByteData(r); err != nil || v != vengeanceID {
			return 0, false
		}
	}
	return familyVengeance, true
}

func candidate(a i2c.Adapter, m sighting) sanshoku.Candidate {
	addr := m.addr
	id := sanshoku.Identity{
		Vendor: 0x1B1C, Bus: sanshoku.BusI2C,
		Name: fmt.Sprintf("Corsair memory (0x%02X)", addr),
		Phys: fmt.Sprintf("%s, address 0x%02x", a.Name, addr), Path: a.Path,
	}
	return sanshoku.Candidate{
		Identity: id,
		Driver:   driverName,
		Key:      fmt.Sprintf("%s@%02x", a.Path, addr),
		Open: func(context.Context) (sanshoku.Device, error) {
			bus, err := openBus(a.Path, addr)
			if err != nil {
				return nil, err //nolint:wrapcheck // i2c names the node
			}
			d := &device{id: id, bus: bus, addr: addr, family: m.family}
			if err := d.start(); err != nil {
				_ = bus.Close()
				return nil, err
			}
			return d, nil
		},
	}
}

// device is one open module.
type device struct {
	id     sanshoku.Identity
	addr   uint8
	family family

	mu  sync.Mutex
	bus i2c.Bus
	// what the device-information block says.
	product  product
	firmware string
	protocol byte
	// held is the frame, and sent whether the module has it.
	held []colour
	sent bool
}

type colour struct{ r, g, b uint8 }

func (d *device) Identity() sanshoku.Identity { return d.id }

// errNoAnswer is a module whose device information did not read back whole.
var errNoAnswer = errors.New("the module's device information did not read back")

/*
start reads the device-information block and checks its CRC, then names the
module from its product ID, as OpenRGB names it, with its address: "Corsair
Dominator Platinum RGB DDR5 (0x18)". A product not known, or one too old for
direct frames, is refused. The caller has not shared the device.
*/
func (d *device) start() error {
	if d.family == familyVengeance {
		// The original Vengeance RGB has no information to read: one LED,
		// named as OpenRGB names it.
		d.product = product{name: "Corsair Vengeance RGB", leds: 1}
		d.id.Name = fmt.Sprintf("%s (0x%02X)", d.product.name, d.addr)
		d.held = make([]colour, 1)
		return nil
	}
	if err := d.write(regDeviceInfo, 0x00); err != nil {
		return err
	}
	if err := d.write(regBinaryStart, 0x00); err != nil {
		return err
	}
	info := make([]byte, infoLen)
	for i := range info {
		b, err := d.bus.ReadByteData(regBinaryData)
		if err != nil {
			return fmt.Errorf("%s: %w: %w", d.id, errNoAnswer, err)
		}
		info[i] = b
	}
	crc, err := d.bus.ReadByteData(regChecksum)
	if err != nil {
		return fmt.Errorf("%s: %w: %w", d.id, errNoAnswer, err)
	}
	if crc != crc8(info) {
		return fmt.Errorf("%s: %w: CRC %02x, the module says %02x", d.id, errNoAnswer, crc8(info), crc)
	}
	pid := uint16(info[2]) | uint16(info[3])<<8
	p, ok := products[pid]
	if !ok {
		return fmt.Errorf("%s: product %04x is not one this driver knows: %w", d.id, pid, sanshoku.ErrUnsupported)
	}
	d.protocol = info[28]
	if d.protocol < directProtocol {
		return fmt.Errorf("%s: protocol %d takes no direct frames: %w", d.id, d.protocol, sanshoku.ErrUnsupported)
	}
	d.product = p
	d.firmware = fmt.Sprintf("%d.%d.%d", info[9], info[8], uint16(info[10])|uint16(info[11])<<8)
	d.id.Name = fmt.Sprintf("%s (0x%02X)", p.name, d.addr)
	d.held = make([]colour, p.leds)
	return nil
}

// Close closes the adapter and sends nothing: the module keeps its frame.
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
write writes one byte to a register on the allow-list, and refuses any other
(spec 042 R1.5). The caller holds mu, or has not shared the device.
*/
func (d *device) write(register, value byte) error {
	if d.bus == nil {
		return fmt.Errorf("%s: %w", d.id.Path, os.ErrClosed)
	}
	if !writable[d.family][register] {
		return fmt.Errorf("%s: register %02x is not one this driver writes", d.id, register)
	}
	if err := d.bus.WriteByteData(register, value); err != nil {
		return fmt.Errorf("%s: %w", d.id, err)
	}
	return nil
}

// block writes an SMBus block to a register on the allow-list. The caller
// holds mu.
func (d *device) block(register byte, data []byte) error {
	if d.bus == nil {
		return fmt.Errorf("%s: %w", d.id.Path, os.ErrClosed)
	}
	if !writable[d.family][register] {
		return fmt.Errorf("%s: register %02x is not one this driver writes", d.id, register)
	}
	if err := d.bus.WriteSMBusBlock(register, data); err != nil {
		return fmt.Errorf("%s: %w", d.id, err)
	}
	return nil
}

// crc8 is CRC-8 with polynomial 07, no reflection, starting at 0: what the
// module computes over a block (OpenRGB's CRC_8).
func crc8(data []byte) byte {
	var crc byte
	for _, b := range data {
		crc ^= b
		for range 8 {
			if crc&0x80 != 0 {
				crc = crc<<1 ^ 0x07
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
