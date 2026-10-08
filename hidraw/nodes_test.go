package hidraw

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A paired child's index is the number after the colon, and only 1 to 6 is
// one: a receiver numbers no device 0 or 7.
func TestPairedIndexIsTheNumberAfterTheColon(t *testing.T) {
	for _, tc := range []struct {
		phys  string
		index byte
		ok    bool
	}{
		{"usb-0000:03:00.0-3/input2:1", 1, true},
		{"usb-0000:03:00.0-3/input2:6", 6, true},
		{"usb-0000:03:00.0-3/input2", 0, false},
		{"usb-0000:03:00.0-3/input2:0", 0, false},
		{"usb-0000:03:00.0-3/input2:7", 0, false},
		{"usb-0000:03:00.0-3/input2:x", 0, false},
	} {
		index, ok := PairedIndex(tc.phys)
		assert.Equal(t, tc.ok, ok, tc.phys)
		assert.Equal(t, tc.index, index, tc.phys)
		assert.Equal(t, tc.ok, PairedChild(tc.phys), tc.phys)
	}
}

// A manufacturer string with a corporate suffix doubles the vendor the same
// way a bare one does, and reads the same once undoubled.
func TestUndoubleDropsACorporateSuffixWithTheVendor(t *testing.T) {
	cases := map[string]string{
		"Razer Razer Mouse Dock Pro":      "Razer Mouse Dock Pro",
		"NZXT, Inc. NZXT Kraken Elite V2": "NZXT Kraken Elite V2",
		"SteelSeries Apex Pro TKL":        "SteelSeries Apex Pro TKL",
		"Logitech USB Receiver":           "Logitech USB Receiver",
		"NZXT":                            "NZXT",
	}
	for in, want := range cases {
		assert.Equal(t, want, undouble(in), in)
	}
}
