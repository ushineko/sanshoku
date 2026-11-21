package corsairdram

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/i2c"
	"github.com/ushineko/sanshoku/lighting"
)

/*
module is a Corsair module in memory, at one address: it answers the two
identification registers, streams its device-information block after 61 and
21 are written, as OpenRGB reads it, and records every write. Its block is
made up in the module's layout.
*/
type module struct {
	// vengeance is an original Vengeance RGB: A0 to AF read BA, and the
	// newer family's registers read 00.
	vengeance bool
	id1, id2  byte
	info      [infoLen]byte
	badCRC    bool
	at        int
	writes    []string
}

func newModule(pid uint16, protocol byte) *module {
	m := &module{id1: 0x1C, id2: 0x04}
	m.info[0], m.info[1] = 0x1C, 0x1B
	m.info[2], m.info[3] = byte(pid), byte(pid>>8)
	m.info[8], m.info[9], m.info[10], m.info[11] = 2, 1, 0x34, 0x12
	m.info[28] = protocol
	return m
}

// conn is one open handle to a module; a test sees the module's writes.
type conn struct{ m *module }

func (c conn) WriteByteData(register, value byte) error {
	c.m.writes = append(c.m.writes, fmt.Sprintf("%02x=%02x", register, value))
	if register == regBinaryStart {
		c.m.at = 0
	}
	return nil
}

func (c conn) WriteBlockData(byte, []byte) error { return errors.New("not used") }

func (c conn) WriteSMBusBlock(register byte, data []byte) error {
	c.m.writes = append(c.m.writes, fmt.Sprintf("%02x:% x", register, data))
	return nil
}

func (c conn) ReadByteData(register byte) (byte, error) {
	if c.m.vengeance {
		if register >= 0xA0 && register <= 0xAF {
			return 0xBA, nil
		}
		return 0x00, nil
	}
	switch register {
	case regIdentify1:
		return c.m.id1, nil
	case regIdentify2:
		return c.m.id2, nil
	case regBinaryData:
		b := c.m.info[c.m.at%infoLen]
		c.m.at++
		return b, nil
	case regChecksum:
		crc := crc8(c.m.info[:])
		if c.m.badCRC {
			crc++
		}
		return crc, nil
	}
	return 0, errors.New("no answer")
}

func (c conn) ReadBlockData(byte, int) ([]byte, error) { return nil, errors.New("not used") }
func (c conn) Close() error                            { return nil }

// raptor is the test machine's memory bus, and gpu an adapter never probed.
var (
	raptor = i2c.Adapter{Path: "/dev/i2c-test21", Name: "SMBus I801 adapter at 0000:00:1f.4", Vendor: 0x8086, Device: 0x7A23}
	gpu    = i2c.Adapter{Path: "/dev/i2c-test10", Name: "NVIDIA i2c adapter 1 at 1:00.0", Vendor: 0x10DE, Device: 0x2216}
)

/*
withModules points the driver at adapters and modules by address. An address
in busy is one the kernel owns, which opening refuses, as i2c-dev does. Every
open is counted.
*/
func withModules(t *testing.T, modules map[uint8]*module, busy map[uint8]bool, adapters ...i2c.Adapter) *[]string {
	t.Helper()
	var opened []string
	wasList, wasOpen, wasPause := listAdapters, openBus, probePause
	listAdapters = func() ([]i2c.Adapter, error) { return adapters, nil }
	openBus = func(path string, addr uint8) (i2c.Bus, error) {
		opened = append(opened, fmt.Sprintf("%s@%02x", path, addr))
		if busy[addr] {
			return nil, syscall.EBUSY
		}
		m, ok := modules[addr]
		if !ok {
			return nil, errors.New("no device")
		}
		return conn{m}, nil
	}
	probePause = 0
	probed.Lock()
	probed.found = map[string][]sighting{}
	probed.Unlock()
	t.Cleanup(func() { listAdapters, openBus, probePause = wasList, wasOpen, wasPause })
	return &opened
}

// The test machine: four Dominator Platinum DDR5 modules at 18 to 1B, and
// SPD hubs the kernel owns at 50 to 53.
func dominators() map[uint8]*module {
	out := map[uint8]*module{}
	for a := uint8(0x18); a <= 0x1B; a++ {
		out[a] = newModule(0x0600, 4)
	}
	return out
}

func TestFindProbesOnlyTheMemoryBusAndOnlyOnce(t *testing.T) {
	opened := withModules(t, dominators(), map[uint8]bool{0x50: true}, gpu, raptor)
	found, err := Driver{}.Find(context.Background())
	require.NoError(t, err)
	require.Len(t, found, 4)
	assert.Equal(t, "Corsair memory (0x18)", found[0].Name)
	assert.Equal(t, raptor.Path, found[0].Path)
	for _, o := range *opened {
		assert.Contains(t, o, raptor.Path, "probed another adapter")
	}
	assert.Len(t, *opened, 16, "addresses 58 to 5F and 18 to 1F")

	*opened = nil
	_, err = Driver{}.Find(context.Background())
	require.NoError(t, err)
	assert.Empty(t, *opened, "probed the bus again")
}

// Something else at an address, answering other values, is not a module;
// probing writes nothing.
func TestSomethingElseIsNotAModule(t *testing.T) {
	other := newModule(0x0600, 4)
	other.id1 = 0x00
	modules := map[uint8]*module{0x1C: other}
	withModules(t, modules, nil, raptor)
	_, err := Driver{}.Find(context.Background())
	require.ErrorIs(t, err, sanshoku.ErrAbsent)
	assert.Empty(t, other.writes)
}

func openModule(t *testing.T, m *module) (*device, error) {
	t.Helper()
	withModules(t, map[uint8]*module{0x18: m}, nil, raptor)
	found, err := Driver{}.Find(context.Background())
	require.NoError(t, err)
	require.Len(t, found, 1)
	dev, err := found[0].Open(context.Background())
	if err != nil {
		return nil, err
	}
	return dev.(*device), nil
}

// Open reads the device information and names the module after its product
// and address, as hotaru 0.1.29 showed it through OpenRGB.
func TestOpenNamesTheModuleFromItsProduct(t *testing.T) {
	m := newModule(0x0600, 4)
	d, err := openModule(t, m)
	require.NoError(t, err)
	assert.Equal(t, []string{"61=00", "21=00"}, m.writes)
	assert.Equal(t, "Corsair Dominator Platinum RGB DDR5 (0x18)", d.Identity().Name)
	assert.Equal(t, "1.2.4660", d.firmware)
	assert.Len(t, d.Keys(), 12)
	assert.Equal(t, "Corsair DRAM LED 0", d.Keys()[0].Name)
	assert.Equal(t, []lighting.Zone{{Name: "Corsair DRAM", Count: 12, Shape: lighting.ShapeLine}}, d.Zones())
}

func TestARefusedModule(t *testing.T) {
	bad := newModule(0x0600, 4)
	bad.badCRC = true
	_, err := openModule(t, bad)
	require.ErrorIs(t, err, errNoAnswer)

	_, err = openModule(t, newModule(0x0F00, 4))
	require.ErrorIs(t, err, sanshoku.ErrUnsupported, "a product not known")

	_, err = openModule(t, newModule(0x0600, 3))
	require.ErrorIs(t, err, sanshoku.ErrUnsupported, "a protocol without direct frames")
}

/*
A frame is the count, the LEDs (last first on a Dominator), and a CRC-8, 32
bytes to 31 and the rest to 32; an unchanged frame sends nothing.
*/
func TestAFrameIsTheLEDsAndACRC(t *testing.T) {
	m := newModule(0x0600, 4)
	d, err := openModule(t, m)
	require.NoError(t, err)
	m.writes = nil
	px := make([]lighting.Pixel, 12)
	for i := range px {
		px[i] = lighting.Pixel{ID: byte(i), R: byte(i), G: 0x10, B: 0x20}
	}
	ctx := context.Background()
	require.NoError(t, d.Frame(ctx, px))
	require.Len(t, m.writes, 2)
	want := []byte{12}
	for i := 11; i >= 0; i-- {
		want = append(want, byte(i), 0x10, 0x20)
	}
	want = append(want, crc8(want))
	assert.Equal(t, fmt.Sprintf("31:% x", want[:32]), m.writes[0])
	assert.Equal(t, fmt.Sprintf("32:% x", want[32:]), m.writes[1])

	m.writes = nil
	require.NoError(t, d.Frame(ctx, px[:3]))
	assert.Empty(t, m.writes)
}

// A ten-LED module's frame fits one block, in order.
func TestAShortFrameIsOneBlock(t *testing.T) {
	m := newModule(0x0700, 4)
	d, err := openModule(t, m)
	require.NoError(t, err)
	m.writes = nil
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 0xAA}}))
	require.Len(t, m.writes, 1)
	assert.Contains(t, m.writes[0], "31:0a aa 00 00")
}

func TestReleaseAndClose(t *testing.T) {
	m := newModule(0x0600, 4)
	d, err := openModule(t, m)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, d.Frame(ctx, []lighting.Pixel{{ID: 0, R: 1}}))
	m.writes = nil
	require.NoError(t, d.Release(ctx))
	assert.Empty(t, m.writes)
	require.Error(t, d.Frame(ctx, []lighting.Pixel{{ID: 12, R: 1}}))
	require.NoError(t, d.Close())
	require.ErrorIs(t, d.Frame(ctx, []lighting.Pixel{{ID: 0, R: 2}}), os.ErrClosed)
}

// Only the allow-list's registers are ever written (spec 042 R1.5).
func TestOnlyTheAllowedRegistersAreWritten(t *testing.T) {
	m := newModule(0x0600, 4)
	d, err := openModule(t, m)
	require.NoError(t, err)
	for _, reg := range []byte{0x0B, 0x20, 0x23, 0x26, 0x63, 0x82} {
		require.Error(t, d.write(reg, 0x00), "%02x", reg)
		require.Error(t, d.block(reg, []byte{1}), "%02x", reg)
	}
}

func TestCRC8(t *testing.T) {
	assert.Equal(t, byte(0xF4), crc8([]byte("123456789")))
}

// The Windows test machine's modules: two original Vengeance RGB at 5A and 5B.
func vengeances() map[uint8]*module {
	return map[uint8]*module{0x5A: {vengeance: true}, 0x5B: {vengeance: true}}
}

var fch = i2c.Adapter{Path: "pawnio:SmbusPIIX4.bin:0", Name: "PawnIO SMBus piix4 0", Vendor: 0x1022, Device: 0x790B}

/*
An original Vengeance RGB is found by A0 to AF reading BA, only by reads,
named as OpenRGB names it, and drawn as one light: no fade, the colour, then
the single-colour mode.
*/
func TestAnOriginalVengeanceIsOneLight(t *testing.T) {
	mods := vengeances()
	withModules(t, mods, nil, fch)
	found, err := Driver{}.Find(context.Background())
	require.NoError(t, err)
	require.Len(t, found, 2)
	assert.Empty(t, mods[0x5A].writes, "probing wrote")
	dev, err := found[0].Open(context.Background())
	require.NoError(t, err)
	d := dev.(*vengeance)
	assert.Equal(t, "Corsair Vengeance RGB (0x5A)", d.Identity().Name)
	assert.Equal(t, []lighting.Key{{ID: 0, Name: "Corsair LED"}}, d.Keys())
	assert.Equal(t, []lighting.Zone{{Name: "Corsair Zone", Count: 1, Shape: lighting.ShapeSingle}}, d.Zones())
	assert.Empty(t, mods[0x5A].writes, "opening wrote")

	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 0x12, G: 0x34, B: 0x56}}))
	assert.Equal(t, []string{"a4=00", "b0=12", "b1=34", "b2=56", "a6=00"}, mods[0x5A].writes)
	assert.Equal(t, vengeanceFloor, d.Floor())
	mods[0x5A].writes = nil
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 0x12, G: 0x34, B: 0x56}}))
	assert.Empty(t, mods[0x5A].writes, "an unchanged frame wrote")
}

// Each family writes only its own registers.
func TestEachFamilyWritesOnlyItsOwnRegisters(t *testing.T) {
	withModules(t, vengeances(), nil, fch)
	found, err := Driver{}.Find(context.Background())
	require.NoError(t, err)
	dev, err := found[0].Open(context.Background())
	require.NoError(t, err)
	d := dev.(*vengeance)
	for _, reg := range []byte{regDeviceInfo, regBinaryStart, regFrame1, 0x23, 0x82, 0xA8, 0xC5} {
		require.Error(t, d.write(reg, 0), "%02x", reg)
	}
	require.Error(t, d.block(regFrame1, []byte{1}))
}

/*
The original Vengeance runs its own effects as Corsair Link writes them
(#122): one colour breathing is mode 0 with a timing and a count of 1; more
colours breathe in mode 2 and fade in mode 1, each in its own slot; the
speed is both timing registers. A frame afterwards shows the colour still,
even an unchanged one.
*/
func TestAnOriginalVengeanceRunsItsOwnEffects(t *testing.T) {
	mods := vengeances()
	withModules(t, mods, nil, fch)
	found, err := Driver{}.Find(context.Background())
	require.NoError(t, err)
	dev, err := found[0].Open(context.Background())
	require.NoError(t, err)
	v, ok := dev.(lighting.Tuned)
	require.True(t, ok, "the original runs no effects of its own")
	assert.Equal(t, []string{"Breathing", "Fading"}, v.Effects())
	assert.Equal(t, 7, v.Catalogue()[0].MaxColours)
	assert.Equal(t, 2, v.Catalogue()[1].MinColours)
	ctx := context.Background()

	require.NoError(t, dev.(lighting.Canvas).Frame(ctx, []lighting.Pixel{{ID: 0, B: 0xFF}}))
	mods[0x5A].writes = nil
	wrote, err := v.SetEffectWith(ctx, "Breathing", lighting.Settings{})
	require.NoError(t, err)
	assert.True(t, wrote)
	cleared := make([]string, 0, 21)
	for r := 0xB0; r <= 0xC4; r++ {
		cleared = append(cleared, fmt.Sprintf("%02x=00", r))
	}
	effect := func(tail ...string) []string { return append(append(slices.Clone(cleared), "a4=00"), tail...) }
	assert.Equal(t, effect("a4=10", "a5=04", "b0=00", "b1=00", "b2=ff", "a7=01", "a6=00"), mods[0x5A].writes,
		"not one colour breathing as Corsair Link writes it")
	effect = func(tail ...string) []string { return append([]string{"a4=00"}, tail...) } // cleared once

	mods[0x5A].writes = nil
	slow := 63
	_, err = v.SetEffectWith(ctx, "Breathing", lighting.Settings{Speed: &slow, Colours: []lighting.Colour{{R: 0xFF}, {G: 0xFF}}})
	require.NoError(t, err)
	assert.Equal(t, effect("a4=3f", "a5=0f", "b0=ff", "b1=00", "b2=00", "b3=00", "b4=ff", "b5=00", "a7=02", "a6=02"), mods[0x5A].writes)

	mods[0x5A].writes = nil
	_, err = v.SetEffectWith(ctx, "Fading", lighting.Settings{Colours: []lighting.Colour{{R: 0xFF}}})
	require.NoError(t, err)
	assert.Equal(t, effect("a4=3f", "a5=0f", "b0=ff", "b1=00", "b2=00", "b3=ff", "b4=00", "b5=00", "a7=02", "a6=01"), mods[0x5A].writes,
		"fading takes two colours at least, the one repeated")
	e, err := v.Effect(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Fading", e.Name)

	mods[0x5A].writes = nil
	require.NoError(t, dev.(lighting.Canvas).Frame(ctx, []lighting.Pixel{{ID: 0, B: 0xFF}}))
	assert.Equal(t, []string{"a4=00", "b0=00", "b1=00", "b2=ff", "a6=00"}, mods[0x5A].writes, "a frame left the effect running")

	// A DDR5 module offers none: its effects are drawn.
	_, isTuned := any(&device{}).(lighting.Tuned)
	assert.False(t, isTuned)
}
