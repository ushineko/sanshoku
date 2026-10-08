//go:build !windows

package hidraw

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

/*
The reports a descriptor declares, one per kind and ID, each with the page and
usage in effect at its first main item and its length in bytes with one for
the ID.

The receiver's two collections give the short report (six data bytes, seven
with the ID) and the long one (nineteen, twenty), in and out. The Kraken's
reports are 511 bytes of data each. The third case is the global stack: a page
pushed and popped around an item must not leak into the report after it, and
an extended usage's own page wins over the global one.
*/
func TestReportsOfReadsEachReportOnce(t *testing.T) {
	stack := []byte{
		0x05, 0x01, // Usage Page (Generic Desktop)
		0x85, 0x01, // Report ID (1)
		0xa4,             // Push
		0x06, 0x00, 0xff, // Usage Page (0xFF00)
		0x75, 0x08, 0x95, 0x02, // Report Size (8), Report Count (2)
		0x09, 0x05, 0xb1, 0x00, // Usage (5), Feature
		0xb4,                   // Pop
		0x75, 0x08, 0x95, 0x01, // Report Size (8), Report Count (1)
		0x0b, 0x42, 0x00, 0x0c, 0x00, // Usage (Consumer 0x42, extended)
		0x81, 0x00, // Input
	}
	cases := []struct {
		name string
		desc []byte
		want []Report
	}{
		{"G502 via Lightspeed", g502HIDPP, []Report{
			{Kind: ReportInput, ID: 0x10, Page: 0xFF00, Usage: 1, Len: 7},
			{Kind: ReportOutput, ID: 0x10, Page: 0xFF00, Usage: 1, Len: 7},
			{Kind: ReportInput, ID: 0x11, Page: 0xFF00, Usage: 2, Len: 20},
			{Kind: ReportOutput, ID: 0x11, Page: 0xFF00, Usage: 2, Len: 20},
		}},
		{"Kraken", kraken, []Report{
			{Kind: ReportOutput, ID: 0x10, Page: 0xFF00, Usage: 1, Len: 512},
			{Kind: ReportOutput, ID: 0x70, Page: 0xFF00, Usage: 1, Len: 512},
			{Kind: ReportOutput, ID: 0x74, Page: 0xFF00, Usage: 1, Len: 512},
			{Kind: ReportInput, ID: 0x11, Page: 0xFF00, Usage: 1, Len: 512},
			{Kind: ReportInput, ID: 0x75, Page: 0xFF00, Usage: 1, Len: 512},
		}},
		{"a pushed page and an extended usage", stack, []Report{
			{Kind: ReportFeature, ID: 1, Page: 0xFF00, Usage: 5, Len: 3},
			{Kind: ReportInput, ID: 1, Page: 0x0C, Usage: 0x42, Len: 2},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, reportsOf(c.desc))
		})
	}
}
