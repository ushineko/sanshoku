package aula

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku/lighting"
)

// lit is a frame with px drawn on a dark board.
func lit(t *testing.T, px ...lighting.Pixel) *frame {
	t.Helper()
	f, err := merged(frame{}, px)
	require.NoError(t, err)
	return &f
}

// one is a pixel of one colour on each slot.
func one(c colour, slots ...byte) []lighting.Pixel {
	px := make([]lighting.Pixel, 0, len(slots))
	for _, s := range slots {
		px = append(px, lighting.Pixel{ID: s, R: c[0], G: c[1], B: c[2]})
	}
	return px
}

var (
	red   = colour{0xff, 0, 0}
	green = colour{0, 0xff, 0}
	blue  = colour{0, 0, 0xff}
)

// The whole-board packet the vendor's program sent for red, from the capture
// in OpenRGB issue 5166, and seen to turn the F75 red.
func TestThePacketIsTheCapturedOne(t *testing.T) {
	want := []byte{0x13, 0x88, 0x01, 0x00, 0x23, 0xff, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xbe}

	assert.Equal(t, want, packet(commandFrame, 1, 0, modeBoard, red[:]))
}

/*
The frame that was watched on the F75: W, A, S and D red, U blue, Escape and
the backtick green. Groups go in the order of each colour's first slot, so
green (slots 0 and 1) leads, and the 19 bytes take two packets.
*/
func TestAFrameIsGroupsInFirstSlotOrder(t *testing.T) {
	px := one(red, 14, 9, 15, 21)
	px = append(px, one(blue, 44)...)
	px = append(px, one(green, 0, 1)...)

	got := receiverFrame(lit(t, px...))

	require.Len(t, got, 2)
	assert.Equal(t, []byte{
		0x13, 0x88, 0x02, 0x00, 0x1e,
		0x00, 0xff, 0x00, 0x02, 0x00, 0x01,
		0xff, 0x00, 0x00, 0x04, 0x09, 0x0e, 0x0f, 0x15,
		0xfb,
	}, got[0])
	assert.Equal(t, []byte{
		0x13, 0x88, 0x02, 0x01, 0x15,
		0x00, 0x00, 0xff, 0x01, 0x2c,
		0, 0, 0, 0, 0, 0, 0, 0, 0,
		0xdf,
	}, got[1])
}

// A dark board is the one-colour mode with black: a message naming no key
// would have no body at all.
func TestADarkBoardIsOneBlackPacket(t *testing.T) {
	got := receiverFrame(&frame{})

	require.Len(t, got, 1)
	assert.Equal(t, packet(commandFrame, 1, 0, modeBoard, black[:]), got[0])
}

// A black light is left out of the message, which is how the keyboard shows
// black.
func TestABlackLightIsNotNamed(t *testing.T) {
	px := one(red, 0, 1)
	px = append(px, one(black, 1)...)

	gs := groups(lit(t, px...))

	require.Len(t, gs, 1)
	assert.Equal(t, []byte{0}, gs[0].slots)
}

// every is a frame with a colour of its own on every key.
func every(t *testing.T) *frame {
	t.Helper()
	var px []lighting.Pixel
	for i, k := range f75Keys {
		px = append(px, lighting.Pixel{ID: k.ID, R: uint8(3 * i), G: uint8(255 - 3*i), B: uint8(7 * i)}) //nolint:gosec // under 256 by the key count
	}
	return lit(t, px...)
}

/*
A frame with a colour for every key does not fit one message, and a second
message would replace the first. It comes out with its nearest colours merged:
within the limit, every key still lit, each key once.
*/
func TestAFrameOverTheLimitIsReducedAndEveryKeyStaysLit(t *testing.T) {
	gs := reduce(groups(every(t)), messageLimit)

	assert.LessOrEqual(t, size(gs), messageLimit)
	assert.LessOrEqual(t, len(gs), ReceiverColours)
	seen := map[byte]int{}
	for _, g := range gs {
		for _, s := range g.slots {
			seen[s]++
		}
	}
	assert.Len(t, seen, keyCount)
	for s, n := range seen {
		assert.Equal(t, 1, n, "slot %d", s)
	}
}

// As many colours as a message holds are shown as they are.
func TestAFrameWithinTheLimitIsNotTouched(t *testing.T) {
	var px []lighting.Pixel
	for i, k := range f75Keys {
		c := i % ReceiverColours
		px = append(px, lighting.Pixel{ID: k.ID, R: uint8(6 * c), G: 1, B: uint8(255 - 6*c)}) //nolint:gosec // under 256
	}
	in := groups(lit(t, px...))
	require.Len(t, in, ReceiverColours)

	assert.Equal(t, in, reduce(in, messageLimit))
}

// The same frame is the same bytes, every time.
func TestReductionIsTheSameEveryTime(t *testing.T) {
	assert.Equal(t, receiverFrame(every(t)), receiverFrame(every(t)))
}

// Two colours merged are their mean by how many keys each lights.
func TestMergedColoursAreWeightedByTheirKeys(t *testing.T) {
	a := group{c: colour{90, 0, 0}, slots: []byte{9, 14, 15}}
	b := group{c: colour{10, 0, 40}, slots: []byte{0}}

	got := merge(a, b)

	assert.Equal(t, colour{70, 0, 10}, got.c)
	assert.Equal(t, []byte{0, 9, 14, 15}, got.slots)
}

/*
The wired frame: report 6, command 0x08, one packet, 378 bytes of data
(0x017A, low byte first), then a colour at three times each slot. A slot with
no key stays black.
*/
func TestAWiredFrameHasAColourForEverySlot(t *testing.T) {
	got := wiredFrame(lit(t, one(red, 14)...))

	require.Len(t, got, wiredLen)
	assert.Equal(t, []byte{0x06, 0x08, 0x00, 0x00, 0x01, 0x00, 0x7a, 0x01}, got[:wiredHeader])
	assert.Equal(t, red[:], got[wiredHeader+3*14:wiredHeader+3*14+3])
	lit := 0
	for _, b := range got[wiredHeader:] {
		if b != 0 {
			lit++
		}
	}
	assert.Equal(t, 1, lit)
}

// The key table is in slot order, names every key once, and leaves out the
// slots this board has no key on.
func TestTheKeyTableIsInSlotOrder(t *testing.T) {
	names := map[string]bool{}
	for i, k := range f75Keys {
		if i > 0 {
			assert.Greater(t, k.ID, f75Keys[i-1].ID)
		}
		assert.NotEmpty(t, k.Name)
		assert.False(t, names[k.Name], "%q is named twice", k.Name)
		names[k.Name] = true
	}
	for _, s := range []byte{6, 75, 76, 84} {
		assert.False(t, isKey[s], "slot %d", s)
	}
	assert.Equal(t, 37, ReceiverColours)
}
