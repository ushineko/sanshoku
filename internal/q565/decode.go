package q565

import (
	"encoding/binary"
	"errors"
)

// errTruncated is a stream that ends before its END op.
var errTruncated = errors.New("q565: the stream ends before its end marker")

/*
Decode reads a Q565 stream into RGB565 pixels, row by row. It reads every op,
DIFF_INDEXED included, and exists so tests can check what an encoder wrote:
the device is the decoder that matters.
*/
func Decode(data []byte) (w, h int, px []uint16, err error) {
	if len(data) < 9 || string(data[:4]) != "q565" {
		return 0, 0, nil, errors.New("q565: not a Q565 stream")
	}
	w, h = int(binary.LittleEndian.Uint16(data[4:])), int(binary.LittleEndian.Uint16(data[6:]))
	px = make([]uint16, 0, w*h)
	var arr [64]uint16
	prev := uint16(0)
	apply := func(p uint16, dr, dg, db int) uint16 {
		r, g, b := channels(p)
		return uint16((r+dr)&31)<<11 | uint16((g+dg)&63)<<5 | uint16((b+db)&31) //nolint:gosec // masked
	}
	for i := 8; i < len(data); {
		op := data[i]
		i++
		switch {
		case op == opEnd:
			if i != len(data) {
				return w, h, px, errors.New("q565: bytes after the end marker")
			}
			return w, h, px, nil
		case op == opRGB565:
			if i+2 > len(data) {
				return w, h, px, errTruncated
			}
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
		default: // LUMA, or DIFF_INDEXED: two bytes
			if i >= len(data) {
				return w, h, px, errTruncated
			}
			c := data[i]
			i++
			if op&0xE0 == opLuma {
				dg := int(op&31) - 16
				prev = apply(prev, int(c>>4)-8+dg, dg, int(c&15)-8+dg)
			} else {
				prev = apply(arr[c&63], int(op&3)-2, int(op>>2&7)-4, int(c>>6)-2)
			}
			arr[slot(prev)] = prev
		}
		px = append(px, prev)
	}
	return w, h, px, errTruncated
}
