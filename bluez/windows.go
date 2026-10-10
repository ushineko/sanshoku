package bluez

import (
	"strings"

	"github.com/ushineko/sanshoku/battery"
)

/*
Bluetooth on Windows, from the device properties the Bluetooth stack keeps
(spec 016).

There is no BlueZ there, and nothing to ask over a bus: the stack writes what
it knows about each paired device into the properties of the device nodes it
creates, one node for the device and one per profile it offers. The battery
level a hands-free headset reports over HFP lands on the profile node that
received it -- the "Hands-Free AG" node, on the Bose QC35 this was measured
on -- and not on the device's own node, so the two are joined by the container
every node of one physical device shares.

This file is the join and the decoding, and builds everywhere so the tests run
on any machine; windows_windows.go reads the properties.
*/

// btNode is one Bluetooth device node and the properties of it this package
// reads. A property the node does not carry is its zero value with its has
// flag false.
type btNode struct {
	// id is the device instance ID: `BTHENUM\DEV_<address>\...` for a
	// Classic device's own node, `BTHLE\DEV_<address>\...` for an LE one, and
	// a profile's UUID in place of `DEV_` for a profile node. It carries the
	// address, so it is never printed.
	id string

	// container is the node's container ID: one per physical device, shared
	// by the device node and every profile node of it.
	container string

	// name is the friendly name: what the device calls itself, or what its
	// owner renamed it to in Windows.
	name string

	// address is DEVPKEY_Bluetooth_DeviceAddress, twelve hex digits.
	address string

	// class is the Bluetooth class of device, DEVPKEY_Bluetooth_ClassOfDevice.
	class    uint32
	hasClass bool

	// connected is whether the device has a link up now. Only a device's own
	// node carries a meaningful one; a profile node of a connected device
	// reads false.
	connected bool

	// level is the battery the stack holds for the device, on whichever node
	// received it.
	level    int
	hasLevel bool

	// vendor and product are the Device ID profile's, on the profile nodes,
	// from either the Bluetooth SIG's list or USB's as the device chose: the
	// same pair BlueZ writes into a Modalias.
	vendor  uint16
	product uint16
}

// isDevice reports whether n is a device's own node rather than one of its
// profiles: `<enumerator>\DEV_...`.
func (n btNode) isDevice() bool {
	_, rest, ok := strings.Cut(n.id, `\`)
	return ok && strings.HasPrefix(strings.ToUpper(rest), "DEV_")
}

/*
joinNodes turns the nodes of every paired device into one Device per
connected one.

A device whose own node is not connected is not listed, as BlueZ lists only
connected devices: Windows keeps a paired headset's nodes, and its last
battery level, while the headset sits in a drawer. A device whose container
has no battery anywhere is listed with HasLevel false, as on Linux.

Apple is always false here. It says the device is worth the accessory
protocol, which is spoken over L2CAP, which this module reads on Linux only.
*/
func joinNodes(nodes []btNode) []Device {
	byContainer := map[string][]btNode{}
	for _, n := range nodes {
		if n.container != "" {
			byContainer[n.container] = append(byContainer[n.container], n)
		}
	}

	var out []Device
	for _, n := range nodes {
		if !n.isDevice() || !n.connected {
			continue
		}
		d := Device{Path: n.id, Name: n.name, Address: n.address}
		if d.Name == "" {
			d.Name = "Bluetooth device"
		}
		if n.hasClass {
			d.Kind = classKind(n.class)
			d.Audio = d.Kind == battery.KindHeadset
		}
		for _, sibling := range byContainer[n.container] {
			if sibling.hasLevel && !d.HasLevel && sibling.level >= 0 && sibling.level <= 100 {
				d.Level, d.HasLevel = sibling.level, true
			}
			if sibling.vendor != 0 && d.Vendor == 0 {
				d.Vendor, d.Product = sibling.vendor, sibling.product
			}
		}
		out = append(out, d)
	}
	return out
}

/*
The class of device's fields, from the Bluetooth SIG's Assigned Numbers
(section 2.8, Class of Device): the major class in bits 12-8, the minor class
in bits 7-2.

Checked against the two devices on the desk spec 016 was written at: a Bose
QC35 reads 0x240418, audio/video and headphones; a DualSense reads 0x002508,
peripheral and gamepad.
*/
const (
	majorAudioVideo = 0x04
	majorPeripheral = 0x05

	// The audio/video minor classes that are worn: a wearable headset, a
	// hands-free device and headphones.
	minorWearableHeadset = 0x01
	minorHandsFree       = 0x02
	minorHeadphones      = 0x06

	// A peripheral's minor class is two fields: bits 7-6 say keyboard,
	// pointing device or both, and bits 5-2 the sort of device (joystick,
	// gamepad, remote control ...).
	peripheralKeyboard = 0x1
	peripheralPointing = 0x2
	peripheralJoystick = 0x1
	peripheralGamepad  = 0x2
)

/*
classKind is what sort of device a class of device says this is.

The class rather than the container category Windows also writes
("Audio.Headphone", "Input.Gaming"): the class is the device's own, the same on
every system, and is what BlueZ picks its icon from on Linux. A keyboard with
a pointing stick (both bits) and anything unlisted is KindOther, which says
nothing false about it.
*/
func classKind(class uint32) battery.Kind {
	major := (class >> 8) & 0x1F
	minor := (class >> 2) & 0x3F
	switch major {
	case majorAudioVideo:
		switch minor {
		case minorWearableHeadset, minorHandsFree, minorHeadphones:
			return battery.KindHeadset
		}
	case majorPeripheral:
		switch minor >> 4 {
		case peripheralKeyboard:
			return battery.KindKeyboard
		case peripheralPointing:
			return battery.KindMouse
		}
		switch minor & 0xF {
		case peripheralJoystick, peripheralGamepad:
			return battery.KindGamepad
		}
	}
	return battery.KindOther
}
