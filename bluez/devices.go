package bluez

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/godbus/dbus/v5"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/battery"
)

/*
BlueZ, over its own D-Bus interface.

The program hayami succeeded ran `upower -i` and parsed its output. BlueZ
carries the same numbers with the device's name, its address and whether it is
connected beside them, and it is an interface rather than a human-readable
report, so it is asked directly.

Two things come from here. Which devices are worth talking to at all — that is
how the AirPods are found — and, for everything that reports one, the battery
BlueZ already has.
*/

// appleVendor is Apple's Bluetooth company identifier, as BlueZ writes it into
// a device's Modalias: "bluetooth:vNNNNpNNNNdNNNN".
const appleVendor = "v004C"

// audioUUIDs are the profiles that make a device a pair of headphones rather
// than a phone that happens to be paired.
var audioUUIDs = map[string]bool{
	"0000110b-0000-1000-8000-00805f9b34fb": true, // Audio Sink
	"0000110d-0000-1000-8000-00805f9b34fb": true, // Advanced Audio Distribution
}

// ErrNoBlueZ is BlueZ not being there to ask: no daemon, no system bus, a
// machine with no Bluetooth at all. It wraps sanshoku.ErrAbsent, because that
// is what it is: nothing to find, not a fault.
var ErrNoBlueZ = fmt.Errorf("bluez is not answering: %w", sanshoku.ErrUnavailable)

// Device is what BlueZ knows about one connected device.
type Device struct {
	// Path is the device's D-Bus object path. It is how a driver asks for the
	// same device again. It carries the address, so it is not printed.
	Path string

	// Name is what the device is called, preferring the alias its owner gave
	// it over the name its manufacturer did. It may be a name the user chose
	// and is kept out of error messages.
	Name string

	// Address is the printed Bluetooth address. It is used to dial the device
	// and never printed.
	Address string

	// Vendor and Product are read from the device's Modalias, in either of
	// the forms BlueZ writes ("bluetooth:v004Cp200E…" or "usb:v054Cp0F8A…").
	// Zero when the device has no Modalias.
	Vendor  uint16
	Product uint16

	// Level is the battery BlueZ has, where it has one. HasLevel is false for
	// a device with no org.bluez.Battery1 — which is most of them, and is the
	// whole reason the accessory protocol exists in this module.
	Level    int
	HasLevel bool

	// Apple and Audio say whether this is worth trying the accessory protocol
	// on.
	Apple bool
	Audio bool

	// Kind is what sort of device BlueZ takes this to be, from the icon it
	// gives the device.
	Kind battery.Kind
}

/*
deviceKinds map BlueZ's icon names onto battery kinds.

The icon and not the class of device. Both are on org.bluez.Device1 and the
class is a bit field this package would have to decode; the icon is the name of
a freedesktop icon and BlueZ has already done the decoding to pick it. It is
also what BlueZ gives a device over LE, which has no class at all.

An icon that is not here is KindOther, which is most of them: a phone, a car,
a speaker.
*/
var deviceKinds = map[string]battery.Kind{
	"input-mouse":      battery.KindMouse,
	"input-keyboard":   battery.KindKeyboard,
	"audio-headset":    battery.KindHeadset,
	"audio-headphones": battery.KindHeadset,
}

/*
Devices lists every connected device BlueZ knows about, from the system bus's
org.bluez ObjectManager.

A device that is not connected is not listed: it is paired and elsewhere. Each
call opens its own connection to the system bus and closes it, so the context
bounds the whole of it and nothing is left running between calls. No bus, or
no BlueZ on it, is ErrNoBlueZ.
*/
func Devices(ctx context.Context) ([]Device, error) {
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoBlueZ, err)
	}
	defer func() { _ = conn.Close() }()

	var objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	err = conn.Object("org.bluez", "/").
		CallWithContext(ctx, "org.freedesktop.DBus.ObjectManager.GetManagedObjects", 0).
		Store(&objects)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("listing bluetooth devices: %w", ctxErr)
		}
		return nil, fmt.Errorf("%w: %w", ErrNoBlueZ, err)
	}

	var found []Device
	for path, interfaces := range objects {
		device, ok := interfaces["org.bluez.Device1"]
		if !ok {
			continue
		}
		if !boolOf(device["Connected"]) {
			continue
		}
		d := describe(device, interfaces)
		d.Path = string(path)
		found = append(found, d)
	}
	return found, nil
}

// describe reads one device's properties.
func describe(device map[string]dbus.Variant, interfaces map[string]map[string]dbus.Variant) Device {
	modalias := stringOf(device["Modalias"])
	out := Device{
		Name:    deviceName(device),
		Address: stringOf(device["Address"]),
		Apple:   strings.Contains(modalias, appleVendor),
		Audio:   isAudio(device),
	}
	out.Vendor, out.Product = parseModalias(modalias)
	out.Kind = kindOf(out, device)

	// The battery interface is a sibling of Device1 on the same object, and
	// most devices do not have one.
	if b, ok := interfaces["org.bluez.Battery1"]; ok {
		if level, ok := intOf(b["Percentage"]); ok && level >= 0 && level <= 100 {
			out.Level, out.HasLevel = level, true
		}
	}
	return out
}

// modalias is the vendor and product at the head of a Modalias, which BlueZ
// writes as "bluetooth:" for a vendor from the Bluetooth SIG's list and "usb:"
// for one from USB's.
var modalias = regexp.MustCompile(`^(?:bluetooth|usb):v([0-9A-Fa-f]{4})p([0-9A-Fa-f]{4})`)

// parseModalias reads the vendor and product out of a Modalias, zero for both
// when it is absent or of another form.
func parseModalias(s string) (vendor, product uint16) {
	m := modalias.FindStringSubmatch(s)
	if m == nil {
		return 0, 0
	}
	v, _ := strconv.ParseUint(m[1], 16, 16)
	p, _ := strconv.ParseUint(m[2], 16, 16)
	return uint16(v), uint16(p)
}

/*
kindOf is what sort of device this is, by its icon and then by its profiles.

The profiles are the fallback because a device may have no icon at all: a pair
of Bose headphones paired to the machine hayami was written on has an Audio
Sink and no Icon property, and BlueZ gives one to a device only when it can
pick one. Where there is nothing to go on it is KindOther, which says nothing
false about it.
*/
func kindOf(out Device, device map[string]dbus.Variant) battery.Kind {
	if k, ok := deviceKinds[strings.ToLower(stringOf(device["Icon"]))]; ok {
		return k
	}
	if out.Audio {
		return battery.KindHeadset
	}
	return battery.KindOther
}

// deviceName is what to call a device: the alias if its owner set one,
// otherwise the name it came with.
func deviceName(device map[string]dbus.Variant) string {
	if alias := strings.TrimSpace(stringOf(device["Alias"])); alias != "" {
		return alias
	}
	if n := strings.TrimSpace(stringOf(device["Name"])); n != "" {
		return n
	}
	return "Bluetooth device"
}

// isAudio reports whether a device is headphones, by its profiles or by the
// icon BlueZ picked for it.
func isAudio(device map[string]dbus.Variant) bool {
	if strings.HasPrefix(stringOf(device["Icon"]), "audio-") {
		return true
	}
	// Guarded rather than only error-checked: Store on a Variant that was
	// never set panics instead of failing, and an absent property is the
	// ordinary shape of a D-Bus dictionary rather than a fault.
	profiles, ok := device["UUIDs"]
	if !ok || profiles.Value() == nil {
		return false
	}

	var uuids []string
	if err := profiles.Store(&uuids); err == nil {
		for _, u := range uuids {
			if audioUUIDs[strings.ToLower(u)] {
				return true
			}
		}
	}
	return false
}

func stringOf(v dbus.Variant) string {
	s, _ := v.Value().(string)
	return s
}

func boolOf(v dbus.Variant) bool {
	b, _ := v.Value().(bool)
	return b
}

// intOf reads a number BlueZ may have sent as any of several widths.
func intOf(v dbus.Variant) (int, bool) {
	switch n := v.Value().(type) {
	case uint8:
		return int(n), true
	case int16:
		return int(n), true
	case uint16:
		return int(n), true
	case int32:
		return int(n), true
	case uint32:
		return int(n), true
	case int64:
		return int(n), true
	default:
		return 0, false
	}
}
