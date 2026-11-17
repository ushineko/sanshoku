package lighting_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku/lighting"
)

var (
	red  = lighting.Colour{R: 255}
	blue = lighting.Colour{B: 255}
)

func speed(n int) *int { return &n }

// Settings are checked against the catalogue, and fewer colours repeat the
// last (spec 032 R1.2).
func TestFitChecksSettingsAgainstTheCatalogue(t *testing.T) {
	taiChi := lighting.EffectInfo{Name: "Tai Chi", Speeds: []int{30}, MinColours: 2, MaxColours: 2}
	got, sp, err := lighting.Fit(taiChi, lighting.Settings{Colours: []lighting.Colour{red}}, false)
	require.NoError(t, err)
	assert.Equal(t, []lighting.Colour{red, red}, got)
	assert.Nil(t, sp)

	_, _, err = lighting.Fit(taiChi, lighting.Settings{Colours: []lighting.Colour{red, blue, red}}, false)
	require.ErrorIs(t, err, lighting.ErrTooManyColours)
	_, _, err = lighting.Fit(taiChi, lighting.Settings{Speed: speed(31)}, false)
	require.ErrorIs(t, err, lighting.ErrSpeed)

	wave := lighting.EffectInfo{Name: "Spectrum Wave", Speeds: []int{350, 300, 250, 150, 80}}
	_, _, err = lighting.Fit(wave, lighting.Settings{Colours: []lighting.Colour{red}}, false)
	require.ErrorIs(t, err, lighting.ErrNoColour)
	_, sp, err = lighting.Fit(wave, lighting.Settings{Speed: speed(200)}, false)
	require.NoError(t, err)
	assert.Equal(t, 200, *sp, "a period between the listed ones is taken as it is")
	_, sp, err = lighting.Fit(wave, lighting.Settings{Speed: speed(180)}, true)
	require.NoError(t, err)
	assert.Equal(t, 150, *sp, "or the nearest, for a device that takes only those")
	_, _, err = lighting.Fit(wave, lighting.Settings{Speed: speed(10)}, false)
	require.ErrorIs(t, err, lighting.ErrSpeed)
}
