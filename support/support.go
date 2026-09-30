package support

import (
	"slices"
	"time"
)

// Tier is how far a device's support has got.
type Tier int

const (
	// Tested is a device the testbench has run against, on the hardware the
	// entry names, with the output recorded in the entry's spec.
	Tested Tier = iota
	// Expected is a device whose protocol is generic and whose code path
	// exists, and which no device has confirmed yet. A user's bench report
	// promotes it.
	Expected
	// Listed is a device recognised by ID and not implemented; its Open
	// returns sanshoku.ErrUnsupported.
	Listed
)

// String names a tier, lower case.
func (t Tier) String() string {
	switch t {
	case Tested:
		return "tested"
	case Expected:
		return "expected"
	case Listed:
		return "listed"
	default:
		return "unknown"
	}
}

/*
Entry is one row of the support table: a device, or a protocol family, that a
driver speaks.

An Expected entry for a family ("any HID++ 2.0 device with feature 0x1004")
sits beside the Tested entries for the devices that confirmed it.
*/
type Entry struct {
	// Driver is the Name of the driver that owns the entry.
	Driver string

	// Device is what a person calls it: a product, or a family.
	Device string

	// Match is the rule the driver matches by, in words: "vendor 046d,
	// report ID 0x10 on a vendor page", "1038:1644 or 1038:1646". It is what
	// the page prints; Vendor, Products and Chips are what Lookup matches.
	Match string

	// Vendor is the USB or Bluetooth vendor ID the entry covers, zero for an
	// entry that is not matched by vendor (a hwmon chip).
	Vendor uint16

	// Products are the product IDs the entry covers. Nil means any product
	// of Vendor: a protocol family.
	Products []uint16

	// Chips are the hwmon chip names the entry covers.
	Chips []string

	// Capabilities names what the device offers, in the words of
	// sanshoku.Capabilities where one applies.
	Capabilities []string

	Tier Tier

	// Hardware is the product the entry was measured on, never the machine.
	// Firmware is that product's firmware version where it matters.
	Hardware string
	Firmware string

	// Tested is the date of the bench run that set the tier; zero for an
	// entry that is not Tested.
	Tested time.Time

	// Spec is the number of the spec that added the entry.
	Spec int

	// Notes is anything a reader needs beside the match: for a Listed entry,
	// why it is not implemented.
	Notes string
}

/*
Lookup finds the entry that covers a found device, for a program or the bench
that wants to say what tier it is at.

Precedence, among entries of the named driver: one naming the exact product
of the vendor; then one covering every product of the vendor (nil Products);
then one naming the chip, compared with name. A family entry sits beside the
devices that confirmed it, so the exact product wins over the family.
*/
func Lookup(entries []Entry, driver string, vendor, product uint16, name string) (Entry, bool) {
	match := []func(Entry) bool{
		func(e Entry) bool { return vendor != 0 && e.Vendor == vendor && slices.Contains(e.Products, product) },
		func(e Entry) bool { return vendor != 0 && e.Vendor == vendor && e.Products == nil },
		func(e Entry) bool { return name != "" && slices.Contains(e.Chips, name) },
	}
	for _, m := range match {
		for _, e := range entries {
			if e.Driver == driver && m(e) {
				return e, true
			}
		}
	}
	return Entry{}, false
}
