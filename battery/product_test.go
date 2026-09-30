package battery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProductDropsTheVendorWord(t *testing.T) {
	cases := []struct {
		vendor, name, want string
	}{
		{"SteelSeries", "SteelSeries Arctis Nova Pro Wireless", "Arctis Nova Pro Wireless"},
		{"Logitech", "Logitech K800", "K800"},
		{"Razer", "Razer Basilisk Ultimate Dongle", "Basilisk Ultimate Dongle"},
		{"Logitech", "G502 X PLUS", "G502 X PLUS"},
		{"Logitech", "Logitech", "Logitech"},
		{"NZXT", "NZXT, Inc. Kraken Elite V2", "Kraken Elite V2"},
		{"steelseries", "SteelSeries Arctis 7", "Arctis 7"},
		{"NZXT", "NZXT, Inc.", "NZXT, Inc."},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, Product(c.vendor, c.name), "Product(%q, %q)", c.vendor, c.name)
	}
}
