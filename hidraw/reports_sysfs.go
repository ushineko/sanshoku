//go:build !windows

package hidraw

// The items reportsOf reads beyond the two the predicates need, from the HID
// 1.11 specification, section 6.2.2.
const (
	typeMain  = 0
	typeLocal = 2

	tagInput      = 0x8
	tagOutput     = 0x9
	tagCollection = 0xA
	tagFeature    = 0xB

	tagReportSize  = 0x7
	tagReportCount = 0x9
	tagPush        = 0xA
	tagPop         = 0xB

	tagUsage    = 0x0
	tagUsageMin = 0x1
)

// globals is the global item state a descriptor carries forward, which Push
// and Pop save and restore whole.
type globals struct {
	page        uint16
	id          byte
	size, count uint32
}

/*
reportsOf reads the reports a descriptor declares: one per kind and report ID,
in the order the descriptor first mentions them.

Each report's page and usage are those in effect at its first main item, which
is what a predicate asks about; its length adds up every main item of that kind
and ID, in bits, rounded up to bytes, with one byte for the ID whether or not
the device numbers its reports, as Report.Len is defined. A usage given in its
four-byte extended form carries its own page in the high half, and that page
wins over the global one, as the specification says it does.
*/
func reportsOf(desc []byte) []Report {
	type key struct {
		kind ReportKind
		id   byte
	}
	var (
		g     globals
		stack []globals
		usage uint16
		page  uint16
		local bool
		order []key
		seen  = map[key]*Report{}
		bits  = map[key]uint32{}
	)
	Walk(desc, func(it Item) bool {
		switch it.Type {
		case TypeGlobal:
			switch it.Tag {
			case TagUsagePage:
				g.page = uint16(it.Value) //nolint:gosec // a usage page is sixteen bits
			case TagReportID:
				g.id = byte(it.Value) //nolint:gosec // a report ID is eight bits
			case tagReportSize:
				g.size = it.Value
			case tagReportCount:
				g.count = it.Value
			case tagPush:
				stack = append(stack, g)
			case tagPop:
				if len(stack) > 0 {
					g, stack = stack[len(stack)-1], stack[:len(stack)-1]
				}
			}
		case typeLocal:
			if (it.Tag == tagUsage || it.Tag == tagUsageMin) && !local {
				usage, page, local = uint16(it.Value), uint16(it.Value>>16), true //nolint:gosec // the halves of an extended usage
			}
		case typeMain:
			var kind ReportKind
			switch it.Tag {
			case tagInput:
				kind = ReportInput
			case tagOutput:
				kind = ReportOutput
			case tagFeature:
				kind = ReportFeature
			}
			if kind != 0 {
				k := key{kind, g.id}
				if seen[k] == nil {
					r := &Report{Kind: kind, ID: g.id, Page: g.page, Usage: usage}
					if page != 0 {
						r.Page = page
					}
					seen[k] = r
					order = append(order, k)
				}
				bits[k] += g.size * g.count
			}
			if kind != 0 || it.Tag == tagCollection {
				// Local items last until the next main item, and no further.
				usage, page, local = 0, 0, false
			}
		}
		return true
	})
	out := make([]Report, 0, len(order))
	for _, k := range order {
		r := *seen[k]
		r.Len = int((bits[k]+7)/8) + 1
		out = append(out, r)
	}
	return out
}
