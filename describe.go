package sanshoku

import "slices"

/*
Description is what a driver says about itself: what a person knows its
devices by, what it looks for, what those devices offer and where it reads
them. Spec 014.

A consumer used to keep its own copy of these facts, one table entry per
driver, and to type-assert one vendor's types to learn the rest; a driver
added here then meant an edit there. With a Description it can list
all.Drivers(), keep the ones it wants, and word what it finds and what it does
not from the driver's own account.
*/
type Description struct {
	// Name is the word a person knows the devices by: a vendor ("Logitech",
	// "Razer"), or a transport where the devices are anybody's ("Bluetooth").
	// Drivers that read the same set of devices share it -- BlueZ's Battery1
	// and Apple's accessory protocol both read the Bluetooth devices on the
	// desk -- so a consumer that groups by Name says one thing about them.
	Name string

	// Finds is what the driver looks for, as a noun after Name: "receiver",
	// "device", "device with a battery", "cooler". "no " + Name + " " + Finds
	// is what nothing found is: "no Logitech receiver", "no Bluetooth device
	// with a battery".
	Finds string

	// Capabilities names what the driver's devices offer, in the words of
	// Capabilities ("battery", "cooling", "screen", "lighting") and of the
	// support table ("temperature" for a hwmon chip). A consumer reading
	// batteries keeps the drivers that list "battery".
	Capabilities []string

	// Platforms are the operating systems the driver reads on, as
	// runtime.GOOS spells them. Off them its Find reports ErrUnavailable or
	// finds nothing. Every driver began on Linux; spec 012 brought the HID
	// drivers to Windows. The support table says the same per device, and a
	// test holds the two together.
	Platforms []string

	// Quiet is whether a candidate that opens and reads nothing is a device
	// that is there and silent, worth saying so: a dock or receiver lists
	// itself whether or not the mouse or keyboard behind it is awake. False
	// where silence says nothing (BlueZ lists only connected devices) or
	// where the driver gives a fuller account through Presencer.
	Quiet bool
}

// On reports whether the driver reads on goos, as runtime.GOOS spells it.
func (d Description) On(goos string) bool { return slices.Contains(d.Platforms, goos) }

// Offers reports whether the driver's devices offer capability.
func (d Description) Offers(capability string) bool {
	return slices.Contains(d.Capabilities, capability)
}

// Describer is a driver that says what it is. Every driver in this module
// satisfies it.
type Describer interface {
	Describe() Description
}

// Describe is d's description, and false for a driver that gives none -- one
// written outside this module.
func Describe(d Driver) (Description, bool) {
	ds, ok := d.(Describer)
	if !ok {
		return Description{}, false
	}
	return ds.Describe(), true
}

/*
Presence is what a receiver holds beyond what could be read: how many of its
nodes were opened, how many paired slots were asked and stayed silent, and
which devices answered in a protocol too old to read. Spec 014, moved here
from the logitech driver (hayami issue #66 is why it exists).

Counted, not named, where it is a slot: a pairing table outlives the hardware
in it, and a slot may be for a device that was never on this desk. Named where
a device answered, because something is there.
*/
type Presence struct {
	// Nodes is how many receiver nodes the device stands for: 1 for one open
	// node. A consumer sums its devices' Presence with Add.
	Nodes int

	// Quiet is how many paired slots were asked and did not answer at the
	// last discovery. A node that stands for one device behind a receiver,
	// beside the receiver's own node, reports 0: the receiver's node asked
	// that slot already, and counting it twice put eight slots on a receiver
	// that numbers six.
	Quiet int

	// TooOld are devices that answered in a protocol older than the driver
	// reads and could not be read in it either, by the name their node
	// carries.
	TooOld []string

	// OldProtocol is the name of the protocol TooOld's devices speak
	// ("HID++ 1.0"), for a consumer to say what they are. Empty when TooOld
	// is.
	OldProtocol string
}

// Add is p and q summed: the Presence of two receivers on one desk.
func (p Presence) Add(q Presence) Presence {
	p.Nodes += q.Nodes
	p.Quiet += q.Quiet
	if len(q.TooOld) > 0 {
		p.TooOld = append(append([]string(nil), p.TooOld...), q.TooOld...)
		if p.OldProtocol == "" {
			p.OldProtocol = q.OldProtocol
		}
	}
	return p
}

// Presencer is a device that reports its Presence. A consumer asserts it on
// any Device rather than naming a driver's type.
type Presencer interface {
	Presence() Presence
}
