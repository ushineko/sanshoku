package main

import (
	"context"
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/all"
	"github.com/ushineko/sanshoku/support"
)

// openTimeout bounds one candidate's Open, so a device that hangs does not
// hold up the rest of the bench.
const openTimeout = 5 * time.Second

// drivers is all.Drivers, or the one named.
func drivers(name string) ([]sanshoku.Driver, error) {
	every := all.Drivers()
	if name == "" {
		return every, nil
	}
	for _, d := range every {
		if d.Name() == name {
			return []sanshoku.Driver{d}, nil
		}
	}
	names := make([]string, 0, len(every))
	for _, d := range every {
		names = append(names, d.Name())
	}
	return nil, fmt.Errorf("no driver %q; the drivers are %s", name, strings.Join(names, ", "))
}

// open opens a candidate with a bound of its own.
func open(ctx context.Context, c sanshoku.Candidate) (sanshoku.Device, error) {
	ctx, cancel := context.WithTimeout(ctx, openTimeout)
	defer cancel()
	return c.Open(ctx)
}

// entryFor finds the support entry that covers a candidate.
func entryFor(entries []support.Entry, c sanshoku.Candidate) (support.Entry, bool) {
	return support.Lookup(entries, c.Driver, c.Vendor, c.Product, c.Name)
}

// tierWords is a candidate's tier as the bench prints it: on this system.
func tierWords(e support.Entry, ok bool) string {
	if !ok {
		return "not in the support table"
	}
	return tierHere(e, runtime.GOOS).String()
}

// tierHere is where an entry stands on the system the bench runs on: its
// Windows tier on Windows where it has one (spec 012), its own elsewhere.
func tierHere(e support.Entry, goos string) support.Tier {
	if goos == "windows" && e.Windows != nil {
		return e.Windows.Tier
	}
	return e.Tier
}

// reportHint is the line printed beside an Expected device.
func reportHint(c sanshoku.Candidate) string {
	return fmt.Sprintf("expected to work, not yet confirmed: run `sanshoku-bench read --json --driver %s` "+
		"and open an issue at https://github.com/ushineko/sanshoku/issues with the output to promote it", c.Driver)
}

// udevRules is docs/udev.md's table, by vendor, for the line a permission
// error prints.
var udevRules = map[uint16][]string{
	0x046d: {`KERNEL=="hidraw*", SUBSYSTEMS=="usb", ATTRS{idVendor}=="046d", TAG+="uaccess"`},
	0x1532: {`KERNEL=="hidraw*", SUBSYSTEMS=="usb", ATTRS{idVendor}=="1532", TAG+="uaccess"`},
	0x1038: {`KERNEL=="hidraw*", SUBSYSTEMS=="usb", ATTRS{idVendor}=="1038", TAG+="uaccess"`},
	0x1e71: {
		`KERNEL=="hidraw*", SUBSYSTEMS=="usb", ATTRS{idVendor}=="1e71", TAG+="uaccess"`,
		`SUBSYSTEM=="usb", ATTRS{idVendor}=="1e71", TAG+="uaccess"`,
	},
}

// permissionHint says what to install when a node could not be opened for
// want of permission.
func permissionHint(id sanshoku.Identity) string {
	rules, ok := udevRules[id.Vendor]
	if !ok {
		return "permission denied: this user may not open the node; see docs/udev.md"
	}
	return "permission denied: install the udev rule (docs/udev.md, packaging/60-sanshoku.rules), then replug:\n      " +
		strings.Join(rules, "\n      ")
}

// bluezAddress is the address segment of a BlueZ object path,
// /org/bluez/hci0/dev_AA_BB_CC_DD_EE_FF.
var bluezAddress = regexp.MustCompile(`(?i)dev(_[0-9a-f]{2}){6}`)

// windowsAddress is the address in a Windows Bluetooth device instance ID,
// which carries it twice and unseparated:
// BTHENUM\DEV_AABBCCDDEEFF\8&...&BLUETOOTHDEVICE_AABBCCDDEEFF for a device,
// and ...\8&...&0&AABBCCDDEEFF_C00000000 for one of its profiles (spec 016).
var windowsAddress = regexp.MustCompile(`(?i)(DEV_|DEVICE_|&)[0-9a-f]{12}`)

// shownPath is a candidate's path as the bench prints it. A Bluetooth device's
// D-Bus object path, and its Windows instance ID, carry its address, which the
// bench never prints, so that part is masked.
func shownPath(id sanshoku.Identity) string {
	p := bluezAddress.ReplaceAllString(id.Path, "dev_XX_XX_XX_XX_XX_XX")
	return windowsAddress.ReplaceAllString(p, "${1}XXXXXXXXXXXX")
}
