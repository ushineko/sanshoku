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

// rgb is RGB565 with red in the high bits, the order Encode is tested in.
func rgb(r, g, b uint8) uint16 { return RGB565(r, g, b) }

// decode is a reader of the format written from the same description, for
// round trips. It is not the reference: see TestTheReferenceDecodes.
func decode(t *testing.T, data []byte) (w, h int, px []uint16) {
	t.Helper()
	require.Equal(t, "q565", string(data[:4]))
	w, h = int(binary.LittleEndian.Uint16(data[4:])), int(binary.LittleEndian.Uint16(data[6:]))
	var arr [64]uint16
	prev := uint16(0)
	apply := func(p uint16, dr, dg, db int) uint16 {
		r, g, b := channels(p)
		return uint16((r+dr)&31)<<11 | uint16((g+dg)&63)<<5 | uint16((b+db)&31) //nolint:gosec // masked
	}
	for i := 8; ; {
		require.Less(t, i, len(data), "no END")
		op := data[i]
		i++
		switch {
		case op == opEnd:
			require.Equal(t, len(data), i, "bytes after END")
			return w, h, px
		case op == opRGB565:
			prev = binary.LittleEndian.Uint16(data[i:])
			i += 2
			arr[slot(prev)] = prev
		case op>>6 == 3:
			for range int(op&63) + 1 {
				px = append(px, prev)
			}
			continue
		case op>>6 == 0:
			prev = arr[op]
		case op>>6 == 1:
			prev = apply(prev, int(op>>4&3)-2, int(op>>2&3)-2, int(op&3)-2)
		case op&0xE0 == opLuma:
			dg := int(op&31) - 16
			c := data[i]
			i++
			prev = apply(prev, int(c>>4)-8+dg, dg, int(c&15)-8+dg)
			arr[slot(prev)] = prev
		default: // DIFF_INDEXED
			c := data[i]
			i++
			prev = apply(arr[c&63], int(op&3)-2, int(op>>2&7)-4, int(c>>6)-2)
			arr[slot(prev)] = prev
		}
		px = append(px, prev)
	}
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
			data := Encode(img, rgb)
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
	data := Encode(pictures()["black"], rgb)
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
		require.NoError(t, os.WriteFile(filepath.Join(dir, name+".q565"), Encode(img, rgb), 0o600))
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
				_ = Encode(img, rgb)
			}
		})
	}
}
