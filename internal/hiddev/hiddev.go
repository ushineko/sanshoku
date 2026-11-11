/*
Package hiddev reads what sysfs says about a kernel HID device: its uevent
fields, its HID ID, and the key that says which physical device it is.

hidraw and powersupply find the same HID devices from two directions, one from
the hidraw node and one from the kernel's battery node, and read them the same
way. Spec 022.
*/
package hiddev

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The bus field of HID_ID (BUS_USB and BUS_BLUETOOTH in linux/input.h).
const (
	busUSB       = 0x03
	busBluetooth = 0x05
)

// Field reads one `KEY=value` line out of a uevent. key includes the "=".
func Field(uevent []byte, key string) string {
	for line := range strings.SplitSeq(string(uevent), "\n") {
		if value, ok := strings.CutPrefix(line, key); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

/*
ID reads HID_ID, which the kernel writes as bus:vendor:product with each field
zero-padded to eight hex digits.

The fields are read as *numbers* and matched as fields: comparing them as text
means deciding what to do about the padding, and stripping the padding off
"0000046D" with TrimLeft takes the vendor's own leading zero with it and leaves
"46D", which matches nothing. Matching as a substring instead would take a
product ID that happens to contain 046D for a Logitech device.
*/
func ID(id string) (bus, vendor, product uint16, ok bool) {
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

// BluetoothKey is the key of a Bluetooth device at address: "bt:" and the
// address in lower case, because the kernel writes it in lower case (HID_UNIQ)
// and BlueZ in upper. Empty for no address.
func BluetoothKey(address string) string {
	if address == "" {
		return ""
	}
	return "bt:" + strings.ToLower(address)
}

// usbLevels bounds the walk up from a HID device to its USB device: an
// interface, a device, and room for an unusual topology. hidraw's walk to the
// usbfs node is bounded the same.
const usbLevels = 8

/*
Key says which physical device the HID device at dir (its sysfs directory, or
a link to it) belongs to, for a scan to tell that two drivers found the same
one:

  - on Bluetooth, BluetoothKey of HID_UNIQ, the device's address;
  - on USB, "usb:" and the name of the USB device above it, the port chain
    ("1-13.3.4"), which every interface of the device shares and so does every
    device behind a receiver plugged into it.

Not HID_UNIQ on USB: it is the serial string there, empty on most devices.
Not the HID device itself: one USB device is several of them, one per
interface, and the kernel's battery can hang off a different interface from
the one a driver opens. Empty on any other bus or when sysfs does not say.

The port chain changes when the device moves ports, so a key is compared
within one scan and is not an identity. It carries an address and is never
printed.
*/
func Key(dir string) string {
	uevent, err := os.ReadFile(filepath.Join(dir, "uevent"))
	if err != nil {
		return ""
	}
	bus, _, _, ok := ID(Field(uevent, "HID_ID="))
	if !ok {
		return ""
	}
	switch bus {
	case busBluetooth:
		return BluetoothKey(Field(uevent, "HID_UNIQ="))
	case busUSB:
		return usbKey(dir)
	}
	return ""
}

// usbKey walks up from a HID device to the first directory that is a USB
// device rather than an interface: it has a busnum, and its name has no ':'
// (an interface is "1-13.3:1.0").
func usbKey(dir string) string {
	d, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return ""
	}
	for range usbLevels {
		parent := filepath.Dir(d)
		if parent == d || parent == "/" {
			break
		}
		d = parent
		if _, err := os.Stat(filepath.Join(d, "busnum")); err == nil && !strings.Contains(filepath.Base(d), ":") {
			return "usb:" + filepath.Base(d)
		}
	}
	return ""
}

// Undouble drops a repeated first word: "Razer Razer Mouse Dock Pro", or
// "NZXT, Inc. NZXT Kraken Elite V2", where the manufacturer string carries a
// corporate suffix the product string does not.
//
// That is what the descriptor's manufacturer and product strings concatenate
// to when a vendor puts its own name in both. The doubling is dropped because
// the name is read by a person.
func Undouble(name string) string {
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
