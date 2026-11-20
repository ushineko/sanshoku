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

// writable are the only registers this driver writes (spec 042 R1.5): the
// device-information read and the direct frame.
var writable = map[byte]bool{regDeviceInfo: true, regBinaryStart: true, regFrame1: true, regFrame2: true}

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
	found map[string][]uint8
}{found: map[string][]uint8{}}

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
		for _, addr := range probe(a.Path) {
			out = append(out, candidate(a, addr))
		}
	}
	if len(out) == 0 {
		return nil, sanshoku.ErrAbsent
	}
	return out, nil
}

// probe is the addresses on an adapter where a module answered, probing the
// adapter the first time it is asked.
func probe(path string) []uint8 {
	probed.Lock()
	defer probed.Unlock()
	if found, ok := probed.found[path]; ok {
		return found
	}
	var found []uint8
	for i, addr := range addresses {
		if i > 0 {
			time.Sleep(probePause)
		}
		if isModule(path, addr) {
			found = append(found, addr)
		}
	}
	probed.found[path] = found
	return found
}

// isModule is whether a Corsair module answers at an address, by its two
// identification registers.
func isModule(path string, addr uint8) bool {
	bus, err := openBus(path, addr)
	if err != nil {
		return false
	}
	defer func() { _ = bus.Close() }()
	v, err := bus.ReadByteData(regIdentify1)
	if err != nil || (v != 0x1A && v != 0x1B && v != 0x1C) {
		return false
	}
	v, err = bus.ReadByteData(regIdentify2)
	return err == nil && (v == 0x01 || v == 0x03 || v == 0x04)
}

func candidate(a i2c.Adapter, addr uint8) sanshoku.Candidate {
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
			d := &device{id: id, bus: bus, addr: addr}
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
	id   sanshoku.Identity
	addr uint8

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
	if !writable[register] {
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
	if !writable[register] {
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
