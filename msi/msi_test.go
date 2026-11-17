package msi

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/i2c"
	"github.com/ushineko/sanshoku/lighting"
)

// bus is an I2C device in memory that records every write. The card answers
// nothing, so nothing is read.
type bus struct {
	writes []string
	closed bool
}

func (b *bus) WriteByteData(register, value byte) error {
	b.writes = append(b.writes, fmt.Sprintf("%02x=%02x", register, value))
	return nil
}

func (b *bus) WriteBlockData(register byte, data []byte) error {
	b.writes = append(b.writes, fmt.Sprintf("%02x=% x", register, data))
	return nil
}

func (b *bus) Close() error { b.closed = true; return nil }

var hero = i2c.Adapter{
	Path: "/dev/i2c-test3", Name: "NVIDIA i2c adapter 1 at 1:00.0",
	Vendor: 0x10DE, Device: 0x2684, SubVendor: 0x1462, SubDevice: 0x5104,
}

// withCard points the driver at adapters and a recording bus, with no waits.
func withCard(t *testing.T, adapters ...i2c.Adapter) *bus {
	t.Helper()
	b := &bus{}
	wasList, wasOpen, wasSettle := listAdapters, openBus, settle
	listAdapters = func() ([]i2c.Adapter, error) { return adapters, nil }
	openBus = func(string, uint8) (i2c.Bus, error) { return b, nil }
	settle = 0
	t.Cleanup(func() { listAdapters, openBus, settle = wasList, wasOpen, wasSettle })
	return b
}

func open(t *testing.T) (*device, *bus) {
	t.Helper()
	b := withCard(t, hero)
	found, err := Driver{}.Find(context.Background())
	require.NoError(t, err)
	require.Len(t, found, 1)
	dev, err := found[0].Open(context.Background())
	require.NoError(t, err)
	return dev.(*device), b
}

// Only adapter 1 of an allow-listed card is a candidate, named as OpenRGB
// names the card.
func TestFindMatchesTheCardByItsPCIIDs(t *testing.T) {
	other := hero
	other.SubDevice = 0x5199 // a subsystem on no list
	two := hero
	two.Path, two.Name = "/dev/i2c-test4", "NVIDIA i2c adapter 2 at 1:00.0"
	plain := i2c.Adapter{Path: "/dev/i2c-test0", Name: "SMBus I801 adapter"}
	withCard(t, plain, other, two, hero)
	found, err := Driver{}.Find(context.Background())
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, "MSI GeForce RTX 4090 Suprim Liquid X", found[0].Name)
	assert.Equal(t, "/dev/i2c-test3", found[0].Path)
	assert.Equal(t, sanshoku.BusI2C, found[0].Bus)

	withCard(t, plain)
	_, err = Driver{}.Find(context.Background())
	require.ErrorIs(t, err, sanshoku.ErrAbsent)
}

// A colour is the static mode: OpenRGB's six writes and the mode last; the
// same colour again sends nothing.
func TestAFrameIsTheStaticMode(t *testing.T) {
	d, b := open(t)
	assert.Equal(t, []lighting.Key{{ID: 0, Name: "Color"}}, d.Keys())
	assert.Equal(t, []lighting.Zone{{Name: "GPU", Count: 1, Shape: lighting.ShapeSingle}}, d.Zones())

	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 0x11, G: 0x22, B: 0x33}}))
	assert.Equal(t, []string{"2e=00", "22=1c", "30=11", "31=22", "32=33", "36=64", "22=13"}, b.writes)

	b.writes = nil
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 0x11, G: 0x22, B: 0x33}}))
	assert.Empty(t, b.writes)

	require.Error(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 1}}))
	assert.Empty(t, b.writes)
}

// Breathing is two colour blocks in B, G, R, full brightness, medium speed,
// then the mode; a colour replaces the first block.
func TestBreathingAsOpenRGBWritesIt(t *testing.T) {
	d, b := open(t)
	_, err := d.Effect(context.Background())
	require.ErrorIs(t, err, lighting.ErrUnknownEffect)

	wrote, err := d.SetEffect(context.Background(), "breathing")
	require.NoError(t, err)
	assert.True(t, wrote)
	assert.Equal(t, []string{"2e=00", "22=1c", "27=00 00 ff", "28=00 ff 00", "36=64", "38=02", "22=04"}, b.writes)

	b.writes = nil
	wrote, err = d.SetColour(context.Background(), lighting.Colour{R: 0x10, G: 0x20, B: 0x30})
	require.NoError(t, err)
	assert.True(t, wrote)
	assert.Equal(t, "27=30 20 10", b.writes[2])
	e, err := d.Effect(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Breathing", e.Name)
	assert.Equal(t, lighting.Colour{R: 0x10, G: 0x20, B: 0x30}, *e.Colour)

	b.writes = nil
	wrote, err = d.SetEffect(context.Background(), "Breathing")
	require.NoError(t, err)
	assert.False(t, wrote)
	assert.Empty(t, b.writes)
}

func TestTheOtherShapes(t *testing.T) {
	cases := map[string][]string{
		"Flashing":     {"2e=00", "22=1c", "30=ff", "31=00", "32=00", "36=64", "38=02", "22=02"},
		"Rainbow Wave": {"46=00", "2e=00", "22=1c", "36=64", "38=02", "22=08"},
		"Magic":        {"2e=00", "22=1c", "46=00", "36=64", "38=02", "22=07"},
		"Wave":         {"46=01", "2e=00", "22=1c", "27=00 00 ff", "28=00 ff 00", "29=ff 00 00", "36=64", "38=02", "22=08"},
		"Color Cycle":  {"2e=00", "22=1c", "46=01", "27=00 00 ff", "28=00 ff 00", "29=ff 00 00", "36=64", "38=02", "22=07"},
		"Off":          {"2e=00", "22=1c", "22=01"},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			d, b := open(t)
			_, err := d.SetEffect(context.Background(), name)
			require.NoError(t, err)
			assert.Equal(t, want, b.writes)
		})
	}
}

func TestAColourlessEffectTakesNoColour(t *testing.T) {
	d, _ := open(t)
	_, err := d.SetEffect(context.Background(), "Lightning")
	require.NoError(t, err)
	_, err = d.SetColour(context.Background(), lighting.Colour{R: 1})
	require.ErrorIs(t, err, lighting.ErrNoColour)
	_, err = d.SetEffect(context.Background(), "Disco")
	require.ErrorIs(t, err, lighting.ErrNoEffect)
}

// A colour after an effect is the static mode again.
func TestAFrameAfterAnEffectIsSent(t *testing.T) {
	d, b := open(t)
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 1}}))
	_, err := d.SetEffect(context.Background(), "Breathing")
	require.NoError(t, err)
	b.writes = nil
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 1}}))
	assert.Equal(t, "22=13", b.writes[len(b.writes)-1])
	_, err = d.Effect(context.Background())
	require.ErrorIs(t, err, lighting.ErrUnknownEffect)
}

func TestReleaseAndCloseSendNothing(t *testing.T) {
	d, b := open(t)
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 1}}))
	b.writes = nil
	require.NoError(t, d.Release(context.Background()))
	require.NoError(t, d.Close())
	assert.Empty(t, b.writes)
	assert.True(t, b.closed)
}

func TestTheCatalogue(t *testing.T) {
	d, _ := open(t)
	cat := d.Catalogue()
	require.Len(t, cat, 17)
	assert.Equal(t, lighting.EffectInfo{Name: "Breathing", Number: 4, Speeds: []int{4, 2, 1}, Speed: 2, MinColours: 1, MaxColours: 2}, cat[14])
	assert.Equal(t, lighting.EffectInfo{Name: "Off", Number: 1}, cat[0])
	assert.Equal(t, []string{"lighting", "onboard"}, sanshoku.Capabilities(d))
}

// Breathing's two colours go into both blocks, and the speed into its
// register, the nearest of the card's three (spec 032).
func TestBreathingTakesTwoColoursAndASpeed(t *testing.T) {
	d, b := open(t)
	ctx := context.Background()
	teal, gold := lighting.Colour{G: 0x80, B: 0x80}, lighting.Colour{R: 0xFF, G: 0xC0}
	fast := 1
	wrote, err := d.SetEffectWith(ctx, "Breathing", lighting.Settings{Colours: []lighting.Colour{teal, gold}, Speed: &fast})
	require.NoError(t, err)
	assert.True(t, wrote)
	assert.Equal(t, []string{"2e=00", "22=1c", "27=80 80 00", "28=00 c0 ff", "36=64", "38=01", "22=04"}, b.writes, "blocks are B G R")

	b.writes = nil
	wrote, err = d.SetEffectWith(ctx, "breathing", lighting.Settings{Colours: []lighting.Colour{teal, gold}, Speed: &fast})
	require.NoError(t, err)
	assert.False(t, wrote)
	assert.Empty(t, b.writes)

	slowish := 3
	_, err = d.SetEffectWith(ctx, "Breathing", lighting.Settings{Speed: &slowish})
	require.NoError(t, err)
	assert.Contains(t, b.writes, "38=04", "3 is nearest 4, the slow setting")
	assert.Contains(t, b.writes, "27=80 80 00", "the colours were kept")

	running, err := d.Effect(ctx)
	require.NoError(t, err)
	assert.Equal(t, []lighting.Colour{teal, gold}, running.Colours)
	require.NotNil(t, running.Speed)
	assert.Equal(t, 4, *running.Speed)

	_, err = d.SetEffectWith(ctx, "Breathing", lighting.Settings{Colours: []lighting.Colour{teal, gold, teal}})
	require.ErrorIs(t, err, lighting.ErrTooManyColours)
	_, err = d.SetEffectWith(ctx, "Rainbow Wave", lighting.Settings{Colours: []lighting.Colour{teal}})
	require.ErrorIs(t, err, lighting.ErrNoColour)
}

// Six cards lack Stack and Rhythm, and an RTX 50 card takes the coloured
// Rhythm as mode 0B (spec 033).
func TestACardHasItsFamilysEffects(t *testing.T) {
	names := func(fx []effect) []string {
		var out []string
		for _, e := range fx {
			out = append(out, e.name)
		}
		return out
	}
	assert.Len(t, effectsFor(familyV2), len(effects))
	assert.NotContains(t, names(effectsFor(familyNoStack)), "Stack")
	assert.NotContains(t, names(effectsFor(familyNoStack)), "Rhythm")
	for _, e := range effectsFor(familyRTX50) {
		if e.name == "Rhythm" {
			assert.Equal(t, byte(0x0B), e.mode)
		}
	}
	for _, e := range effects {
		if e.name == "Rhythm" {
			assert.Equal(t, byte(0x0C), e.mode, "the shared table was changed")
		}
	}
	assert.Equal(t, familyRTX50, cards[card{0x10DE, 0x2B85, 0x1462, 0x5300}].family, "the 5090 Suprim Liquid")
	assert.Len(t, cards, 47)
}
