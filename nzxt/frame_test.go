package nzxt

import (
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku/internal/q565"
	"github.com/ushineko/sanshoku/lighting"
	"github.com/ushineko/sanshoku/screen"
)

// withFrames is an open Kraken Elite panel that takes frames.
func withFrames(f *fake, r *recorder) *framePanel {
	return &framePanel{withPanel(f, r)}
}

// solid is an RGBA picture of one colour.
func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

/*
A frame is CAM's sequence: `36 01 00 01 09`, the 20-byte header naming format
09 and the payload's length, the payload as raw pixels blue first, then `36 02`. No bucket
is asked about, reserved or shown.
*/
func TestAFrameIsSentAsCAMSendsIt(t *testing.T) {
	f := newFake()
	r := &recorder{}
	p := withFrames(f, r)

	require.NoError(t, p.Stream(context.Background(), solid(640, 640, color.RGBA{200, 100, 50, 255})))

	assert.Equal(t, []string{"36 01 00", "36 02"}, commands(f))
	assert.Equal(t, []byte{0x36, 0x01, 0x00, 0x01, frameBGR888}, f.Told[0][:5])

	require.Equal(t, []byte{bulkEndpoint, bulkEndpoint}, r.endpoints)
	require.Len(t, r.first, 20)
	assert.Equal(t, header, r.first[:12])
	assert.Equal(t, byte(frameBGR888), r.first[12])
	assert.Equal(t, uint32(640*640*3), binary.LittleEndian.Uint32(r.first[16:]))
	require.Len(t, r.last, 640*640*3)
	assert.Equal(t, []byte{50, 100, 200, 50, 100, 200}, r.last[:6], "blue, green, red, with no alpha")
	assert.Equal(t, []byte{50, 100, 200}, r.last[len(r.last)-3:])
}

// A picture of another size, or not RGBA, arrives at the panel's size, blue first,
// in its place: the top-left of a half-size picture is the top-left quarter.
func TestAFrameOfAnotherSizeOrKindIsFitted(t *testing.T) {
	f := newFake()
	r := &recorder{}
	p := withFrames(f, r)

	pal := color.Palette{color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}}
	img := image.NewPaletted(image.Rect(0, 0, 320, 320), pal)
	for y := range 320 {
		for x := range 320 {
			if x >= 160 {
				img.SetColorIndex(x, y, 1)
			}
		}
	}
	require.NoError(t, p.Stream(context.Background(), img))

	require.Len(t, r.last, 640*640*3)
	at := func(x, y int) []byte { i := (y*640 + x) * 3; return r.last[i : i+3] }
	assert.Equal(t, []byte{0, 0, 255}, at(0, 0), "red, blue first")
	assert.Equal(t, []byte{0, 0, 255}, at(319, 639))
	assert.Equal(t, []byte{255, 0, 0}, at(320, 0), "blue, blue first")
	assert.Equal(t, []byte{255, 0, 0}, at(639, 639))
}

// A refused start or end is the frame's error, and an empty picture is
// refused before anything is written.
func TestARefusedFrameIsAnError(t *testing.T) {
	for _, refuse := range [][]byte{{0x36, 0x01}, {0x36, 0x02}} {
		f := newFake()
		f.Refuse = refuse
		p := withFrames(f, &recorder{})
		require.Error(t, p.Stream(context.Background(), solid(640, 640, color.RGBA{A: 255})), "% x", refuse)
	}

	f := newFake()
	p := withFrames(f, &recorder{})
	require.Error(t, p.Stream(context.Background(), image.NewRGBA(image.Rectangle{})))
	assert.Empty(t, f.Told)
}

/*
After a frame no bucket is on screen. The next image may use the slot that
was showing before the frame, and Close hands the panel back.
*/
func TestAfterAFrameEveryBucketIsFreeAndClosingHandsBack(t *testing.T) {
	f := newFake()
	r := &recorder{}
	p := withFrames(f, r)
	require.NoError(t, p.Image(context.Background(), card(640, 640)))
	require.Equal(t, 0, p.showing)

	require.NoError(t, p.Stream(context.Background(), solid(640, 640, color.RGBA{A: 255})))
	assert.Equal(t, -1, p.showing)

	f.mu.Lock()
	f.Told = nil
	f.mu.Unlock()
	require.NoError(t, p.Image(context.Background(), card(640, 640)))
	assert.Contains(t, commands(f), "32 01 00", "the slot that showed before the frame was not reused")

	require.NoError(t, p.Close())
	last := f.Told[len(f.Told)-1]
	assert.Equal(t, []byte{0x38, 0x01, modeLiquid, 0x00}, last[:4])
}

// The hold is the measured 30 s less a margin.
func TestFrameHoldIsUnderTheFirmwaresReturn(t *testing.T) {
	p := withFrames(newFake(), &recorder{})
	assert.Less(t, p.StreamHold().Seconds(), 30.0)
	assert.Positive(t, p.StreamHold())
}

// The Kraken Elite's panel takes frames; the Kraken 2024 Plus's is a panel
// that does not, until a bench run on one says otherwise. Both are still
// lighting canvases: a Frame method on the screen once hid the canvas's.
func TestOnlyAMeasuredModelTakesFrames(t *testing.T) {
	for product, frames := range map[uint16]bool{0x3012: true, 0x3014: false} {
		n := krakenNode
		n.Product = product
		cand := candidate(n, Driver{}.options(), func(string) (node, error) { return newFake(), nil }, nil)
		dev, err := cand.Open(context.Background())
		require.NoError(t, err)
		_, isPanel := dev.(screen.Panel)
		_, isCanvas := dev.(lighting.Canvas)
		assert.True(t, isCanvas, "%04x: taking frames hid the lights", product)
		_, isStreamer := dev.(screen.Streamer)
		_, isMover := dev.(screen.Mover)
		assert.True(t, isPanel, "%04x", product)
		assert.Equal(t, frames, isStreamer, "%04x", product)
		assert.Equal(t, frames, isMover, "%04x", product)
	}
}

/*
A Move is CAM's Q565 frame (spec 038): `36 01 00 01 08`, the 20-byte header
naming format 08 and the payload's length, a Q565 payload of the panel's size
that decodes to the picture, then `36 02`. Red stays in the high bits.
*/
func TestAMoveIsAQ565Frame(t *testing.T) {
	f := newFake()
	r := &recorder{}
	p := withFrames(f, r)

	require.NoError(t, p.Move(context.Background(), solid(640, 640, color.RGBA{255, 0, 0, 255})))

	assert.Equal(t, []string{"36 01 00", "36 02"}, commands(f))
	assert.Equal(t, []byte{0x36, 0x01, 0x00, 0x01, frameQ565}, f.Told[0][:5])
	require.Len(t, r.first, 20)
	assert.Equal(t, header, r.first[:12])
	assert.Equal(t, byte(frameQ565), r.first[12])
	assert.Equal(t, uint32(len(r.last)), binary.LittleEndian.Uint32(r.first[16:])) //nolint:gosec // a frame's size

	w, h, px, err := q565.Decode(r.last)
	require.NoError(t, err)
	assert.Equal(t, [2]int{640, 640}, [2]int{w, h})
	require.Len(t, px, 640*640)
	assert.Equal(t, uint16(0xF800), px[0], "red, in the high bits")
	assert.Equal(t, uint16(0xF800), px[len(px)-1])
}

// A smaller picture is scaled to the panel before it is encoded.
func TestAMoveOfAnotherSizeIsFitted(t *testing.T) {
	r := &recorder{}
	p := withFrames(newFake(), r)
	require.NoError(t, p.Move(context.Background(), solid(320, 320, color.RGBA{0, 0, 255, 255})))

	w, h, px, err := q565.Decode(r.last)
	require.NoError(t, err)
	assert.Equal(t, [2]int{640, 640}, [2]int{w, h})
	assert.Equal(t, uint16(0x001F), px[len(px)-1], "blue, in the low bits")
}

// Raw and Q565 frames follow each other in any order, each its own complete
// exchange, and a refused Q565 frame is an error like a raw one.
func TestRawAndQ565FramesAlternate(t *testing.T) {
	f := newFake()
	r := &recorder{}
	p := withFrames(f, r)
	ctx := context.Background()
	require.NoError(t, p.Stream(ctx, solid(640, 640, color.RGBA{A: 255})))
	require.NoError(t, p.Move(ctx, solid(640, 640, color.RGBA{A: 255})))
	require.NoError(t, p.Stream(ctx, solid(640, 640, color.RGBA{A: 255})))

	var formats []byte
	for _, t := range f.Told {
		if t[0] == 0x36 && t[1] == 0x01 {
			formats = append(formats, t[4])
		}
	}
	assert.Equal(t, []byte{frameBGR888, frameQ565, frameBGR888}, formats)
	assert.Equal(t, []string{"36 01 00", "36 02", "36 01 00", "36 02", "36 01 00", "36 02"}, commands(f))

	refused := newFake()
	refused.Refuse = []byte{0x36, 0x02}
	require.Error(t, withFrames(refused, &recorder{}).Move(ctx, solid(640, 640, color.RGBA{A: 255})))
	require.Error(t, p.Move(ctx, image.NewRGBA(image.Rectangle{})))
}
