package hidraw

// The item type and the global item tags a report-descriptor reader needs.
// From the HID 1.11 specification, section 6.2.2.
const (
	// TypeGlobal is the item type of Usage Page and Report ID.
	TypeGlobal = 1
	// TagUsagePage is the global item that sets the usage page.
	TagUsagePage = 0x0
	// TagReportID is the global item that declares a report ID.
	TagReportID = 0x8
)

// Item is one short item of a report descriptor: its type (main 0, global 1,
// local 2), its tag, and its data read little-endian.
type Item struct {
	Type  byte
	Tag   byte
	Value uint32
}

/*
Walk reads a report descriptor item by item, calling visit for each until it
returns false.

**Items, not bytes.** `06 00 ff` is a usage page here and could be the tail of
a longer item's data there, and a byte search cannot tell the two apart. The
whole descriptor is walked, not only its first item: a Razer dock declares its
vendor collection behind a mouse, a keyboard and a consumer one. A truncated
item ends the walk: whatever such a descriptor is, it is not one to draw
conclusions from.
*/
func Walk(desc []byte, visit func(item Item) bool) {
	for i := 0; i < len(desc); {
		prefix := desc[i]
		size := int(prefix & 0x03)
		if size == 3 {
			size = 4
		}
		if i+1+size > len(desc) {
			return
		}
		item := Item{Type: (prefix >> 2) & 0x03, Tag: prefix >> 4, Value: value(desc[i+1 : i+1+size])}
		if !visit(item) {
			return
		}
		i += 1 + size
	}
}

// value reads an item's data, which is little-endian and one, two or four
// bytes wide, or absent.
func value(b []byte) uint32 {
	var v uint32
	for i := len(b) - 1; i >= 0; i-- {
		v = v<<8 | uint32(b[i])
	}
	return v
}

// UsagePage is a predicate for Nodes: the node's descriptor declares the
// given usage page anywhere in it. It is how a vendor's control endpoint is
// told from its keyboard and mouse ones.
func UsagePage(want uint16) func(Node) bool {
	return func(n Node) bool {
		found := false
		Walk(n.Descriptor, func(it Item) bool {
			if it.Type == TypeGlobal && it.Tag == TagUsagePage && it.Value == uint32(want) {
				found = true
				return false
			}
			return true
		})
		return found
	}
}

/*
HasReportID is a predicate for Nodes: the descriptor declares report id while
the current usage page is page or above.

It is hayami's test for a HID++ endpoint, HasReportID(0xFF00, 0x10): a
vendor-defined usage page (0xFF00 and above is the vendor range) with report
0x10 declared within it. A Logitech receiver presents three nodes and only
that one answers a request.
*/
func HasReportID(page uint16, id byte) func(Node) bool {
	return func(n Node) bool {
		onPage, found := false, false
		Walk(n.Descriptor, func(it Item) bool {
			if it.Type != TypeGlobal {
				return true
			}
			switch it.Tag {
			case TagUsagePage:
				onPage = it.Value >= uint32(page)
			case TagReportID:
				if onPage && it.Value == uint32(id) {
					found = true
					return false
				}
			}
			return true
		})
		return found
	}
}
