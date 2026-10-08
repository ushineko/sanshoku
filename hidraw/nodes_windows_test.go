package hidraw

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The collection paths and instances below are invented in the shape Windows
// gives them (spec 012); none is a real device's.
const (
	receiverInstance = `USB\VID_046D&PID_C52B&MI_02\7&AAAAAAA&0&0002`
	keysInstance     = `USB\VID_046D&PID_C52B&MI_00\7&AAAAAAA&0&0000`
	shortPath        = `\\?\HID#VID_046D&PID_C52B&MI_02&Col01#8&bbbbbbb&0&0000#{4d1e55b2-f16f-11cf-88cb-001111000030}`
	longPath         = `\\?\HID#VID_046D&PID_C52B&MI_02&Col02#8&bbbbbbb&0&0001#{4d1e55b2-f16f-11cf-88cb-001111000030}`
	veryLongPath     = `\\?\HID#VID_046D&PID_C52B&MI_02&Col03#8&bbbbbbb&0&0002#{4d1e55b2-f16f-11cf-88cb-001111000030}`
	keysPath         = `\\?\HID#VID_046D&PID_C52B&MI_00#8&ccccccc&0&0000#{4d1e55b2-f16f-11cf-88cb-001111000030}\KBD`
)

// hidpp is a Logitech receiver's HID++ interface as Windows lists it: three
// collections, in an order that is not their paths' order.
func hidpp() []collectionInfo {
	info := func(path string, id byte, usage uint16, n int) collectionInfo {
		return collectionInfo{
			path: path, parent: receiverInstance, vendor: 0x046D, product: 0xC52B,
			manufacturer: "Logitech", name: "USB Receiver", page: 0xFF00, usage: usage,
			inLen: n, outLen: n,
			reports: []Report{
				{Kind: ReportInput, ID: id, Page: 0xFF00, Usage: usage, Len: n},
				{Kind: ReportOutput, ID: id, Page: 0xFF00, Usage: usage, Len: n},
			},
		}
	}
	return []collectionInfo{
		info(longPath, 0x11, 2, 20),
		info(veryLongPath, 0x20, 4, 32),
		info(shortPath, 0x10, 1, 7),
	}
}

/*
The collections of one interface become one node, and another interface's
become another.

The receiver's three HID++ collections share their parent instance and are one
node, whose Path is the first by path and whose Reports are all three's, so
HasReportID finds the short report on it. Its keyboard interface is a node of
its own and does not match. The name is the manufacturer and product joined,
as the kernel's HID_NAME is.
*/
func TestGroupMakesOneNodePerInterface(t *testing.T) {
	keys := collectionInfo{
		path: keysPath, parent: keysInstance, vendor: 0x046D, product: 0xC52B,
		manufacturer: "Logitech", name: "USB Receiver", page: 0x01, usage: 0x06, inLen: 9,
		reports: []Report{{Kind: ReportInput, ID: 0, Page: 0x07, Len: 9}},
	}

	nodes := group(append(hidpp(), keys))

	require.Len(t, nodes, 2)
	var hid Node
	for _, n := range nodes {
		if n.Phys == receiverInstance {
			hid = n
		}
	}
	assert.Equal(t, shortPath, hid.Path, "the node's path is its first collection's")
	assert.Equal(t, "Logitech USB Receiver", hid.Name)
	assert.Equal(t, uint16(0x046D), hid.Vendor)
	assert.Equal(t, uint16(BusUSB), hid.Bus)
	assert.Len(t, hid.Reports, 6)
	assert.Nil(t, hid.Descriptor)
	assert.True(t, HasReportID(0xFF00, 0x10)(hid))

	for _, n := range nodes {
		if n.Phys == keysInstance {
			assert.False(t, HasReportID(0xFF00, 0x10)(n), "the keyboard interface matched as HID++")
		}
	}
	assert.False(t, PairedChild(hid.Phys), "a Windows instance read as a paired child")
}

// A Razer manufacturer string doubles the vendor in the name, and the name is
// undoubled as the kernel's is.
func TestGroupUndoublesTheName(t *testing.T) {
	nodes := group([]collectionInfo{{
		path: `\\?\HID#VID_1532&PID_0088&MI_00#x`, parent: `USB\VID_1532&PID_0088&MI_00\x`,
		manufacturer: "Razer", name: "Razer Basilisk Ultimate Dongle",
	}})
	require.Len(t, nodes, 1)
	assert.Equal(t, "Razer Basilisk Ultimate Dongle", nodes[0].Name)
}

func TestBusIsReadFromTheEnumerator(t *testing.T) {
	assert.Equal(t, uint16(BusUSB), busOf(receiverInstance))
	assert.Equal(t, uint16(BusBluetooth), busOf(`BTHENUM\{00001124-0000-1000-8000-00805F9B34FB}_X`))
	assert.Equal(t, uint16(BusBluetooth), busOf(`BTHLEDevice\{00001812-0000-1000-8000-00805f9b34fb}_X`))
}

/*
A length Windows lists no capability for is still a report: the Razer mouse's
91-byte feature report, measured with no feature capability behind it (spec
012), is recorded unnumbered on the collection's own page and usage. A kind the
collection has no length for is not recorded at all.
*/
func TestReportsFromCapsKeepsALengthWithNoCapabilities(t *testing.T) {
	info := collectionInfo{page: 0x01, usage: 0x02, inLen: 9, featLen: 91}
	read := func(kind uintptr, button bool) []hidCap {
		if kind == hidpInput && !button {
			return []hidCap{{UsagePage: 0xFF00, Usage: 0x40}, {UsagePage: 0x01, Usage: 0x30}}
		}
		return nil
	}

	got := reportsFromCaps(info, read)

	assert.Equal(t, []Report{
		{Kind: ReportInput, ID: 0, Page: 0xFF00, Usage: 0x40, Len: 9},
		{Kind: ReportInput, ID: 0, Page: 0x01, Usage: 0x30, Len: 9},
		{Kind: ReportFeature, ID: 0, Page: 0x01, Usage: 0x02, Len: 91},
	}, got)
}
