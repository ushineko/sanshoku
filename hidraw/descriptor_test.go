package hidraw

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

/*
Two descriptors in the shapes the drivers match against, written by hand from
the HID 1.11 item encoding rather than captured, so they carry no identifier.

g502HIDPP is a Lightspeed receiver's HID++ interface, which a G502 X PLUS is
read through: two vendor collections on page 0xFF00, one carrying the short
report 0x10 and one the long report 0x11.

kraken is the Kraken Elite's control interface, shortened: one vendor
collection on page 0xFF00 whose report IDs are the protocol's command bytes,
output 0x10, 0x70 and 0x74 (the status ask) and input 0x11 and 0x75 (the status
reply). The real one declares about forty. It carries report 0x10 on a vendor
page as a HID++ node does, which is why a HID++ reader matches the Logitech
vendor as well as the descriptor.
*/
var (
	g502HIDPP = []byte{
		0x06, 0x00, 0xff, // Usage Page (0xFF00)
		0x09, 0x01, // Usage (1)
		0xa1, 0x01, // Collection (Application)
		0x85, 0x10, // Report ID (0x10)
		0x95, 0x06, // Report Count (6)
		0x75, 0x08, // Report Size (8)
		0x15, 0x00, // Logical Minimum (0)
		0x26, 0xff, 0x00, // Logical Maximum (255)
		0x09, 0x01, 0x81, 0x00, // Usage (1), Input
		0x09, 0x01, 0x91, 0x00, // Usage (1), Output
		0xc0,             // End Collection
		0x06, 0x00, 0xff, // Usage Page (0xFF00)
		0x09, 0x02, // Usage (2)
		0xa1, 0x01, // Collection (Application)
		0x85, 0x11, // Report ID (0x11)
		0x95, 0x13, // Report Count (19)
		0x75, 0x08, // Report Size (8)
		0x15, 0x00, // Logical Minimum (0)
		0x26, 0xff, 0x00, // Logical Maximum (255)
		0x09, 0x02, 0x81, 0x00, // Usage (2), Input
		0x09, 0x02, 0x91, 0x00, // Usage (2), Output
		0xc0, // End Collection
	}
	kraken = []byte{
		0x06, 0x00, 0xff, // Usage Page (0xFF00)
		0x09, 0x01, // Usage (1)
		0xa1, 0x01, // Collection (Application)
		0x85, 0x10, 0x09, 0x01, 0x15, 0x00, 0x26, 0xff, 0x00, // Report ID (0x10), Usage (1), Logical 0..255
		0x75, 0x08, 0x96, 0xff, 0x01, 0x91, 0x82, // Report Size (8), Report Count (511), Output
		0x85, 0x70, 0x09, 0x01, 0x15, 0x00, 0x26, 0xff, 0x00, // Report ID (0x70)
		0x75, 0x08, 0x96, 0xff, 0x01, 0x91, 0x82, // Output
		0x85, 0x74, 0x09, 0x01, 0x15, 0x00, 0x26, 0xff, 0x00, // Report ID (0x74)
		0x75, 0x08, 0x96, 0xff, 0x01, 0x91, 0x82, // Output
		0x85, 0x11, 0x09, 0x01, 0x75, 0x08, 0x96, 0xff, 0x01, 0x81, 0x82, // Report ID (0x11), Input
		0x85, 0x75, 0x09, 0x01, 0x75, 0x08, 0x96, 0xff, 0x01, 0x81, 0x82, // Report ID (0x75), Input
		0xc0, // End Collection
	}
)

// Walk reads every item, so a reader sees each usage page and report ID a
// descriptor declares and not only its first: the page a HID++ node is chosen
// by, and the report 0x10 that makes it one. The Kraken passes the same test,
// which is what the vendor half of a HID++ match is for.
func TestWalkReportsTheVendorPageAndReportIDs(t *testing.T) {
	cases := []struct {
		name      string
		desc      []byte
		reportIDs []uint32
		shortID   bool
	}{
		{"G502 via Lightspeed", g502HIDPP, []uint32{0x10, 0x11}, true},
		{"Kraken", kraken, []uint32{0x10, 0x70, 0x74, 0x11, 0x75}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var pages, ids []uint32
			Walk(c.desc, func(it Item) bool {
				if it.Type == TypeGlobal && it.Tag == TagUsagePage {
					pages = append(pages, it.Value)
				}
				if it.Type == TypeGlobal && it.Tag == TagReportID {
					ids = append(ids, it.Value)
				}
				return true
			})

			assert.NotEmpty(t, pages)
			for _, p := range pages {
				assert.Equal(t, uint32(0xFF00), p)
			}
			assert.Equal(t, c.reportIDs, ids)
			n := Node{Descriptor: c.desc}
			assert.True(t, UsagePage(0xFF00)(n))
			assert.Equal(t, c.shortID, HasReportID(0xFF00, 0x10)(n))
		})
	}
}
