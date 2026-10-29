package powersupply

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/battery"
	"github.com/ushineko/sanshoku/internal/hiddev"
)

// driverName is the driver's name, as Name returns it and the support table keys it.
const driverName = "powersupply"

// Root is where the kernel lists power supplies. A variable so a test can point
// it at a tree it wrote itself.
var Root = "/sys/class/power_supply"

/*
Driver reads a HID device's battery from the kernel's power_supply node for
it. Nothing is sent to the device and no device node is opened: the files are
the kernel driver's account of what the device last told it.

The zero value is ready to use.
*/
type Driver struct{}

// Name is "powersupply".
func (Driver) Name() string { return driverName }

// Fallback marks the driver as one whose candidates give way to a driver that
// reads the same device directly (sanshoku.Fallback).
func (Driver) Fallback() {}

/*
Find returns one candidate per battery node that belongs to a device
(scope=Device, so not a laptop's own battery), is present, and hangs off a HID
device. sanshoku.ErrAbsent when there is none, and off Linux. Find opens
nothing.
*/
func (Driver) Find(ctx context.Context) ([]sanshoku.Candidate, error) {
	if runtime.GOOS != "linux" {
		return nil, sanshoku.ErrAbsent
	}
	return find(ctx, Root)
}

// find is Find over a root.
func find(ctx context.Context, root string) ([]sanshoku.Candidate, error) {
	nodes, err := list(ctx, root)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, sanshoku.ErrAbsent
	}
	out := make([]sanshoku.Candidate, 0, len(nodes))
	for _, n := range nodes {
		dev := &device{id: n.id, key: n.key, root: root}
		out = append(out, sanshoku.Candidate{
			Identity: n.id,
			Driver:   driverName,
			Key:      n.key,
			Open: func(context.Context) (sanshoku.Device, error) {
				return dev, nil
			},
		})
	}
	return out, nil
}

// node is one battery node and the HID device it hangs off.
type node struct {
	// dir is the power_supply node's directory. Its name can carry the
	// device's address (hid-playstation's does), so it is not the identity's
	// Path and is never printed.
	dir string
	id  sanshoku.Identity
	key string
	// kernel is the name of the HID device's kernel driver.
	kernel string
}

// list reads every device battery node under root that hangs off a HID
// device. A root that does not exist is a machine with none.
func list(ctx context.Context, root string) ([]node, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing power supplies: %w", err)
	}
	var out []node
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("listing power supplies: %w", err)
		}
		dir := filepath.Join(root, e.Name())
		if read(dir, "type") != "Battery" || read(dir, "scope") != "Device" || read(dir, "present") != "1" {
			continue
		}
		hid, err := filepath.EvalSymlinks(filepath.Join(dir, "device"))
		if err != nil {
			continue
		}
		uevent, err := os.ReadFile(filepath.Join(hid, "uevent"))
		if err != nil {
			continue
		}
		bus, vendor, product, ok := hiddev.ID(hiddev.Field(uevent, "HID_ID="))
		if !ok {
			continue
		}
		kernel := ""
		if drv, err := filepath.EvalSymlinks(filepath.Join(hid, "driver")); err == nil {
			kernel = filepath.Base(drv)
		}
		out = append(out, node{
			dir: dir,
			id: sanshoku.Identity{
				Vendor: vendor, Product: product, Bus: busOf(bus),
				Name: hiddev.Undouble(hiddev.Field(uevent, "HID_NAME=")),
				Path: hid,
			},
			key:    hiddev.Key(hid),
			kernel: kernel,
		})
	}
	return out, nil
}

// busOf is HID_ID's bus field as the module's Bus; none for a bus it has no
// word for.
func busOf(bus uint16) sanshoku.Bus {
	switch bus {
	case 0x03:
		return sanshoku.BusUSB
	case 0x05:
		return sanshoku.BusBluetooth
	}
	return 0
}

// read is one sysfs attribute, trimmed; empty when it cannot be read.
func read(dir, name string) string {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// device is one found battery. It holds nothing open.
type device struct {
	id   sanshoku.Identity
	key  string
	root string
}

func (d *device) Identity() sanshoku.Identity { return d.id }
func (*device) Close() error                  { return nil }

/*
Batteries reads the device's node afresh.

The node is found again by the device's key, or by its HID device where it
has none, not by the node's name: a controller that reconnects comes back
under a new HID instance number and so a new node name, and is read without a
rescan. A device whose node has gone is no reading and no error.
*/
func (d *device) Batteries(ctx context.Context) ([]battery.Battery, error) {
	nodes, err := list(ctx, d.root)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", d.id, err)
	}
	for _, n := range nodes {
		if (d.key != "" && n.key == d.key) || (d.key == "" && n.id.Path == d.id.Path) {
			b, ok := reading(n)
			if !ok {
				return nil, nil
			}
			return []battery.Battery{b}, nil
		}
	}
	return nil, nil
}

// reading is a node's battery, and false for a node that does not say how
// full the battery is.
func reading(n node) (battery.Battery, bool) {
	b := battery.Battery{Name: n.id.Name, State: state(read(n.dir, "status")), Kind: kind(n.kernel)}
	if c, err := strconv.Atoi(read(n.dir, "capacity")); err == nil && c >= 0 && c <= 100 {
		b.Level, b.HasLevel = c, true
		return b, true
	}
	band, ok := bands[read(n.dir, "capacity_level")]
	if !ok {
		return battery.Battery{}, false
	}
	b.Band, b.HasBand = band, true
	return b, true
}

/*
bands is the kernel's capacity_level as the four bands (spec 022, Q1).

The kernel has five words; hid-nintendo publishes the controller's own five
steps one to one (empty, low, medium, high, full in joycon_parse_battery_status).
Normal, the controller's "medium", is Good, so Low keeps meaning "charge it
soon". Unknown is not here: it is no reading.
*/
var bands = map[string]battery.Band{
	"Full":     battery.BandFull,
	"High":     battery.BandGood,
	"Normal":   battery.BandGood,
	"Low":      battery.BandLow,
	"Critical": battery.BandCritical,
}

// state is the kernel's status. "Not charging" and "Unknown" say nothing a
// panel draws, so they are Discharging, the state that draws nothing.
func state(status string) battery.State {
	switch status {
	case "Charging":
		return battery.Charging
	case "Full":
		return battery.Full
	}
	return battery.Discharging
}

// gamepads are the kernel drivers whose every device is a game controller.
var gamepads = map[string]bool{
	"nintendo":    true, // hid-nintendo
	"playstation": true, // hid-playstation
	"sony":        true, // hid-sony: DualShock 3 and 4
	"xpadneo":     true,
}

// kind is a device's kind from its kernel driver; KindOther where the driver
// binds to more than one sort of device, as hid-microsoft (Xbox pads, and
// Microsoft's keyboards and mice) and hid-generic do.
func kind(kernel string) battery.Kind {
	if gamepads[kernel] {
		return battery.KindGamepad
	}
	return battery.KindOther
}
