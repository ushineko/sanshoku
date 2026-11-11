//go:build !windows

package powersupply

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/battery"
)

// tree is a fake sysfs: a power_supply class directory and the HID devices
// its nodes link to.
type tree struct {
	t    *testing.T
	root string // the class directory, as Root
	sys  string // where the HID devices live
}

func newTree(t *testing.T) tree {
	t.Helper()
	base := t.TempDir()
	tr := tree{t: t, root: filepath.Join(base, "class", "power_supply"), sys: filepath.Join(base, "devices")}
	require.NoError(t, os.MkdirAll(tr.root, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(base, "drivers"), 0o755))
	return tr
}

// hid writes a HID device named instance, bound to kernel, with uevent.
func (tr tree) hid(instance, kernel, uevent string) string {
	tr.t.Helper()
	dir := filepath.Join(tr.sys, "uhid", instance)
	require.NoError(tr.t, os.MkdirAll(dir, 0o755))
	require.NoError(tr.t, os.WriteFile(filepath.Join(dir, "uevent"), []byte(uevent), 0o644))
	if kernel != "" {
		drv := filepath.Join(filepath.Dir(tr.root), "..", "drivers", kernel)
		require.NoError(tr.t, os.MkdirAll(drv, 0o755))
		require.NoError(tr.t, os.Symlink(drv, filepath.Join(dir, "driver")))
	}
	return dir
}

// node writes a power_supply node named name, linked to device, with attrs.
func (tr tree) node(name, device string, attrs map[string]string) string {
	tr.t.Helper()
	dir := filepath.Join(tr.root, name)
	require.NoError(tr.t, os.MkdirAll(dir, 0o755))
	for k, v := range attrs {
		require.NoError(tr.t, os.WriteFile(filepath.Join(dir, k), []byte(v+"\n"), 0o644))
	}
	if device != "" {
		require.NoError(tr.t, os.Symlink(device, filepath.Join(dir, "device")))
	}
	return dir
}

// battery attributes as a device battery node carries them, with extra over
// them.
func attrs(extra map[string]string) map[string]string {
	a := map[string]string{"type": "Battery", "scope": "Device", "present": "1", "status": "Discharging"}
	for k, v := range extra {
		if v == "" {
			delete(a, k)
			continue
		}
		a[k] = v
	}
	return a
}

const (
	proUevent = "HID_ID=0005:0000057E:00002009\nHID_NAME=Pro Controller\nHID_UNIQ=aa:bb:cc:dd:ee:ff\n"
	proNode   = "nintendo_switch_controller_battery_0005:057E:2009.0001"
)

// readOne finds and reads every candidate and returns the one reading.
func readOne(t *testing.T, root string) (sanshoku.Candidate, []battery.Battery) {
	t.Helper()
	found, err := find(context.Background(), root)
	require.NoError(t, err)
	require.Len(t, found, 1)
	dev, err := found[0].Open(context.Background())
	require.NoError(t, err)
	got, err := dev.(battery.Source).Batteries(context.Background())
	require.NoError(t, err)
	return found[0], got
}

// The 8BitDo in its Switch pose, as measured: a capacity level and no
// capacity, which is a band and never a percentage.
func TestTheProControllerReadsAsABand(t *testing.T) {
	tr := newTree(t)
	hid := tr.hid("0005:057E:2009.0001", "nintendo", proUevent)
	tr.node(proNode, hid, attrs(map[string]string{"capacity_level": "High"}))

	c, got := readOne(t, tr.root)

	assert.Equal(t, "Pro Controller", c.Name)
	assert.Equal(t, uint16(0x057E), c.Vendor)
	assert.Equal(t, uint16(0x2009), c.Product)
	assert.Equal(t, sanshoku.BusBluetooth, c.Bus)
	assert.Equal(t, "bt:aa:bb:cc:dd:ee:ff", c.Key)
	assert.Equal(t, driverName, c.Driver)
	require.Len(t, got, 1)
	assert.Equal(t, battery.Battery{
		Name: "Pro Controller", State: battery.Discharging, Kind: battery.KindGamepad,
		Band: battery.BandGood, HasBand: true,
	}, got[0])
}

func TestEachCapacityLevelIsABand(t *testing.T) {
	for level, want := range map[string]battery.Band{
		"Full": battery.BandFull, "High": battery.BandGood, "Normal": battery.BandGood,
		"Low": battery.BandLow, "Critical": battery.BandCritical,
	} {
		t.Run(level, func(t *testing.T) {
			tr := newTree(t)
			hid := tr.hid("0005:057E:2009.0001", "nintendo", proUevent)
			tr.node(proNode, hid, attrs(map[string]string{"capacity_level": level}))

			_, got := readOne(t, tr.root)
			require.Len(t, got, 1)
			assert.Equal(t, want, got[0].Band)
			assert.False(t, got[0].HasLevel)
		})
	}
}

// A node with a percentage reads as one, and the band is left alone even
// where the kernel writes a level beside it, as hid-input does.
func TestACapacityIsALevelAndNoBand(t *testing.T) {
	tr := newTree(t)
	hid := tr.hid("0005:054C:0CE6.0002", "playstation",
		"HID_ID=0005:0000054C:00000CE6\nHID_NAME=Wireless Controller\nHID_UNIQ=aa:bb:cc:dd:ee:01\n")
	tr.node("ps-controller-battery-aa:bb:cc:dd:ee:01", hid,
		attrs(map[string]string{"capacity": "65", "capacity_level": "Normal", "status": "Charging"}))

	c, got := readOne(t, tr.root)

	require.Len(t, got, 1)
	assert.True(t, got[0].HasLevel)
	assert.Equal(t, 65, got[0].Level)
	assert.False(t, got[0].HasBand)
	assert.Equal(t, battery.Charging, got[0].State)
	assert.Equal(t, battery.KindGamepad, got[0].Kind)
	assert.NotContains(t, c.Path, "aa:bb", "the node's name carries the address; the Path is the HID device's")
}

func TestStatusIsTheState(t *testing.T) {
	for status, want := range map[string]battery.State{
		"Charging": battery.Charging, "Full": battery.Full, "Discharging": battery.Discharging,
		"Not charging": battery.Discharging, "Unknown": battery.Discharging,
	} {
		t.Run(status, func(t *testing.T) {
			tr := newTree(t)
			hid := tr.hid("0005:057E:2009.0001", "nintendo", proUevent)
			tr.node(proNode, hid, attrs(map[string]string{"capacity_level": "Low", "status": status}))

			_, got := readOne(t, tr.root)
			require.Len(t, got, 1)
			assert.Equal(t, want, got[0].State)
		})
	}
}

// A node that does not say how full the battery is, is no reading.
func TestNoLevelIsNoReading(t *testing.T) {
	for name, extra := range map[string]map[string]string{
		"unknown level": {"capacity_level": "Unknown"},
		"neither file":  {},
		"bad capacity":  {"capacity": "-1"},
	} {
		t.Run(name, func(t *testing.T) {
			tr := newTree(t)
			hid := tr.hid("0005:057E:2009.0001", "nintendo", proUevent)
			tr.node(proNode, hid, attrs(extra))

			_, got := readOne(t, tr.root)
			assert.Empty(t, got)
		})
	}
}

// Only a present battery that belongs to a HID device is a candidate: not
// the machine's own battery, not a charger, not a device with no HID parent.
func TestOnlyADeviceBatteryOnAHIDDeviceIsFound(t *testing.T) {
	tr := newTree(t)
	hid := tr.hid("0005:057E:2009.0001", "nintendo", proUevent)
	notHID := filepath.Join(tr.sys, "platform", "thing")
	require.NoError(t, os.MkdirAll(notHID, 0o755))
	tr.node("BAT0", notHID, attrs(map[string]string{"scope": "System", "capacity": "80"}))
	tr.node("AC", notHID, attrs(map[string]string{"type": "Mains"}))
	tr.node("absent", hid, attrs(map[string]string{"present": "0", "capacity": "50"}))
	tr.node("stylus", notHID, attrs(map[string]string{"capacity": "50"}))
	tr.node("unlinked", "", attrs(map[string]string{"capacity": "50"}))

	_, err := find(context.Background(), tr.root)
	assert.ErrorIs(t, err, sanshoku.ErrAbsent)

	tr.node(proNode, hid, attrs(map[string]string{"capacity_level": "Full"}))
	found, err := find(context.Background(), tr.root)
	require.NoError(t, err)
	assert.Len(t, found, 1)
}

func TestNoClassDirectoryIsAbsent(t *testing.T) {
	_, err := find(context.Background(), filepath.Join(t.TempDir(), "none"))
	assert.ErrorIs(t, err, sanshoku.ErrAbsent)
}

// A device that disconnects is no reading, and when it reconnects under a
// new HID instance, and so a new node, it is read again by its key without a
// rescan.
func TestAReconnectedDeviceIsReadAgain(t *testing.T) {
	tr := newTree(t)
	hid := tr.hid("0005:057E:2009.0001", "nintendo", proUevent)
	tr.node(proNode, hid, attrs(map[string]string{"capacity_level": "High"}))
	found, err := find(context.Background(), tr.root)
	require.NoError(t, err)
	dev, err := found[0].Open(context.Background())
	require.NoError(t, err)
	src := dev.(battery.Source)

	require.NoError(t, os.RemoveAll(filepath.Join(tr.root, proNode)))
	got, err := src.Batteries(context.Background())
	require.NoError(t, err, "gone is not an error")
	assert.Empty(t, got)

	hid2 := tr.hid("0005:057E:2009.0002", "nintendo", proUevent)
	tr.node("nintendo_switch_controller_battery_0005:057E:2009.0002", hid2,
		attrs(map[string]string{"capacity_level": "Low"}))
	got, err = src.Batteries(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, battery.BandLow, got[0].Band)
}

// The kernel's kind: a gamepad driver's device is a gamepad; hid-generic and
// hid-microsoft bind more than gamepads and say nothing.
func TestTheKindComesFromTheKernelDriver(t *testing.T) {
	assert.Equal(t, battery.KindGamepad, kind("nintendo"))
	assert.Equal(t, battery.KindGamepad, kind("xpadneo"))
	assert.Equal(t, battery.KindOther, kind("hid-generic"))
	assert.Equal(t, battery.KindOther, kind("microsoft"))
	assert.Equal(t, battery.KindOther, kind(""))
}

// An error says which device and never carries its address.
func TestAnErrorCarriesNoAddress(t *testing.T) {
	tr := newTree(t)
	hid := tr.hid("0005:057E:2009.0001", "nintendo", proUevent)
	tr.node(proNode, hid, attrs(map[string]string{"capacity_level": "High"}))
	found, err := find(context.Background(), tr.root)
	require.NoError(t, err)
	dev, err := found[0].Open(context.Background())
	require.NoError(t, err)

	require.NoError(t, os.RemoveAll(tr.root))
	require.NoError(t, os.WriteFile(tr.root, nil, 0o644)) // a file where the class directory was
	_, err = dev.(battery.Source).Batteries(context.Background())
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "aa:bb")
}

func TestACancelledContextStopsTheListing(t *testing.T) {
	tr := newTree(t)
	hid := tr.hid("0005:057E:2009.0001", "nintendo", proUevent)
	tr.node(proNode, hid, attrs(map[string]string{"capacity_level": "High"}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := find(ctx, tr.root)
	assert.True(t, errors.Is(err, context.Canceled))
}

func TestItIsAFallback(t *testing.T) {
	var d sanshoku.Driver = Driver{}
	_, ok := d.(sanshoku.Fallback)
	assert.True(t, ok)
}
