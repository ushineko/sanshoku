package hidraw

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// SysRoot is where the kernel lists hidraw nodes. A variable so a test can
// point it at a tree it wrote itself and touch no device.
var SysRoot = "/sys/class/hidraw"

// DevRoot is where the nodes themselves live, and the usbfs tree under it,
// for the same reason.
var DevRoot = "/dev"

// busUSB is the bus field of HID_ID for a USB device (BUS_USB in
// linux/input.h). Only a USB node has a usbfs node above it.
const busUSB = 0x03

// Node is one hidraw node: where it is and what the kernel says about it.
//
// The name comes from here rather than from the protocol because the kernel
// already has it (`HID_NAME=SteelSeries Apex Pro TKL Wireless Gen 3`), and
// asking the device for a name it may not have is a round trip for something
// already on disk.
type Node struct {
	// Path is the character device, /dev/hidrawN.
	Path string

	// Name is the kernel's HID_NAME with a doubled vendor word dropped.
	Name string

	// Phys is the kernel's HID_PHYS: which USB interface a node is, and for a
	// Logitech receiver's children the device index too (see PairedChild).
	// On Bluetooth it carries the adapter's address, so it is never printed.
	Phys string

	// Vendor and Product are the IDs out of HID_ID. The product is not how a
	// node is found (see Nodes) but is how a driver with an allow-list decides
	// whether it may write to the device at all.
	Vendor  uint16
	Product uint16

	// Bus is HID_ID's bus field: 0x03 for USB, 0x05 for Bluetooth.
	Bus uint16

	// Descriptor is the node's report descriptor, as the kernel read it.
	Descriptor []byte

	// USBPath is the usbfs node of the USB device the interface belongs to,
	// /dev/bus/usb/BBB/DDD, found by walking up the sysfs tree. Empty when
	// the node is not on USB or no USB device was found above it.
	USBPath string
}

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
			Path:       filepath.Join(DevRoot, e.Name()),
			Name:       undouble(field(uevent, "HID_NAME=")),
			Phys:       field(uevent, "HID_PHYS="),
			Vendor:     v,
			Product:    p,
			Bus:        bus,
			Descriptor: descriptor,
		}
		if want != nil && !want(n) {
			continue
		}
		if bus == busUSB {
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

// undouble drops a repeated first word: "Razer Razer Mouse Dock Pro".
//
// That is what the descriptor's manufacturer and product strings concatenate
// to when a vendor puts its own name in both. The doubling is dropped because
// the name is read by a person.
func undouble(name string) string {
	words := strings.Fields(name)
	if len(words) > 1 && strings.EqualFold(words[0], words[1]) {
		return strings.Join(words[1:], " ")
	}
	return strings.Join(words, " ")
}

/*
PairedChild reports whether a node is one device behind a receiver rather than
the receiver itself.

`hid-logitech-dj` gives a receiver's children the receiver's own HID_PHYS with
`:index` appended:

	usb-0000:03:00.0-3/input2     Logitech USB Receiver
	usb-0000:03:00.0-3/input2:1   Logitech K800
	usb-0000:03:00.0-3/input2:2   Logitech Performance MX

The distinction is load-bearing for a *name*. A receiver node answers for every
device paired to it, so an answer arriving there carries the receiver's name
and not the device's, which put "Logitech USB Receiver" beside the keyboard
that had actually answered.
*/
func PairedChild(phys string) bool {
	_, suffix, ok := strings.Cut(phys, "input")
	if !ok {
		return false
	}
	_, index, ok := strings.Cut(suffix, ":")
	if !ok || index == "" {
		return false
	}
	for _, c := range index {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
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
			return filepath.Join(DevRoot, "bus", "usb",
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
