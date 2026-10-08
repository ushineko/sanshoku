//go:build !windows

package hidraw

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

/*
Nodes lists the hidraw nodes of one vendor that satisfy want. A vendor of zero
means any vendor; a nil want takes every node.

**Never by product ID.** The SteelSeries keyboard hayami was written against
enumerates as `1038:1644` with its keyboard on 2.4 GHz and `1038:1646` with the
same keyboard on its cable, on the same USB port, and the hidraw numbers land
on the same indices both times. A reader keyed to the product reads whichever
one it was told about and says nothing about the other. The vendor does not
move, and the usage page is what actually says "this endpoint speaks the
protocol".

Nor by node number: the numbering changes when a device is replugged.

A machine with no hidraw tree at all (a container, a kernel without the
driver) returns nil and no error: it is a machine with no devices to report.
*/
func Nodes(vendor uint16, want func(Node) bool) ([]Node, error) {
	entries, err := os.ReadDir(SysRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing hidraw nodes: %w", err)
	}

	var found []Node
	for _, e := range entries {
		dir := filepath.Join(SysRoot, e.Name(), "device")
		uevent, err := os.ReadFile(filepath.Join(dir, "uevent"))
		if err != nil {
			continue
		}
		bus, v, p, ok := hidID(field(uevent, "HID_ID="))
		if !ok || (vendor != 0 && v != vendor) {
			continue
		}
		descriptor, err := os.ReadFile(filepath.Join(dir, "report_descriptor"))
		if err != nil {
			continue
		}
		n := Node{
			Path:       path.Join(DevRoot, e.Name()),
			Name:       undouble(field(uevent, "HID_NAME=")),
			Phys:       field(uevent, "HID_PHYS="),
			Vendor:     v,
			Product:    p,
			Bus:        bus,
			Descriptor: descriptor,
			Reports:    reportsOf(descriptor),
		}
		if want != nil && !want(n) {
			continue
		}
		if bus == BusUSB {
			n.USBPath = usbNode(filepath.Join(SysRoot, e.Name()))
		}
		found = append(found, n)
	}
	return found, nil
}

/*
hidID reads HID_ID, which the kernel writes as bus:vendor:product with each
field zero-padded to eight hex digits.

The fields are read as *numbers* and matched as fields: comparing them as text
means deciding what to do about the padding, and stripping the padding off
"0000046D" with TrimLeft takes the vendor's own leading zero with it and leaves
"46D", which matches nothing. Matching as a substring instead would take a
product ID that happens to contain 046D for a Logitech device.
*/
func hidID(id string) (bus, vendor, product uint16, ok bool) {
	fields := strings.Split(id, ":")
	if len(fields) != 3 {
		return 0, 0, 0, false
	}
	var v [3]uint16
	for i, f := range fields {
		n, err := strconv.ParseUint(strings.TrimSpace(f), 16, 16)
		if err != nil {
			return 0, 0, 0, false
		}
		v[i] = uint16(n)
	}
	return v[0], v[1], v[2], true
}

// field reads one `KEY=value` line out of a node's uevent.
func field(uevent []byte, key string) string {
	for line := range strings.SplitSeq(string(uevent), "\n") {
		if value, ok := strings.CutPrefix(line, key); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// usbLevels bounds the walk up from a hidraw node: an interface, a device,
// and room for an unusual topology.
const usbLevels = 8

/*
usbNode walks up from a hidraw node to the USB device it belongs to, and builds
the usbfs path from its bus and device numbers.

Up rather than across: the same physical device appears as a hidraw character
device and as a USB device, and the only reliable link between them is the
sysfs tree that contains both. Empty when nothing above has a bus and device
number; sysfs is not guaranteed to look the way one machine's does, and a node
without a usbfs path is still a node.
*/
func usbNode(hidraw string) string {
	dir, err := filepath.EvalSymlinks(filepath.Join(hidraw, "device"))
	if err != nil {
		return ""
	}
	for range usbLevels {
		bus, err1 := os.ReadFile(filepath.Join(dir, "busnum"))
		dev, err2 := os.ReadFile(filepath.Join(dir, "devnum"))
		if err1 == nil && err2 == nil {
			return path.Join(DevRoot, "bus", "usb",
				fmt.Sprintf("%03d", atoi(bus)), fmt.Sprintf("%03d", atoi(dev)))
		}
		parent := filepath.Dir(dir)
		if parent == dir || parent == "/" {
			break
		}
		dir = parent
	}
	return ""
}

func atoi(b []byte) int {
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return n
}
