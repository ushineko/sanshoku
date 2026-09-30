package sanshoku

import (
	"context"
	"errors"
	"fmt"
	"syscall"

	"github.com/ushineko/sanshoku/battery"
	"github.com/ushineko/sanshoku/cooling"
	"github.com/ushineko/sanshoku/screen"
)

// Bus is the kind of connection a device is found on.
type Bus int

// The buses a driver in this module reports. The zero value is none of them.
const (
	// BusUSB is a device on USB, including one behind a USB receiver.
	BusUSB Bus = iota + 1
	// BusBluetooth is a device on Bluetooth, classic or LE.
	BusBluetooth
	// BusHwmon is a kernel sensor chip under /sys/class/hwmon, which has no
	// bus of its own a program can talk to.
	BusHwmon
)

// String names a bus for the testbench and a consumer's diagnostics.
func (b Bus) String() string {
	switch b {
	case BusUSB:
		return "usb"
	case BusBluetooth:
		return "bluetooth"
	case BusHwmon:
		return "hwmon"
	default:
		return "unknown"
	}
}

/*
Identity is what a program shows the user about a device and what the
testbench keys its report on.

Name is the kernel's HID_NAME with a doubled vendor word dropped ("Razer Razer
Mouse Dock Pro" reads as "Razer Mouse Dock Pro"), or the BlueZ alias, or the
hwmon chip name. Phys is the kernel's physical path, which for a Bluetooth
device carries an address: it is kept for matching and never printed.
*/
type Identity struct {
	Vendor  uint16
	Product uint16
	Bus     Bus
	Name    string
	Phys    string
	Path    string
}

// String is `Name (vvvv:pppp)`, the form a person reads.
func (i Identity) String() string {
	return fmt.Sprintf("%s (%04x:%04x)", i.Name, i.Vendor, i.Product)
}

// Candidate is a device a driver found and has not opened.
type Candidate struct {
	Identity

	// Driver is the Name of the driver that found it.
	Driver string

	// Open opens the device. The caller closes what it returns.
	Open func(context.Context) (Device, error)
}

/*
Device is an open handle on one device.

What else it can do is asked with a type assertion against a capability:
battery.Source, cooling.Source, screen.Panel. There is no capability enum.
A Device is safe for concurrent use from one process; two Opens of the same
candidate are two handles and the caller keeps them apart.
*/
type Device interface {
	Identity() Identity
	Close() error
}

// Driver knows one vendor's protocol on one transport and finds the devices
// that speak it. Find opens nothing.
type Driver interface {
	Name() string
	Find(context.Context) ([]Candidate, error)
}

// The sentinels every driver wraps with %w. See docs/design.md, "Errors".
var (
	// ErrAbsent is nothing to find. Not an error a program shows in red.
	ErrAbsent = errors.New("absent")
	// ErrGone is a device that was there and is not now. Rescan.
	ErrGone = errors.New("device gone")
	// ErrUnsupported is a device the driver recognises and will not speak to.
	ErrUnsupported = errors.New("unsupported device")
)

/*
Scan runs each driver's Find in order and returns every candidate found.

It is a pull: there is no udev monitor and no hotplug event, and a consumer
scans when it wants to know what is there. A driver's error does not stop the
scan; errors are joined and returned beside the candidates the other drivers
found. ErrAbsent is not joined, because absence is not an error. Scan opens
nothing.
*/
func Scan(ctx context.Context, drivers ...Driver) ([]Candidate, error) {
	var (
		found []Candidate
		errs  []error
	)
	for _, d := range drivers {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}
		cands, err := d.Find(ctx)
		found = append(found, cands...)
		if err != nil && !errors.Is(err, ErrAbsent) {
			errs = append(errs, fmt.Errorf("%s: %w", d.Name(), err))
		}
	}
	return found, errors.Join(errs...)
}

/*
IsPermission reports an EACCES or EPERM anywhere in err's chain.

A node the user may not open is found and cannot be opened, which looks like
absence unless someone says otherwise; this is how a consumer says "install
the udev rule" (docs/udev.md) instead.
*/
func IsPermission(err error) bool {
	return errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM)
}

// Capabilities names the capability interfaces a device satisfies, in a fixed
// order: "battery", "cooling", "screen". For the testbench and a consumer's
// diagnostics; a program that wants to use a capability asserts it.
func Capabilities(d Device) []string {
	var names []string
	if _, ok := d.(battery.Source); ok {
		names = append(names, "battery")
	}
	if _, ok := d.(cooling.Source); ok {
		names = append(names, "cooling")
	}
	if _, ok := d.(screen.Panel); ok {
		names = append(names, "screen")
	}
	return names
}
