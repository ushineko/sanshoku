package hidraw

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The descriptors below are written by hand in the shapes of the devices
// hayami measured (docs/design.md, "Public repository": no captured traffic).
// A Logitech receiver presents three nodes and only the third speaks HID++.
var (
	descriptorMouse    = []byte{0x05, 0x01, 0x09, 0x02, 0xa1, 0x01, 0x09, 0x01, 0xa1, 0x00, 0x95, 0x10}
	descriptorKeyboard = []byte{0x05, 0x01, 0x09, 0x06, 0xa1, 0x01, 0x85, 0x01, 0x05, 0x07, 0x19, 0xe0}
	descriptorHIDPP    = []byte{0x06, 0x00, 0xff, 0x09, 0x01, 0xa1, 0x01, 0x85, 0x10, 0x95, 0x06, 0x75}
)

// withTree points the package at a hidraw tree the test writes.
func withTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	sys, dev := SysRoot, DevRoot
	SysRoot, DevRoot = root, "/dev"
	t.Cleanup(func() { SysRoot, DevRoot = sys, dev })
	return root
}

// node writes one hidraw node into the fake tree.
func node(t *testing.T, root, name, vendor, product string, descriptor []byte) {
	t.Helper()
	dir := filepath.Join(root, name, "device")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	uevent := "DRIVER=hid-generic\nHID_ID=0003:0000" + vendor + ":0000" + product +
		"\nHID_NAME=Razer Razer A Device\nHID_PHYS=usb-0000:00:14.0-1/input0\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "uevent"), []byte(uevent), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "report_descriptor"), descriptor, 0o600))
}

/*
Nodes finds the node of the vendor asked for that declares the usage page asked
for, and not by its number.

Deliberately numbered so that the wanted node is neither first nor last: a
selection that happened to work by position would pass on one ordering and fail
on the next reboot. The same descriptor under another vendor, and the same
vendor's mouse and keyboard interfaces, are ignored.
*/
func TestNodesMatchesVendorAndUsagePageAndIgnoresTheRest(t *testing.T) {
	root := withTree(t)
	node(t, root, "hidraw10", "046D", "C547", descriptorMouse)
	node(t, root, "hidraw12", "046D", "C547", descriptorHIDPP)
	node(t, root, "hidraw11", "046D", "C547", descriptorKeyboard)
	node(t, root, "hidraw3", "1038", "1644", descriptorHIDPP)

	found, err := Nodes(0x046D, UsagePage(0xFF00))

	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, "/dev/hidraw12", found[0].Path)
	assert.Equal(t, uint16(0x046D), found[0].Vendor)
	assert.Equal(t, uint16(0xC547), found[0].Product)
	assert.Equal(t, "Razer A Device", found[0].Name, "a doubled vendor word is read out once")
}

// A machine with no hidraw tree at all (a container, a kernel without the
// driver) is a machine with no devices, not a machine with a fault.
func TestNodesWithNoTreeIsNothingAndNoError(t *testing.T) {
	sys := SysRoot
	SysRoot = filepath.Join(t.TempDir(), "absent")
	t.Cleanup(func() { SysRoot = sys })

	found, err := Nodes(0x046D, nil)

	require.NoError(t, err)
	assert.Nil(t, found)
}
