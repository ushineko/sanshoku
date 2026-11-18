package q565

import (
	"encoding/binary"
	"image"
	"image/color"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rgb is RGB565 with red in the high bits, the order Encode writes.
func rgb(r, g, b uint8) uint16 { return rgb565(r, g, b) }

// decode is Decode, failing the test on a malformed stream. It is written
// from the same description as Encode, so it is not the reference: the
// reference's reading is checked by hand (TestTheReferenceDecodes).
func decode(t *testing.T, data []byte) (w, h int, px []uint16) {
	t.Helper()
	w, h, px, err := Decode(data)
	require.NoError(t, err)
	return w, h, px
}

// The decoder reads DIFF_INDEXED, which the encoder never writes: an entry
// of the array, plus a difference. Here a red pixel through RGB565 fills
// slot of red, and the op reads it back one step greener.
func TestDecodeReadsDiffIndexed(t *testing.T) {
	red := rgb565(248, 0, 0)
	data := []byte("q565\x02\x00\x01\x00")
	data = append(data, opRGB565, byte(red), byte(red>>8))
	// green +1 (bias 4 -> 5), red 0 (bias 2), blue 0 (bias 2), index slot(red)
	data = append(data, 0xA0|5<<2|2, 2<<6|slot(red), opEnd)
	_, _, px, err := Decode(data)
	require.NoError(t, err)
	require.Equal(t, []uint16{red, red + 1<<5}, px)
}

// A stream cut short is an error, not a picture.
func TestDecodeRefusesATruncatedStream(t *testing.T) {
	data := Encode(pictures()["gradient"])
	_, _, _, err := Decode(data[:len(data)/2])
	require.Error(t, err)
}

// want is what an image should decode to.
func want(img image.Image) []uint16 {
	b := img.Bounds()
	out := make([]uint16, 0, b.Dx()*b.Dy())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			out = append(out, rgb(uint8(r>>8), uint8(g>>8), uint8(bl>>8))) //nolint:gosec // high byte
		}
	}
	return out
}

// pictures are the shapes the ops exist for: a flat field (runs), a
// gradient (small differences), noise (raw pixels), and a few colours
// repeated (the array).
func pictures() map[string]*image.RGBA {
	const w, h = 640, 640
	out := map[string]*image.RGBA{}
	mk := func(name string, f func(x, y int) color.RGBA) {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := range h {
			for x := range w {
				img.SetRGBA(x, y, f(x, y))
			}
		}
		out[name] = img
	}
	mk("black", func(int, int) color.RGBA { return color.RGBA{A: 255} })
	mk("gradient", func(x, y int) color.RGBA {
		return color.RGBA{uint8(x * 255 / w), uint8(y * 255 / h), 128, 255} //nolint:gosec // below 256
	})
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // test data
	mk("noise", func(int, int) color.RGBA {
		return color.RGBA{uint8(rng.UintN(256)), uint8(rng.UintN(256)), uint8(rng.UintN(256)), 255} //nolint:gosec // < 256
	})
	palette := []color.RGBA{{230, 40, 40, 255}, {40, 200, 60, 255}, {30, 60, 220, 255}, {240, 240, 240, 255}}
	mk("stripes", func(x, y int) color.RGBA { return palette[(x/3+y/5)%len(palette)] })
	return out
}

func TestEveryPictureComesBackAsItWent(t *testing.T) {
	for name, img := range pictures() {
		t.Run(name, func(t *testing.T) {
			data := Encode(img)
			w, h, px := decode(t, data)
			assert.Equal(t, 640, w)
			assert.Equal(t, 640, h)
			require.Equal(t, want(img), px)
			t.Logf("%s: %d bytes", name, len(data))
		})
	}
}

// CAM's all-black frame was 6616 bytes: the header, 6607 runs, END. The same
// frame from this encoder is the same size.
func TestABlackFrameIsTheSizeCAMSent(t *testing.T) {
	data := Encode(pictures()["black"])
	assert.Len(t, data, 6616)
	assert.Equal(t, byte(opRun|61), data[8], "a run of 62")
}

/*
TestTheReferenceDecodes writes the pictures for the reference decoder, when
Q565_OUT names a directory; the check itself is run by hand:

	Q565_OUT=/tmp/q go test ./internal/q565 -run Reference
	q565-cli decode /tmp/q/gradient.q565 ...
*/
func TestTheReferenceDecodes(t *testing.T) {
	dir := os.Getenv("Q565_OUT")
	if dir == "" {
		t.Skip("Q565_OUT is not set")
	}
	for name, img := range pictures() {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name+".q565"), Encode(img), 0o600))
		// The pixels it should decode to, RGB565 little-endian, row by row.
		px := want(img)
		raw := make([]byte, 2*len(px))
		for i, p := range px {
			binary.LittleEndian.PutUint16(raw[2*i:], p)
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, name+".565"), raw, 0o600))
	}
}

func BenchmarkEncode(b *testing.B) {
	for name, img := range pictures() {
		b.Run(name, func(b *testing.B) {
			for range b.N {
				_ = Encode(img)
			}
		})
	}
}
