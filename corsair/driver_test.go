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

// The pad's zones are its lights, one each, as OpenRGB names them.
func TestZonesAreLeftRightLogo(t *testing.T) {
	d, err := opened(newFake())
	require.NoError(t, err)
	assert.Equal(t, []lighting.Zone{
		{Name: "Left", First: 0, Count: 1, Shape: lighting.ShapeSingle},
		{Name: "Right", First: 1, Count: 1, Shape: lighting.ShapeSingle},
		{Name: "Logo", First: 2, Count: 1, Shape: lighting.ShapeSingle},
	}, d.Zones())
	require.ErrorIs(t, d.Resize("Left", 2), lighting.ErrNoZone)
}

/*
The lighting endpoint is never left open (spec 036): a frame cut off after
its open still closes it; one left open by another is closed and opened
again; one that stays refused is the frame's error until it is free; and
Close shuts one a frame could not.
*/
func TestAFrameCutOffAfterItsOpenStillCloses(t *testing.T) {
	f := newFake()
	d, err := opened(f)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	f.After = func(cmd []byte) {
		if cmd[1] == cmdOpen {
			cancel()
		}
	}
	require.Error(t, d.Frame(ctx, []lighting.Pixel{{ID: 0, R: 1}}))
	assert.False(t, f.Held, "the endpoint was left open")
	assert.Equal(t, "08 05", f.told(2)[len(f.Told)-1], "the last command was not a close")
}

func TestAnEndpointLeftOpenIsClosedAndOpenedAgain(t *testing.T) {
	f := newFake()
	d, err := opened(f)
	require.NoError(t, err)
	f.Held = true // as a stopped service or another program left it
	f.reset()
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 1}}))
	assert.Equal(t, []string{"08 02", "08 01", "08 0d", "08 05", "08 0d", "08 06", "08 05"}, f.told(2))
	assert.False(t, f.Held)
}

func TestAnEndpointThatStaysRefusedIsAnErrorUntilItIsFree(t *testing.T) {
	f := newFake()
	d, err := opened(f)
	require.NoError(t, err)
	f.Busy = 2
	require.ErrorIs(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 1}}), errStatus)
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 1}}), "the next frame was not sent")
	assert.False(t, f.Held)
}

func TestCloseShutsAnEndpointAFrameCouldNot(t *testing.T) {
	f := newFake()
	d, err := opened(f)
	require.NoError(t, err)
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 1}}))
	f.Refuse = map[byte]byte{cmdClose: 0x09}
	require.Error(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 2}}))
	require.True(t, f.Held, "the fake did not keep the endpoint open")
	f.Refuse = nil
	f.reset()
	require.NoError(t, d.Close())
	assert.Equal(t, []string{"08 05"}, f.told(2))
	assert.False(t, f.Held)
}

// Cut off while its colours are on their way, the frame still closes what
// it opened.
func TestAFrameCutOffInItsColoursStillCloses(t *testing.T) {
	f := newFake()
	d, err := opened(f)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	f.After = func(cmd []byte) {
		if cmd[1] == cmdBlock {
			cancel()
		}
	}
	require.Error(t, d.Frame(ctx, []lighting.Pixel{{ID: 0, R: 1}}))
	assert.False(t, f.Held, "the endpoint was left open")
}
