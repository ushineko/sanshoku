package main

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/support"
)

// The bench never prints a Bluetooth address. A BlueZ object path ends in one,
// so its segment is masked; a hidraw path is printed as it is.
func TestTheBenchMasksTheAddressInABlueZPath(t *testing.T) {
	bt := sanshoku.Identity{Bus: sanshoku.BusBluetooth, Path: "/org/bluez/hci0/dev_A1_B2_C3_d4_e5_F6"}
	assert.Equal(t, "/org/bluez/hci0/dev_XX_XX_XX_XX_XX_XX", shownPath(bt))

	hid := sanshoku.Identity{Bus: sanshoku.BusUSB, Path: "/dev/hidraw5"}
	assert.Equal(t, "/dev/hidraw5", shownPath(hid))
}

// On Windows the address is in the device instance ID, twice and with no
// separators, and both are masked (spec 016). A Windows HID path, whose
// instance segment is shorter, is printed as it is.
func TestTheBenchMasksTheAddressInAWindowsInstanceID(t *testing.T) {
	dev := sanshoku.Identity{Bus: sanshoku.BusBluetooth,
		Path: `BTHENUM\DEV_A1B2C3D4E5F6\8&C98FE1A&0&BLUETOOTHDEVICE_a1b2c3d4e5f6`}
	assert.Equal(t, `BTHENUM\DEV_XXXXXXXXXXXX\8&C98FE1A&0&BLUETOOTHDEVICE_XXXXXXXXXXXX`, shownPath(dev))

	profile := sanshoku.Identity{Bus: sanshoku.BusBluetooth,
		Path: `BTHENUM\{0000111E-0000-1000-8000-00805F9B34FB}_VID&0001009E_PID&400C\8&C98FE1A&0&A1B2C3D4E5F6_C00000000`}
	assert.Equal(t,
		`BTHENUM\{0000111E-0000-1000-8000-00805F9B34FB}_VID&0001009E_PID&400C\8&C98FE1A&0&XXXXXXXXXXXX_C00000000`,
		shownPath(profile))

	hid := sanshoku.Identity{Bus: sanshoku.BusUSB, Path: `\?\HID#VID_1532&PID_0088&MI_00#8&2f1a&0&0000#{4d1e55b2-f16f-11cf-88cb-001111000030}`}
	assert.Equal(t, hid.Path, shownPath(hid))
}

// light writes to a device, so it refuses without --yes and opens nothing.
func TestLightRefusesWithoutYes(t *testing.T) {
	var out, errOut strings.Builder
	assert.Equal(t, 2, run(context.Background(), []string{"light"}, &out, &errOut))
	assert.Contains(t, errOut.String(), "--yes")
	assert.Empty(t, out.String())
}

// --only reads hex key ids, with or without 0x; anything else is refused.
func TestLightReadsKeyIDs(t *testing.T) {
	ids, err := parseIDs("29, 0x00,e6")
	require.NoError(t, err)
	assert.Equal(t, []byte{0x29, 0x00, 0xe6}, ids)
	_, err = parseIDs("29,zz")
	require.Error(t, err)
}

// The bench says where a device stands on the system it runs on: an entry's
// Windows tier on Windows, its own tier everywhere else and wherever it has
// no Windows standing (spec 012).
func TestTierHereIsTheRunningSystems(t *testing.T) {
	e := support.Entry{Tier: support.Expected, Windows: &support.Port{Tier: support.Tested}}
	assert.Equal(t, support.Tested, tierHere(e, "windows"))
	assert.Equal(t, support.Expected, tierHere(e, "linux"))
	assert.Equal(t, support.Tested, tierHere(support.Entry{Tier: support.Tested}, "windows"))
}
