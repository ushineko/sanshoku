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

// The bus field of HID_ID, as Node.Bus carries it (BUS_USB and BUS_BLUETOOTH
// in linux/input.h). Only a USB node has a usbfs node above it.
const (
	// BusUSB is a device on USB, including one behind a USB receiver.
	BusUSB = 0x03
	// BusBluetooth is a device on Bluetooth.
	BusBluetooth = 0x05
)

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
	// Logitech receiver's children the device index too (see PairedIndex).
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

// undouble drops a repeated first word: "Razer Razer Mouse Dock Pro", or
// "NZXT, Inc. NZXT Kraken Elite V2", where the manufacturer string carries a
// corporate suffix the product string does not.
//
// That is what the descriptor's manufacturer and product strings concatenate
// to when a vendor puts its own name in both. The doubling is dropped because
// the name is read by a person.
func undouble(name string) string {
	words := strings.Fields(name)
	if len(words) < 2 {
		return strings.Join(words, " ")
	}
	first := strings.Trim(words[0], ",.")
	i := 1
	for i < len(words) && corporate(strings.Trim(words[i], ",.")) {
		i++
	}
	if i < len(words) && strings.EqualFold(first, strings.Trim(words[i], ",.")) {
		return strings.Join(words[i:], " ")
	}
	return strings.Join(words, " ")
}

// corporate is a word that follows a company's name and not a product's.
func corporate(word string) bool {
	switch strings.ToLower(word) {
	case "inc", "ltd", "co", "corp", "corporation", "gmbh", "llc", "limited":
		return true
	}
	return false
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
	_, ok := PairedIndex(phys)
	return ok
}

// maxPairedIndex is the highest device index a receiver numbers: a receiver
// pairs at most six devices, on indices 1 to 6.
const maxPairedIndex = 6

/*
PairedIndex returns the device index a paired child's HID_PHYS ends in, the N
of `:N`, and whether there is one.

A child node answers for that one device and no other, so a driver speaking to
it need ask no other index. N of 0 or above 6 is not an index a receiver
numbers a device with, and is not one.
*/
func PairedIndex(phys string) (index byte, ok bool) {
	_, suffix, ok := strings.Cut(phys, "input")
	if !ok {
		return 0, false
	}
	_, n, ok := strings.Cut(suffix, ":")
	if !ok || n == "" {
		return 0, false
	}
	for _, c := range n {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	v, err := strconv.Atoi(n)
	if err != nil || v < 1 || v > maxPairedIndex {
		return 0, false
	}
	return byte(v), true
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
