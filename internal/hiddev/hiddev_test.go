//go:build !windows

package hiddev

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hidDevice writes a HID device directory at rel under root with the given
// uevent, and returns its path.
func hidDevice(t *testing.T, root, rel, uevent string) string {
	t.Helper()
	dir := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "uevent"), []byte(uevent), 0o644))
	return dir
}

// usbDevice marks a directory as a USB device, as sysfs does with busnum.
func usbDevice(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "busnum"), []byte("1\n"), 0o644))
}

func TestTwoInterfacesOfOneUSBDeviceHaveOneKey(t *testing.T) {
	root := t.TempDir()
	dev := filepath.Join(root, "usb1", "1-4")
	usbDevice(t, filepath.Join(root, "usb1"))
	usbDevice(t, dev)
	a := hidDevice(t, dev, "1-4:1.0/0003:1038:12E5.0001", "HID_ID=0003:00001038:000012E5\n")
	b := hidDevice(t, dev, "1-4:1.3/0003:1038:12E5.0002", "HID_ID=0003:00001038:000012E5\n")

	assert.Equal(t, "usb:1-4", Key(a))
	assert.Equal(t, Key(a), Key(b))
}

func TestADeviceBehindAHubHasItsWholePortChain(t *testing.T) {
	root := t.TempDir()
	usbDevice(t, filepath.Join(root, "usb1"))
	usbDevice(t, filepath.Join(root, "usb1", "1-2"))
	dev := filepath.Join(root, "usb1", "1-2", "1-2.3")
	usbDevice(t, dev)
	h := hidDevice(t, dev, "1-2.3:1.0/0003:046D:C547.0003", "HID_ID=0003:0000046D:0000C547\n")

	assert.Equal(t, "usb:1-2.3", Key(h))
}

// A device a receiver pairs is a HID device under the receiver's interface,
// so it has the receiver's key.
func TestAPairedDeviceHasItsReceiversKey(t *testing.T) {
	root := t.TempDir()
	dev := filepath.Join(root, "usb1", "1-5")
	usbDevice(t, dev)
	rx := hidDevice(t, dev, "1-5:1.2/0003:046D:C547.0004", "HID_ID=0003:0000046D:0000C547\n")
	child := hidDevice(t, rx, "0003:046D:4082.0005", "HID_ID=0003:0000046D:00004082\n")

	assert.Equal(t, Key(rx), Key(child))
}

func TestABluetoothDeviceIsKeyedByAddressWhateverItsCase(t *testing.T) {
	root := t.TempDir()
	h := hidDevice(t, root, "uhid/0005:057E:2009.0006",
		"HID_ID=0005:0000057E:00002009\nHID_NAME=Pro Controller\nHID_UNIQ=aa:bb:cc:dd:ee:ff\n")

	assert.Equal(t, "bt:aa:bb:cc:dd:ee:ff", Key(h))
	assert.Equal(t, Key(h), BluetoothKey("AA:BB:CC:DD:EE:FF"), "BlueZ writes the address in upper case")
}

func TestNoKeyWithoutSomethingToKeyOn(t *testing.T) {
	root := t.TempDir()
	i2c := hidDevice(t, root, "i2c/0018:04F3:3242.0007", "HID_ID=0018:000004F3:00003242\n")
	btNoUniq := hidDevice(t, root, "uhid/0005:057E:2009.0008", "HID_ID=0005:0000057E:00002009\n")
	usbNoDevice := hidDevice(t, root, "loose/0003:1038:12E5.0009", "HID_ID=0003:00001038:000012E5\n")

	assert.Empty(t, Key(i2c), "a bus other than USB and Bluetooth")
	assert.Empty(t, Key(btNoUniq), "Bluetooth without an address")
	assert.Empty(t, Key(usbNoDevice), "USB with no USB device above it")
	assert.Empty(t, Key(filepath.Join(root, "absent")))
	assert.Empty(t, BluetoothKey(""))
}

func TestIDReadsThePaddedFieldsAsNumbers(t *testing.T) {
	bus, v, p, ok := ID("0003:0000046D:0000C547")
	assert.True(t, ok)
	assert.Equal(t, [3]uint16{0x03, 0x046D, 0xC547}, [3]uint16{bus, v, p})

	_, _, _, ok = ID("0003:0000046D")
	assert.False(t, ok)
}
