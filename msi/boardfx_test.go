package msi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku/lighting"
)

// The header offers the chip's own effects, each with its colours and
// speeds (#122).
func TestTheHeadersOwnEffects(t *testing.T) {
	d, err := openTheBoard(t, newLPC(0xD352))
	require.NoError(t, err)
	assert.Equal(t, []string{"Breathing", "Flashing", "Color Cycle"}, d.Effects())
	cat := d.Catalogue()
	assert.Empty(t, cat[0].Speeds, "the pulse's pace is the chip's own")
	assert.Equal(t, []int{7, 6, 5, 4, 3, 2}, cat[1].Speeds)
	assert.Equal(t, 8, cat[2].MaxColours)
	_, err = d.Effect(context.Background())
	require.ErrorIs(t, err, lighting.ErrUnknownEffect)
}

/*
Breathing is the chip's pulse in one colour: all eight frames the colour,
E4's pulse bit, no step time. The same effect again sends nothing; a frame
afterwards turns the pulse off and shows the colour still.
*/
func TestBreathingIsTheChipsPulse(t *testing.T) {
	chip := newLPC(0xD352)
	d, err := openTheBoard(t, chip)
	require.NoError(t, err)
	ctx := context.Background()
	wrote, err := d.SetEffectWith(ctx, "Breathing", lighting.Settings{Colours: []lighting.Colour{{R: 0, G: 0x40, B: 0xFF}}})
	require.NoError(t, err)
	assert.True(t, wrote)
	for _, w := range []string{"12:f0=00", "12:f3=00", "12:f4=44", "12:f8=ff", "12:fb=ff", "12:e4=08", "12:fe=00", "12:ff=fe"} {
		assert.Contains(t, chip.writes, w)
	}
	e, err := d.Effect(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Breathing", e.Name)
	assert.Equal(t, lighting.Colour{G: 0x40, B: 0xFF}, *e.Colour)

	chip.writes = nil
	wrote, err = d.SetEffectWith(ctx, "Breathing", lighting.Settings{})
	require.NoError(t, err)
	assert.False(t, wrote)
	assert.Empty(t, chip.writes)

	require.NoError(t, d.Frame(ctx, []lighting.Pixel{{ID: 0, R: 0xFF}}))
	assert.Contains(t, chip.writes, "12:e4=00", "a frame left the pulse on")
	_, err = d.Effect(ctx)
	require.ErrorIs(t, err, lighting.ErrUnknownEffect)
}

// Flashing writes its period into E4's blink bits; Color Cycle its colours
// in turn into the eight frames, and the nine-bit step time.
func TestFlashingAndColorCycle(t *testing.T) {
	chip := newLPC(0xD352)
	d, err := openTheBoard(t, chip)
	require.NoError(t, err)
	ctx := context.Background()
	slow := 7
	_, err = d.SetEffectWith(ctx, "Flashing", lighting.Settings{Speed: &slow, Colours: []lighting.Colour{{R: 0xFF}}})
	require.NoError(t, err)
	assert.Contains(t, chip.writes, "12:e4=07")

	chip.writes = nil
	step := 0x1FF
	_, err = d.SetEffectWith(ctx, "Color Cycle", lighting.Settings{Speed: &step,
		Colours: []lighting.Colour{{R: 0xFF}, {G: 0xFF}}})
	require.NoError(t, err)
	// Frames 0 and 1 are red then green: the high nibble is frame 1.
	for _, w := range []string{"12:f0=0f", "12:f4=f0", "12:f8=00", "12:e4=00", "12:fe=ff", "12:ff=ff"} {
		assert.Contains(t, chip.writes, w)
	}
	_, err = d.SetEffectWith(ctx, "Breathing", lighting.Settings{Colours: []lighting.Colour{{R: 1}, {G: 1}}})
	require.ErrorIs(t, err, lighting.ErrTooManyColours)
	_, err = d.SetEffectWith(ctx, "Rainbow", lighting.Settings{})
	require.ErrorIs(t, err, lighting.ErrNoEffect)
}
