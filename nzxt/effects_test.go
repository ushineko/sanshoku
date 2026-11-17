package nzxt

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku/lighting"
)

// effectNamed is the table's entry for name.
func effectNamed(t *testing.T, name string) effect {
	t.Helper()
	for _, e := range effects {
		if e.name == name {
			return e
		}
	}
	t.Fatalf("no effect %q", name)
	return effect{}
}

/*
The `2A 04` reports byte for byte, as CAM sent them on firmware 1.2.12: one
effect with colours, one with none, and Alternating, whose bytes 0x37 and
0x3B differ from the rest.
*/
func TestEffectReportsAreCAMs(t *testing.T) {
	b := effectReport(0, effectNamed(t, "Breathing"), effectNamed(t, "Breathing").colours, 24)
	require.Len(t, b, lightReportLen)
	assert.Equal(t, []byte{0x2A, 0x04, 0x01, 0x01, 0x07, 20, 0x00, 0x00, 0x39, 0x58, 0x00}, b[:11], "G, R, B")
	assert.Equal(t, []byte{0x00, 0x01, 0x08, 0x18, 0x03}, b[0x37:0x3C])

	w := effectReport(1, effectNamed(t, "Spectrum Wave"), nil, 24)
	assert.Equal(t, []byte{0x2A, 0x04, 0x02, 0x02, 0x02, 0xFA, 0x00, 0x00}, w[:8], "speed 250, no colours")
	assert.Equal(t, []byte{0x00, 0x00, 0x00, 0x18, 0x03}, w[0x37:0x3C])

	alt := effectNamed(t, "Alternating")
	a := effectReport(0, alt, alt.colours, 24)
	assert.Equal(t, []byte{0xF4, 0x01}, a[5:7], "speed 500")
	assert.Equal(t, []byte{0x00, 0x39, 0x58, 0xBC, 0x00, 0x83}, a[7:13])
	assert.Equal(t, []byte{0x01, 0x02, 0x00, 0x18, 0x04}, a[0x37:0x3C])
}

func TestTheCatalogueDescribesEveryEffect(t *testing.T) {
	d := withFake(newFake())
	cat := d.Catalogue()
	require.Len(t, cat, 14)
	assert.Equal(t, d.Effects()[0], cat[0].Name)
	tai := cat[11]
	assert.Equal(t, lighting.EffectInfo{Name: "Tai Chi", Number: 0x0E, Speeds: []int{30}, Speed: 30, MinColours: 2, MaxColours: 2}, tai)
	for _, e := range cat {
		assert.Contains(t, e.Speeds, e.Speed, "%s runs at a speed it does not list", e.Name)
	}
}

func TestSetEffectRunsOnEveryChannelOnce(t *testing.T) {
	f := newFake()
	d := withFake(f)
	ctx := context.Background()

	_, err := d.Effect(ctx)
	require.ErrorIs(t, err, lighting.ErrUnknownEffect, "an effect before one was set")

	wrote, err := d.SetEffect(ctx, "breathing")
	require.NoError(t, err)
	assert.True(t, wrote)
	assert.Equal(t, []string{"20 03 00", "2a 04 01", "2a 04 02"}, effectCommands(f))

	wrote, err = d.SetEffect(ctx, "Breathing")
	require.NoError(t, err)
	assert.False(t, wrote, "the same effect again was sent")
	assert.Len(t, effectCommands(f), 3)

	running, err := d.Effect(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Breathing", running.Name)
	require.NotNil(t, running.Colour)
	assert.Equal(t, camPurple, *running.Colour)

	_, err = d.SetEffect(ctx, "Disco")
	require.ErrorIs(t, err, lighting.ErrNoEffect)
}

func TestSetColourIsTheEffectsFirstColour(t *testing.T) {
	f := newFake()
	d := withFake(f)
	ctx := context.Background()
	gold := lighting.Colour{R: 0xFF, G: 0xC0, B: 0x00}

	_, err := d.SetColour(ctx, gold)
	require.ErrorIs(t, err, lighting.ErrUnknownEffect)

	_, err = d.SetEffect(ctx, "Tai Chi")
	require.NoError(t, err)
	wrote, err := d.SetColour(ctx, gold)
	require.NoError(t, err)
	assert.True(t, wrote)
	last := f.Told[len(f.Told)-1]
	assert.Equal(t, []byte{0xC0, 0xFF, 0x00, 0x00, 0x00, 0xBC, 0x00}, last[7:14], "gold first, CAM's blue second, and no third")
	assert.Equal(t, byte(2), last[0x38])

	wrote, err = d.SetColour(ctx, gold)
	require.NoError(t, err)
	assert.False(t, wrote, "the same colour again was sent")

	_, err = d.SetEffect(ctx, "Rainbow Flow")
	require.NoError(t, err)
	_, err = d.SetColour(ctx, gold)
	require.ErrorIs(t, err, lighting.ErrNoColour)

	// The colour carries to the next effect that takes one.
	_, err = d.SetEffect(ctx, "Pulse")
	require.NoError(t, err)
	last = f.Told[len(f.Told)-1]
	assert.Equal(t, []byte{0xC0, 0xFF, 0x00}, last[7:10])
}

/*
An effect set over a drawn frame is not replaced by the commit before a
readout: the effect is what the channel's mode now shows (spec 024 R1.13).
*/
func TestAnEffectIsNotCommittedOverByTheReadout(t *testing.T) {
	f := newFake()
	p := withPanel(f, &recorder{})
	ctx := context.Background()

	require.NoError(t, p.Frame(ctx, []lighting.Pixel{{ID: 0, R: 255}}))
	_, err := p.SetEffect(ctx, "Candle")
	require.NoError(t, err)
	n := len(lights(f))
	require.NoError(t, p.Readout(ctx))
	assert.Equal(t, []string{"38 01 02"}, lights(f)[n:], "the readout committed a frame over the effect")
}

// effectCommands is the accessory question and the effect commands written.
func effectCommands(f *fake) []string {
	var out []string
	for _, r := range f.told(3) {
		if r[0] == 0x20 || r[0] == 0x2A {
			out = append(out, hexs(r))
		}
	}
	return out
}
