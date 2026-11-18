/*
Package q565 encodes pictures in Q565, the format NZXT CAM streams to the
Kraken Elite's panel as format `08` (spec 038, issue #103).

Q565 is QOI adapted to 16-bit RGB565 pixels. It was designed by seritools,
whose reference implementation (https://github.com/seritools/q565, MIT or
Apache-2.0) describes it; this is written from that description. A stream is:

	"q565", width u16le, height u16le
	ops, from pixel 0, starting from black with an all-black colour array
	FF

The ops, by tag:

	00iiiiii              INDEX: colour array entry i
	01rrggbb              DIFF: the last pixel plus r, g, b in -2..1 (bias 2)
	100ggggg rrrrbbbb     LUMA: green -16..15 (bias 16); red and blue minus
	                      green, -8..7 (bias 8)
	101gggrr bbiiiiii     DIFF_INDEXED: array entry i plus green -4..3,
	                      red -2..1, blue -2..1
	11nnnnnn              RUN: the last pixel n+1 more times, n+1 in 1..62
	FE lo hi              RGB565: a pixel, little-endian, red in the high bits
	FF                    END

Differences wrap within each channel's bits. A pixel from LUMA, DIFF_INDEXED
or RGB565 goes into the colour array at (low byte + high byte) & 63; INDEX
and DIFF pixels do not.

**This encoder never writes DIFF_INDEXED.** Finding a near colour means
comparing against all 64 entries, which made a 640x640 frame of noise take
55 ms against 5 ms without it, for 3.7% fewer bytes; the device shows a frame
every 44 ms. The reference marks the op as one an encoder may leave out.
*/
package q565

import (
	"encoding/binary"
	"image"
)

// Ops, by their tags. DIFF_INDEXED (0xA0) is decoded by the device and never
// written here.
const (
	opIndex  = 0x00
	opDiff   = 0x40
	opLuma   = 0x80
	opRun    = 0xC0
	opRGB565 = 0xFE
	opEnd    = 0xFF

	maxRun = 62
)

// slot is where a pixel lives in the colour array.
func slot(p uint16) uint8 { return (uint8(p) + uint8(p>>8)) & 63 } //nolint:gosec // the two bytes, by design

// rgb565 packs a colour, red in the high bits, as the Kraken Elite takes it
// (spec 038: a red, green and blue card read in that order).
func rgb565(r, g, b uint8) uint16 {
	return uint16(r>>3)<<11 | uint16(g>>2)<<5 | uint16(b>>3)
}

// channels splits a pixel into red (5 bits), green (6) and blue (5).
func channels(p uint16) (r, g, b int) { return int(p >> 11 & 31), int(p >> 5 & 63), int(p & 31) }

// diff is a - b wrapped into a signed n-bit value.
func diff(a, b, n int) int {
	d := (a - b) & (1<<n - 1)
	if d >= 1<<(n-1) {
		d -= 1 << n
	}
	return d
}

/*
Encode writes img as a Q565 stream at its own size.

Each pixel gets the shortest op that fits, in QOI's order of preference: a
run, then the colour array, then a one-byte difference, then a two-byte one,
then the pixel itself. An *image.RGBA is read from its bytes, because At
allocates a colour per pixel: 400000 a frame.
*/
func Encode(img image.Image) []byte {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	out := make([]byte, 8, 8+w*h/4)
	copy(out, "q565")
	binary.LittleEndian.PutUint16(out[4:], uint16(w)) //nolint:gosec // a panel is far below 65536
	binary.LittleEndian.PutUint16(out[6:], uint16(h)) //nolint:gosec // as above

	var arr [64]uint16
	prev, run := uint16(0), 0
	flush := func() {
		if run > 0 {
			out = append(out, opRun|byte(run-1)) //nolint:gosec // run is 1..62
			run = 0
		}
	}
	rgba, direct := img.(*image.RGBA)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			var p uint16
			if direct {
				o := rgba.PixOffset(x, y)
				p = rgb565(rgba.Pix[o], rgba.Pix[o+1], rgba.Pix[o+2])
			} else {
				r, g, b, _ := img.At(x, y).RGBA()
				p = rgb565(uint8(r>>8), uint8(g>>8), uint8(b>>8)) //nolint:gosec // 16-bit colour, high byte
			}
			if p == prev {
				run++
				if run == maxRun {
					flush()
				}
				continue
			}
			flush()
			out = encodeOne(out, &arr, prev, p)
			prev = p
		}
	}
	flush()
	return append(out, opEnd)
}

// encodeOne writes the shortest op for p after prev, and keeps the array.
func encodeOne(out []byte, arr *[64]uint16, prev, p uint16) []byte {
	i := slot(p)
	if arr[i] == p {
		return append(out, opIndex|i)
	}
	pr, pg, pb := channels(prev)
	r, g, b := channels(p)
	dr, dg, db := diff(r, pr, 5), diff(g, pg, 6), diff(b, pb, 5)
	if dr >= -2 && dr <= 1 && dg >= -2 && dg <= 1 && db >= -2 && db <= 1 {
		return append(out, opDiff|byte(dr+2)<<4|byte(dg+2)<<2|byte(db+2)) //nolint:gosec // each 0..3
	}
	arr[i] = p
	if rg, bg := dr-dg, db-dg; dg >= -16 && dg <= 15 && rg >= -8 && rg <= 7 && bg >= -8 && bg <= 7 {
		return append(out, opLuma|byte(dg+16), byte(rg+8)<<4|byte(bg+8)) //nolint:gosec // within the fields
	}
	return append(out, opRGB565, byte(p), byte(p>>8)) //nolint:gosec // the two bytes, little-endian
}
