package hidraw

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

/*
A node with no descriptor is matched by its Reports, which is every node on
Windows (spec 012). The reports are those Windows listed for a Logitech
receiver's HID++ interface and a Razer mouse's interface 0, as the spec
records them: the receiver's short report 0x10 on page 0xFF00, and the mouse's
vendor input on 0xFF00 beside its pointer, with the unnumbered 91-byte feature
report the battery is read through.
*/
func TestPredicatesReadReportsWhereThereIsNoDescriptor(t *testing.T) {
	receiver := Node{Reports: []Report{
		{Kind: ReportInput, ID: 0x10, Page: 0xFF00, Usage: 1, Len: 7},
		{Kind: ReportOutput, ID: 0x10, Page: 0xFF00, Usage: 1, Len: 7},
		{Kind: ReportInput, ID: 0x11, Page: 0xFF00, Usage: 2, Len: 20},
	}}
	mouse := Node{Reports: []Report{
		{Kind: ReportInput, ID: 0, Page: 0xFF00, Usage: 0x40, Len: 9},
		{Kind: ReportInput, ID: 0, Page: 0x01, Usage: 0x30, Len: 9},
		{Kind: ReportFeature, ID: 0, Page: 0x01, Usage: 0x02, Len: 91},
	}}
	keyboard := Node{Reports: []Report{
		{Kind: ReportInput, ID: 0x10, Page: 0x07, Usage: 0, Len: 9},
	}}

	assert.True(t, HasReportID(0xFF00, 0x10)(receiver))
	assert.False(t, HasReportID(0xFF00, 0x10)(mouse), "the mouse declares no report 0x10")
	assert.False(t, HasReportID(0xFF00, 0x10)(keyboard), "report 0x10 on a keyboard page is not HID++")
	assert.True(t, UsagePage(0xFF00)(mouse))
	assert.False(t, UsagePage(0xFF01)(mouse))
	assert.False(t, UsagePage(0xFF00)(keyboard))
	assert.False(t, UsagePage(0xFF00)(Node{}), "a node that declares nothing matches nothing")
}

// A node that has a descriptor is matched by walking it, even where its
// Reports say something else: the Linux match is unchanged by spec 012.
func TestPredicatesPreferTheDescriptor(t *testing.T) {
	n := Node{
		Descriptor: g502HIDPP,
		Reports:    []Report{{Kind: ReportInput, ID: 0x10, Page: 0x07}},
	}
	assert.True(t, UsagePage(0xFF00)(n))
	assert.True(t, HasReportID(0xFF00, 0x10)(n))
	assert.False(t, UsagePage(0x07)(n), "the Reports were read although there is a descriptor")
}
