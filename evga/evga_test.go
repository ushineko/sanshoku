package evga

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/i2c"
	"github.com/ushineko/sanshoku/lighting"
)

/*
bus is the controller in memory: it answers block reads from regs, as the
card answers OpenRGB's (a length byte, then the register's bytes), and records
every write. The values are made up in the card's layout.
*/
type bus struct {
	regs   map[byte][]byte
	writes []string
	closed bool
}

func newBus() *bus {
	return &bus{regs: map[byte][]byte{
		regFirmware: {0x05, 0, 0, 0, 0x00, 0x0C},
		// Zone modes 1, 1, 1, 1; LED counts 1, 1, 1, 0 (no addressable
		// header); zones in step.
		regModes:      {0x09, 1, 1, 1, 1, 1, 1, 1, 0, 1},
		regStatic:     {0x04, 0xFF, 0x10, 0x20, 0x30},
		regStatic + 1: {0x04, 0xFF, 0x40, 0x50, 0x60},
		regStatic + 2: {0x04, 0x80, 0x70, 0x80, 0x90},
	}}
}

func (b *bus) WriteByteData(register, value byte) error {
	b.writes = append(b.writes, fmt.Sprintf("%02x=%02x", register, value))
	return nil
}

func (b *bus) WriteBlockData(register byte, data []byte) error {
	b.writes = append(b.writes, fmt.Sprintf("%02x=% x", register, data))
	return nil
}

func (b *bus) WriteSMBusBlock(byte, []byte) error { return errors.New("not used") }
func (b *bus) ReadByteData(byte) (byte, error)    { return 0, errors.New("not used") }

func (b *bus) ReadBlockData(register byte, n int) ([]byte, error) {
	r, ok := b.regs[register]
	if !ok {
		return nil, errors.New("no answer")
	}
	return append([]byte(nil), r[:min(n, len(r))]...), nil
}

func (b *bus) Close() error { b.closed = true; return nil }

var ftw3 = i2c.Adapter{
	Path: "/dev/i2c-test10", Name: "NVIDIA i2c adapter 1 at 1:00.0",
	Vendor: 0x10DE, Device: 0x2216, SubVendor: 0x3842, SubDevice: 0x4897,
}

// withCard points the driver at adapters and b.
func withCard(t *testing.T, b *bus, adapters ...i2c.Adapter) {
	t.Helper()
	wasList, wasOpen := listAdapters, openBus
	listAdapters = func() ([]i2c.Adapter, error) { return adapters, nil }
	openBus = func(_ string, addr uint8) (i2c.Bus, error) {
		require.Equal(t, uint8(address), addr)
		return b, nil
	}
	t.Cleanup(func() { listAdapters, openBus = wasList, wasOpen })
}

func open(t *testing.T, b *bus) *device {
	t.Helper()
	withCard(t, b, ftw3)
	found, err := Driver{}.Find(context.Background())
	require.NoError(t, err)
	require.Len(t, found, 1)
	dev, err := found[0].Open(context.Background())
	require.NoError(t, err)
	return dev.(*device)
}

func TestFindMatchesTheCardOnItsFirstAdapter(t *testing.T) {
	other := ftw3
	other.Name = "NVIDIA i2c adapter 5 at 1:00.0"
	unknown := ftw3
	unknown.SubDevice = 0x1234
	withCard(t, newBus(), other, unknown, ftw3)
	found, err := Driver{}.Find(context.Background())
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, "EVGA GeForce RTX 3080 FTW3 Ultra LHR", found[0].Name)
	assert.Equal(t, ftw3.Path, found[0].Path)

	withCard(t, newBus(), unknown)
	_, err = Driver{}.Find(context.Background())
	require.ErrorIs(t, err, sanshoku.ErrAbsent)
}

// Open reads and writes nothing; the zones with lights are the keys.
func TestOpenReadsTheZonesAndWritesNothing(t *testing.T) {
	b := newBus()
	d := open(t, b)
	assert.Empty(t, b.writes)
	assert.Equal(t, "1.00.12", d.firmware)
	assert.Equal(t, []lighting.Key{
		{ID: 0, Name: "Front Logo"}, {ID: 1, Name: "End plate Logo"}, {ID: 2, Name: "Back Logo"},
	}, d.Keys())
}

// Each lit zone is a zone of one light under OpenRGB's name, so a scene made
// through OpenRGB finds it (#118); none can be resized.
func TestTheZonesAreOpenRGBs(t *testing.T) {
	d := open(t, newBus())
	assert.Equal(t, []lighting.Zone{
		{Name: "Front Logo", First: 0, Count: 1, Shape: lighting.ShapeSingle},
		{Name: "End plate Logo", First: 1, Count: 1, Shape: lighting.ShapeSingle},
		{Name: "Back Logo", First: 2, Count: 1, Shape: lighting.ShapeSingle},
	}, d.Zones())
	require.ErrorIs(t, d.Resize("Front Logo", 2), lighting.ErrNoZone)
}

func TestACardThatDoesNotAnswerIsNotOpened(t *testing.T) {
	b := newBus()
	delete(b.regs, regFirmware)
	withCard(t, b, ftw3)
	found, err := Driver{}.Find(context.Background())
	require.NoError(t, err)
	_, err = found[0].Open(context.Background())
	require.ErrorIs(t, err, errNoAnswer)
	assert.True(t, b.closed)
	assert.Empty(t, b.writes)
}

/*
The first frame unlocks the card and sets every zone static, then writes each
zone's static register; the next sends only what changed.
*/
func TestAFrameClaimsTheCardThenWritesEachZone(t *testing.T) {
	b := newBus()
	d := open(t, b)
	ctx := context.Background()
	require.NoError(t, d.Frame(ctx, []lighting.Pixel{
		{ID: 0, R: 1, G: 2, B: 3}, {ID: 1, R: 4, G: 5, B: 6}, {ID: 2, R: 7, G: 8, B: 9},
	}))
	assert.Equal(t, []string{
		"b2=04 c6 eb ea 15",
		"c0=09 01 01 01 01 ff ff ff ff 01",
		"c1=04 ff 01 02 03",
		"c2=04 ff 04 05 06",
		"c3=04 ff 07 08 09",
	}, b.writes)
	b.writes = nil
	require.NoError(t, d.Frame(ctx, []lighting.Pixel{{ID: 1, R: 9, G: 9, B: 9}, {ID: 0, R: 1, G: 2, B: 3}}))
	assert.Equal(t, []string{"c2=04 ff 09 09 09"}, b.writes)
}

func TestAKeyNotOnTheCardSendsNothing(t *testing.T) {
	b := newBus()
	d := open(t, b)
	require.Error(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 3, R: 1}}))
	assert.Empty(t, b.writes)
}

// Release puts back the colours and modes read at open; with nothing drawn,
// it sends nothing.
func TestReleasePutsBackWhatTheCardShowed(t *testing.T) {
	b := newBus()
	d := open(t, b)
	ctx := context.Background()
	require.NoError(t, d.Release(ctx))
	assert.Empty(t, b.writes)

	require.NoError(t, d.Frame(ctx, []lighting.Pixel{{ID: 0, R: 1}}))
	b.writes = nil
	require.NoError(t, d.Release(ctx))
	assert.Equal(t, []string{
		"b2=04 c6 eb ea 15",
		"c1=04 ff 10 20 30",
		"c2=04 ff 40 50 60",
		"c3=04 80 70 80 90",
		"c0=09 01 01 01 01 01 01 01 00 01",
	}, b.writes)

	b.writes = nil
	require.NoError(t, d.Frame(ctx, []lighting.Pixel{{ID: 0, R: 1}}))
	assert.Equal(t, "b2=04 c6 eb ea 15", b.writes[0], "a frame after release claims the card again")
	require.NoError(t, d.Close())
	require.ErrorIs(t, d.Frame(ctx, []lighting.Pixel{{ID: 0, R: 2}}), os.ErrClosed)
}

// The save register is never written, and write refuses it outright.
func TestTheSaveRegisterIsNeverWritten(t *testing.T) {
	b := newBus()
	d := open(t, b)
	ctx := context.Background()
	require.NoError(t, d.Frame(ctx, []lighting.Pixel{{ID: 0, R: 1}, {ID: 2, B: 1}}))
	require.NoError(t, d.Release(ctx))
	for _, w := range b.writes {
		assert.NotEqual(t, "90", w[:2])
	}
	require.Error(t, d.write(regSave, []byte{0x04, 0x9E, 0xEB, 0x00, 0x90}))
}

// Every card on the list is on NVIDIA's and EVGA's vendor IDs.
func TestTheCardsAreEVGAsOnNVIDIA(t *testing.T) {
	require.Len(t, cards, 60)
	for c, name := range cards {
		assert.Equal(t, uint16(0x10DE), c.vendor, name)
		assert.Equal(t, uint16(0x3842), c.subVendor, name)
	}
}
