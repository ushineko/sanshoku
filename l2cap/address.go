package l2cap

import (
	"fmt"
	"strconv"
	"strings"
)

/*
ParseAddress reads a printed Bluetooth address, "A1:B2:C3:D4:E5:F6", into the
order Dial wants, which is the order it is written.

The error does not repeat the input: it may be most of somebody's address.
*/
func ParseAddress(s string) ([6]byte, error) {
	var out [6]byte
	parts := strings.Split(s, ":")
	if len(parts) != len(out) {
		return out, fmt.Errorf("not a bluetooth address: %d fields, not 6", len(parts))
	}
	for i, p := range parts {
		if len(p) != 2 {
			return out, fmt.Errorf("not a bluetooth address: field %d is not two hex digits", i+1)
		}
		v, err := strconv.ParseUint(p, 16, 8)
		if err != nil {
			return out, fmt.Errorf("not a bluetooth address: field %d is not two hex digits", i+1)
		}
		out[i] = byte(v)
	}
	return out, nil
}
