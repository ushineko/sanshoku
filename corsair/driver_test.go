package corsair

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/hidraw"
	"github.com/ushineko/sanshoku/lighting"
)

// mm700Node is the pad's lighting interface as Nodes reports it, under a
// made-up path.
var mm700Node = hidraw.Node{Vendor: vendor, Product: 0x1B9B, Name: "Corsair CORSAIR MM700 RGB Gaming Mousepad", Path: "/dev/hidraw-test1"}

// A freshly enumerated pad does not answer at once: the product is asked
// again until it does (R1.2).
func TestOpenAsksTheProductUntilItIsAnswered(t *testing.T) {
	f := newFake()
	f.Silent = 3
	d, err := opened(f)
	require.NoError(t, err)
	assert.Equal(t, "Corsair MM700", d.Identity().Name)
	assert.Len(t, f.told(3), 4, "three unanswered questions and one answered")

	silent := newFake()
	silent.Silent = productTries
	_, err = opened(silent)
	require.ErrorIs(t, err, sanshoku.ErrAbsent)
	assert.True(t, silent.closed, "a pad that never answered was left open")

	other := newFake()
	other.Product = 0x1B9C
	_, err = opened(other)
	require.ErrorIs(t, err, sanshoku.ErrUnsupported)
}

func TestAProductOffTheListIsNeverOpened(t *testing.T) {
	n := mm700Node
	n.Product = 0x1B00
	c := candidate(n, func(string) (node, error) { t.Fatal("opened"); return nil, nil }, 0)
	_, err := c.Open(context.Background())
	require.ErrorIs(t, err, sanshoku.ErrUnsupported)
}

/*
A frame from hardware mode: the mode is read, software mode claimed, and the
frame is open, one planar block, close (R2.2, R2.3).
*/
func TestAFrameClaimsSoftwareModeAndWritesPlanar(t *testing.T) {
	f := newFake()
	d, err := opened(f)
	require.NoError(t, err)
	f.reset()

	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{
		{ID: 0, R: 0xFF}, {ID: 1, G: 0xFF}, {ID: 2, B: 0xFF},
	}))
	assert.Equal(t, []string{
		"08 02 03 00 00", // read the mode
		"08 01 03 00 02", // software mode
		"08 0d 00 01 00", // open endpoint 01
		"08 06 00 09 00", // a block of 9 bytes
		"08 05 01 00 00", // close
	}, f.told(5))
	assert.Equal(t, []byte{0xFF, 0, 0, 0, 0xFF, 0, 0, 0, 0xFF}, f.Told[3][7:16], "reds, greens, blues")

	f.reset()
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 2, R: 0x10}}))
	assert.Equal(t, []string{"08 02 03", "08 0d 00", "08 06 00", "08 05 01"}, f.told(3), "already in software mode: no claim")
	assert.Equal(t, []byte{0xFF, 0, 0x10, 0, 0xFF, 0, 0, 0, 0}, f.Told[2][7:16], "the held frame, light 2 now dim red")

	f.reset()
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 2, R: 0x10}}))
	assert.Empty(t, f.Told, "an unchanged frame was sent")
}

func TestAKeyThatIsNotALightSendsNothing(t *testing.T) {
	f := newFake()
	d, err := opened(f)
	require.NoError(t, err)
	f.reset()
	require.Error(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 1}, {ID: 3, R: 1}}))
	assert.Empty(t, f.Told)
}

// A refused command is an error, and the frame is sent again next time.
func TestARefusedStepIsAnErrorAndTheFrameIsResent(t *testing.T) {
	f := newFake()
	d, err := opened(f)
	require.NoError(t, err)
	f.Refuse = map[byte]byte{cmdOpen: 0x09}
	require.ErrorIs(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 1}}), errStatus)

	f.Refuse = nil
	f.reset()
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 1}}))
	assert.Contains(t, f.told(2), "08 06", "the frame that failed was not sent again")
}

func TestReleaseReturnsTheStoredEffectAndCloseSendsNothing(t *testing.T) {
	f := newFake()
	d, err := opened(f)
	require.NoError(t, err)
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 1}}))
	f.reset()

	require.NoError(t, d.Release(context.Background()))
	assert.Equal(t, []string{"08 01 03 00 01"}, f.told(5))
	assert.Equal(t, uint32(modeHardware), f.Mode)

	f.reset()
	require.NoError(t, d.Close())
	assert.Empty(t, f.Told, "close wrote to the pad")
	assert.True(t, f.closed)
}

func TestKeysAreLeftRightLogo(t *testing.T) {
	d, err := opened(newFake())
	require.NoError(t, err)
	assert.Equal(t, []lighting.Key{{ID: 0, Name: "Left"}, {ID: 1, Name: "Right"}, {ID: 2, Name: "Logo"}}, d.Keys())
	assert.Equal(t, []string{"lighting"}, sanshoku.Capabilities(d))
}
