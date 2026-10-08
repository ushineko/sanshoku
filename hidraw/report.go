package hidraw

// ReportKind is which way a report travels.
type ReportKind byte

// The three kinds of report a HID device declares, as the HID 1.11 main items
// name them.
const (
	// ReportInput is a report the device sends.
	ReportInput ReportKind = iota + 1
	// ReportOutput is a report the host sends with a write.
	ReportOutput
	// ReportFeature is a report exchanged with a feature request.
	ReportFeature
)

// String names a kind for a test's message and the testbench.
func (k ReportKind) String() string {
	switch k {
	case ReportInput:
		return "input"
	case ReportOutput:
		return "output"
	case ReportFeature:
		return "feature"
	default:
		return "unknown"
	}
}

/*
Report is one report a node declares: which way it goes, its number, the usage
it carries and how long it is.

It is what a predicate reads where there is no report descriptor to walk.
Windows hands a program no descriptor at all, only what its HID parser made of
one, per top-level collection (spec 012); Linux has the descriptor and fills
Reports from it as well, so a driver can read either the same way.

Len is in bytes with one for the report ID, whether or not the device numbers
its reports, which is how Windows counts and how a write is framed on both.
Where Windows describes a collection, Len is that collection's length for the
kind, which is its longest report of that kind.
*/
type Report struct {
	Kind  ReportKind
	ID    byte
	Page  uint16
	Usage uint16
	Len   int
}

// UsagePage is a predicate for Nodes: the node declares the given usage page
// anywhere in it. It is how a vendor's control endpoint is told from its
// keyboard and mouse ones.
//
// The descriptor is walked where the node has one, which on Linux it always
// does, so a Linux match is the item-by-item one it has always been. A node
// with no descriptor (Windows) is read from its Reports.
func UsagePage(want uint16) func(Node) bool {
	return func(n Node) bool {
		if len(n.Descriptor) == 0 {
			for _, r := range n.Reports {
				if r.Page == want {
					return true
				}
			}
			return false
		}
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
HasReportID is a predicate for Nodes: the node declares report id while the
current usage page is page or above.

It is hayami's test for a HID++ endpoint, HasReportID(0xFF00, 0x10): a
vendor-defined usage page (0xFF00 and above is the vendor range) with report
0x10 declared within it. A Logitech receiver presents three nodes and only
that one answers a request.

As with UsagePage, the descriptor is walked where there is one, and a node
without one is read from its Reports: a report numbered id on a page at or
above page.
*/
func HasReportID(page uint16, id byte) func(Node) bool {
	return func(n Node) bool {
		if len(n.Descriptor) == 0 {
			for _, r := range n.Reports {
				if r.ID == id && r.Page >= page {
					return true
				}
			}
			return false
		}
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
