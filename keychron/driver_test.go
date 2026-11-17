package keychron

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/lighting"
)

func TestOpenAsksWhatTheKeyboardIs(t *testing.T) {
	f := newFake()
	d, err := opened(f)
	require.NoError(t, err)
	assert.Equal(t, []string{"01", "a2", "a8"}, f.told(1))
	assert.Equal(t, "Keychron K4 HE", d.Identity().Name)
	assert.Len(t, d.Keys(), 100)
	assert.Equal(t, []lighting.Zone{{Name: "Keyboard", Count: 100}}, d.Zones())
	require.ErrorIs(t, d.Resize("Keyboard", 10), lighting.ErrNoZone)

	noRGB := newFake()
	noRGB.Features = 0x0F
	_, err = opened(noRGB)
	require.ErrorIs(t, err, errNoRGB)
	assert.True(t, noRGB.closed)

	silent := newFake()
	silent.Silent = true
	_, err = opened(silent)
	require.ErrorIs(t, err, errNoAnswer)
}

func TestAProductOffTheListIsNeverOpened(t *testing.T) {
	n := k4Node
	n.Product = 0x0E41
	_, err := candidate(n, func(string) (node, error) { t.Fatal("opened"); return nil, nil }).Open(context.Background())
	require.ErrorIs(t, err, sanshoku.ErrUnsupported)
}

/*
The first frame reads the effect, records the keyboard's own and its
brightness, claims the per-key effect, sends every key and the brightness of
the brightest key.
*/
func TestTheFirstFrameClaimsAndSendsEverything(t *testing.T) {
	f := newFake()
	d, err := opened(f)
	require.NoError(t, err)
	f.reset()

	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 0xFF}}))
	got := f.told(3)
	assert.Equal(t, []string{"08 03 02", "08 03 01", "07 03 02", "a8 08 00"}, got[:4])
	assert.Equal(t, byte(perKeyEffect), f.Told[2][3])
	assert.Len(t, got, 4+12+1, "the claim, 12 packets of keys, the brightness")
	assert.Equal(t, "a8 0a 00", got[4])
	assert.Equal(t, []byte{0x00, 0xFF, 0xFF}, f.Told[4][4:7], "red: hue 0, full saturation and value")
	assert.Equal(t, "07 03 01", got[16])
	assert.Equal(t, byte(0xFF), f.Told[16][3], "the brightest key")
	assert.Equal(t, &setting{effect: 20, brightness: 200}, d.found)
}

// Only the packets whose hue or saturation changed go; a change of value
// alone is the board's brightness.
func TestOnlyWhatChangedIsSent(t *testing.T) {
	f := newFake()
	d, err := opened(f)
	require.NoError(t, err)
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 0xFF}}))
	f.reset()

	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 20, G: 0xFF}}))
	assert.Equal(t, []string{"08 03 02", "a8 0a 12"}, f.told(3), "no claim; only the packet from key 18")

	f.reset()
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 0x40}, {ID: 20, G: 0x40}}))
	assert.Equal(t, []string{"08 03 02", "07 03 01"}, f.told(3), "dimmer: the same hues, a lower brightness")
	assert.Equal(t, byte(0x40), f.Told[1][3])

	f.reset()
	require.NoError(t, d.Frame(context.Background(), nil))
	assert.Equal(t, []string{"08 03 02"}, f.told(3), "nothing changed: only the look at the effect")
}

// The keyboard's own keys can change its effect; the next frame takes it
// back and sends everything again.
func TestAnEffectChangedAtTheKeyboardIsTakenBack(t *testing.T) {
	f := newFake()
	d, err := opened(f)
	require.NoError(t, err)
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 0xFF}}))
	f.mu.Lock()
	f.Effect = 5
	f.mu.Unlock()
	f.reset()

	require.NoError(t, d.Frame(context.Background(), nil))
	got := f.told(3)
	assert.Equal(t, []string{"08 03 02", "07 03 02", "a8 08 00"}, got[:3], "claimed again, the original not re-read")
	assert.Len(t, got, 3+12+1)
	assert.Equal(t, &setting{effect: 20, brightness: 200}, d.found, "what the keyboard ran before this handle is kept")
}

func TestReleasePutsTheKeyboardsOwnBack(t *testing.T) {
	f := newFake()
	d, err := opened(f)
	require.NoError(t, err)
	require.NoError(t, d.Release(context.Background()))
	assert.Equal(t, []string{"01", "a2", "a8"}, f.told(1), "a handle that never drew sends nothing")

	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 0xFF}}))
	f.reset()
	require.NoError(t, d.Release(context.Background()))
	assert.Equal(t, []string{"07 03 02 14", "07 03 01 c8"}, f.told(4))
	assert.Equal(t, uint8(20), f.Effect)

	f.reset()
	require.NoError(t, d.Close())
	assert.Empty(t, f.Told)
}

func TestAKeyThatIsNotALightSendsNothing(t *testing.T) {
	f := newFake()
	d, err := opened(f)
	require.NoError(t, err)
	f.reset()
	require.Error(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 100}}))
	assert.Empty(t, f.Told)
}

func TestHSVIsAsTheKeyboardTakesIt(t *testing.T) {
	cases := map[colour][3]uint8{
		{0xFF, 0, 0}:       {0, 0xFF, 0xFF},
		{0, 0xFF, 0}:       {85, 0xFF, 0xFF},
		{0, 0, 0xFF}:       {171, 0xFF, 0xFF},
		{0xFF, 0, 0x01}:    {0, 0xFF, 0xFF},
		{0x80, 0x80, 0x80}: {0, 0, 0x80},
		{0, 0, 0}:          {0, 0, 0},
		{0xFF, 0x80, 0x00}: {21, 0xFF, 0xFF},
	}
	for c, want := range cases {
		h, s, v := hsv(c)
		assert.Equal(t, want, [3]uint8{h, s, v}, "%+v", c)
	}
}
