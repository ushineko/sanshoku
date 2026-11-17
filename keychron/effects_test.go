package keychron

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku/lighting"
)

// The effects are OpenRGB's, numbered 1 to 22, and the reactive ones coloured.
func TestTheEffectsAreOpenRGBs(t *testing.T) {
	d, err := opened(newFake())
	require.NoError(t, err)
	names := d.Effects()
	require.Len(t, names, 22)
	assert.Equal(t, "Solid Color", names[0])
	assert.Equal(t, "Solid Splash", names[21])
	cat := d.Catalogue()
	assert.Equal(t, lighting.EffectInfo{Name: "Solid Reactive Multinexus", Number: 20, MinColours: 1, MaxColours: 1}, cat[19])
	assert.Equal(t, lighting.EffectInfo{Name: "Cycle All", Number: 4}, cat[3])
}

// The running effect is read with its stored colour at the board's brightness;
// the per-key effect is not one of the keyboard's own.
func TestTheRunningEffectIsRead(t *testing.T) {
	f := newFake()
	d, err := opened(f)
	require.NoError(t, err)
	e, err := d.Effect(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Solid Reactive Multinexus", e.Name)
	require.NotNil(t, e.Colour)
	h, s, v := hsv(colour{e.Colour.R, e.Colour.G, e.Colour.B})
	assert.Equal(t, [3]uint8{0x3B, 0x86, 200}, [3]uint8{h, s, v})

	f.Effect = perKeyEffect
	_, err = d.Effect(context.Background())
	require.ErrorIs(t, err, lighting.ErrUnknownEffect)
}

// An effect is set only where another runs, and the next frame claims again
// and gives this one back on release.
func TestSetEffectThenAFrameThenRelease(t *testing.T) {
	f := newFake()
	d, err := opened(f)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, d.Frame(ctx, []lighting.Pixel{{ID: 0, R: 255}}))
	require.Equal(t, uint8(perKeyEffect), f.Effect)

	wrote, err := d.SetEffect(ctx, "solid reactive multiwide")
	require.NoError(t, err)
	assert.True(t, wrote)
	assert.Equal(t, uint8(19), f.Effect)
	wrote, err = d.SetEffect(ctx, "Solid Reactive Multiwide")
	require.NoError(t, err)
	assert.False(t, wrote)
	_, err = d.SetEffect(ctx, "Direct")
	require.ErrorIs(t, err, lighting.ErrNoEffect)

	require.NoError(t, d.Frame(ctx, []lighting.Pixel{{ID: 0, G: 255}}))
	require.Equal(t, uint8(perKeyEffect), f.Effect)
	require.NoError(t, d.Release(ctx))
	assert.Equal(t, uint8(19), f.Effect)
}

// A colour goes as hue and saturation, and its value as the brightness, each
// only where it differs; the effect is kept.
func TestSetColourWritesWhatDiffers(t *testing.T) {
	f := newFake()
	d, err := opened(f)
	require.NoError(t, err)
	ctx := context.Background()
	h, s, _ := hsv(colour{0x9f, 0xc9, 0x5f})
	f.Hue, f.Sat = h, s
	f.reset()
	wrote, err := d.SetColour(ctx, lighting.Colour{R: 0x9f, G: 0xc9, B: 0x5f})
	require.NoError(t, err)
	assert.True(t, wrote)
	assert.Equal(t, []string{"08 03 04", "08 03 01", "07 03 01"}, f.told(3), "only the brightness differed")
	assert.Equal(t, uint8(0xc9), f.Brightness)
	assert.Equal(t, uint8(20), f.Effect)

	f.reset()
	wrote, err = d.SetColour(ctx, lighting.Colour{R: 0x9f, G: 0xc9, B: 0x5f})
	require.NoError(t, err)
	assert.False(t, wrote)
	assert.Equal(t, []string{"08 03 04", "08 03 01"}, f.told(3))

	wrote, err = d.SetColour(ctx, lighting.Colour{R: 255})
	require.NoError(t, err)
	assert.True(t, wrote)
	assert.Equal(t, [3]uint8{0, 255, 255}, [3]uint8{f.Hue, f.Sat, f.Brightness})
}

// rgb undoes hsv, within rounding.
func TestRGBUndoesHSV(t *testing.T) {
	for _, c := range []colour{{255, 0, 0}, {0, 255, 0}, {0, 0, 255}, {0x9f, 0xc9, 0x5f}, {128, 128, 128}, {255, 128, 0}} {
		h, s, v := hsv(c)
		got := rgb(h, s, v)
		for i, pair := range [][2]uint8{{got.r, c.r}, {got.g, c.g}, {got.b, c.b}} {
			assert.InDelta(t, int(pair[1]), int(pair[0]), 3, "%v channel %d", c, i)
		}
	}
}
