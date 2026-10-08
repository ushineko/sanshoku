package hidraw

import (
	"strconv"
	"strings"
)

// SysRoot is where the kernel lists hidraw nodes. A variable so a test can
// point it at a tree it wrote itself and touch no device. Linux only: Windows
// lists HID devices through its device manager and reads neither.
var SysRoot = "/sys/class/hidraw"

// DevRoot is where the nodes themselves live, and the usbfs tree under it,
// for the same reason.
var DevRoot = "/dev"

// The bus field of HID_ID, as Node.Bus carries it (BUS_USB and BUS_BLUETOOTH
// in linux/input.h). Only a USB node has a usbfs node above it. Windows nodes
// carry the same numbers, read from the bus their interface hangs off.
const (
	// BusUSB is a device on USB, including one behind a USB receiver.
	BusUSB = 0x03
	// BusBluetooth is a device on Bluetooth.
	BusBluetooth = 0x05
)

/*
Node is one HID interface: where it is and what the system says about it.

On Linux that is one hidraw node. On Windows it is one USB interface's
top-level collections together (spec 012): Windows gives each collection a
device path of its own, so a Logitech receiver's HID++ interface is three
paths -- one for the short report, one for the long, one for the very long --
where Linux has one node. A driver asks one node one question on both.

The name comes from here rather than from the protocol because the system
already has it (`HID_NAME=SteelSeries Apex Pro TKL Wireless Gen 3`), and
asking the device for a name it may not have is a round trip for something
already on disk.
*/
type Node struct {
	// Path is what Open takes: the character device, /dev/hidrawN, on Linux;
	// on Windows the device path of the interface's first collection, from
	// which Open finds the rest.
	Path string

	// Name is the kernel's HID_NAME with a doubled vendor word dropped. On
	// Windows it is the manufacturer and product strings the device reports,
	// joined as the kernel joins them and undoubled the same way.
	Name string

	// Phys is the kernel's HID_PHYS: which USB interface a node is, and for a
	// Logitech receiver's children the device index too (see PairedIndex).
	// On Windows it is the device instance the collections hang off. On
	// Bluetooth it carries the adapter's or the device's address, and a USB
	// device's instance can carry its serial, so it is never printed.
	Phys string

	// Vendor and Product are the IDs out of HID_ID. The product is not how a
	// node is found (see Nodes) but is how a driver with an allow-list decides
	// whether it may write to the device at all.
	Vendor  uint16
	Product uint16

	// Bus is HID_ID's bus field: 0x03 for USB, 0x05 for Bluetooth.
	Bus uint16

	// Descriptor is the node's report descriptor, as the kernel read it. Nil
	// on Windows, which hands a program no descriptor.
	Descriptor []byte

	// Reports are the reports the node declares, read from Descriptor on
	// Linux and from Windows' parse of the descriptor there. The predicates
	// read them where there is no Descriptor.
	Reports []Report

	// USBPath is the usbfs node of the USB device the interface belongs to,
	// /dev/bus/usb/BBB/DDD, found by walking up the sysfs tree. Empty when
	// the node is not on USB or no USB device was found above it, and always
	// empty on Windows, which has no usbfs.
	USBPath string
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
