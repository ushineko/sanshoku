package bluez

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/battery"
)

// anAddress is a printed Bluetooth address in any case, the pattern no error
// message may contain.
var anAddress = regexp.MustCompile(`(?i)[0-9a-f]{2}([:_][0-9a-f]{2}){5}`)

// fixed is a lister over a device list the test controls, so neither the
// system bus nor a radio is touched. Changing *devices between calls is a
// device whose level moved, or that went away.
func fixed(devices *[]Device) lister {
	return func(context.Context) ([]Device, error) { return *devices, nil }
}

const (
	keyboardPath = "/org/bluez/hci0/dev_AA_BB_CC_DD_EE_01"
	airpodsPath  = "/org/bluez/hci0/dev_AA_BB_CC_DD_EE_02"
	carPath      = "/org/bluez/hci0/dev_AA_BB_CC_DD_EE_03"
	phonePath    = "/org/bluez/hci0/dev_AA_BB_CC_DD_EE_04"
)

func desk() []Device {
	return []Device{
		{Path: keyboardPath, Name: "A Keyboard", Address: "AA:BB:CC:DD:EE:01", Vendor: 0x05ac, Product: 0x0267,
			Level: 54, HasLevel: true, Kind: battery.KindKeyboard},
		{Path: airpodsPath, Name: "Some AirPods", Address: "AA:BB:CC:DD:EE:02", Vendor: 0x004c, Product: 0x200e,
			Level: 42, HasLevel: true, Apple: true, Audio: true, Kind: battery.KindHeadset},
		{Path: carPath, Name: "A Car", Address: "AA:BB:CC:DD:EE:03"},
		{Path: phonePath, Name: "An Apple Phone", Address: "AA:BB:CC:DD:EE:04", Vendor: 0x004c, Apple: true,
			Level: 80, HasLevel: true},
	}
}

/*
Every connected device with a level is a candidate, whatever it is, except
Apple audio, which the apple driver reads and would otherwise be reported
twice. A device with no Battery1 is not a candidate: most connected devices are
in that position, and a list of them would be a list of everything paired.
*/
func TestFindListsEveryLevelExceptAppleAudio(t *testing.T) {
	devices := desk()
	found, err := find(context.Background(), fixed(&devices))
	require.NoError(t, err)

	var names []string
	for _, c := range found {
		names = append(names, c.Name)
		assert.Equal(t, driverName, c.Driver)
		assert.Equal(t, sanshoku.BusBluetooth, c.Bus)
	}
	assert.ElementsMatch(t, []string{"A Keyboard", "An Apple Phone"}, names)

	kb := found[0]
	if kb.Name != "A Keyboard" {
		kb = found[1]
	}
	assert.Equal(t, sanshoku.Identity{
		Vendor: 0x05ac, Product: 0x0267, Bus: sanshoku.BusBluetooth,
		Name: "A Keyboard", Phys: "AA:BB:CC:DD:EE:01", Path: keyboardPath,
	}, kb.Identity, "the address is in Phys and the object path in Path")
}

// Nothing with a level connected is absence, not an error.
func TestNothingWithALevelIsAbsent(t *testing.T) {
	devices := []Device{{Path: carPath, Name: "A Car", Address: "AA:BB:CC:DD:EE:03"}}
	_, err := find(context.Background(), fixed(&devices))
	require.ErrorIs(t, err, sanshoku.ErrAbsent)
}

/*
A reading asks BlueZ again, so a level that moved is the new level; a device
that has for now no level is no reading and no error; and a device that is no
longer connected is ErrGone.
*/
func TestBatteriesRereadsTheLevelForItsPath(t *testing.T) {
	devices := desk()
	found, err := find(context.Background(), fixed(&devices))
	require.NoError(t, err)
	c := found[0]
	if c.Name != "A Keyboard" {
		c = found[1]
	}
	dev, err := c.Open(context.Background())
	require.NoError(t, err)
	src := dev.(battery.Source)

	devices[0].Level = 53
	got, err := src.Batteries(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, battery.Battery{Name: "A Keyboard", Level: 53, HasLevel: true, Kind: battery.KindKeyboard}, got[0])

	devices[0].HasLevel = false
	got, err = src.Batteries(context.Background())
	require.NoError(t, err)
	assert.Empty(t, got)

	devices = devices[1:]
	_, err = src.Batteries(context.Background())
	require.ErrorIs(t, err, sanshoku.ErrGone)
	require.NoError(t, dev.Close())
}

/*
No error this driver returns names the device or carries its address.

The repository and its bug tracker are public, and an alias is often a name
somebody chose. Every error path is driven and its message is checked for an
address in either of the forms BlueZ writes one, and for the alias.
*/
func TestNoErrorNamesTheDeviceOrItsAddress(t *testing.T) {
	devices := desk()
	found, err := find(context.Background(), fixed(&devices))
	require.NoError(t, err)

	var errs []error
	for _, c := range found {
		dev, err := c.Open(context.Background())
		require.NoError(t, err)
		src := dev.(battery.Source)

		broken := dev.(*device)
		broken.list = func(context.Context) ([]Device, error) { return nil, errors.New("the bus went away") }
		_, err = src.Batteries(context.Background())
		errs = append(errs, err)

		broken.list = fixed(&[]Device{})
		_, err = src.Batteries(context.Background())
		errs = append(errs, err)

		require.NoError(t, dev.Close())
		_, err = src.Batteries(context.Background())
		errs = append(errs, err)
	}
	for _, d := range desk() {
		for _, err := range errs {
			require.Error(t, err)
			assert.NotRegexp(t, anAddress, err.Error())
			assert.NotContains(t, err.Error(), d.Name)
		}
	}
}

/*
A bus that cannot be reached is BlueZ being absent, not BlueZ failing.

On a machine with no radio at all, `bluetooth.service` never starts, and D-Bus
answers an activation request with "Could not activate remote peer 'org.bluez':
unit failed" (hayami issue #54). Driven through the real Devices with a bus
address that resolves to nothing, because the wrapping is what is under test
and a stub that returned ErrNoBlueZ would be asserting the test's own setup.
*/
func TestAnUnreachableBusIsErrNoBlueZ(t *testing.T) {
	t.Setenv("DBUS_SYSTEM_BUS_ADDRESS", "unix:path=/nonexistent/sanshoku-test")

	_, err := Devices(context.Background())

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoBlueZ,
		"an unreachable system bus was reported as a failure rather than as no Bluetooth")
	assert.ErrorIs(t, err, sanshoku.ErrUnavailable, "no BlueZ is a missing transport, which Scan reports")
	assert.NotErrorIs(t, err, sanshoku.ErrAbsent, "no BlueZ was reported as no device, which Scan would hide")

	_, err = Driver{}.Find(context.Background())
	assert.ErrorIs(t, err, sanshoku.ErrUnavailable)
}

/*
The vendor and product come from the Modalias in either form BlueZ writes, so
an Apple accessory keys on vendor 004c; Apple is hayami's test, "v004C" in the
Modalias. The kind comes from the icon, then from the profiles.
*/
func TestDescribeReadsTheModaliasAndTheKind(t *testing.T) {
	for _, c := range []struct {
		props   map[string]any
		vendor  uint16
		product uint16
		apple   bool
		audio   bool
		kind    battery.Kind
	}{
		{map[string]any{"Modalias": "bluetooth:v004Cp200Ed0100", "Icon": "audio-headphones"},
			0x004c, 0x200e, true, true, battery.KindHeadset},
		{map[string]any{"Modalias": "usb:v054Cp0F8Ad0300", "Icon": "audio-headset"},
			0x054c, 0x0f8a, false, true, battery.KindHeadset},
		{map[string]any{"Modalias": "usb:v05ACp0267d0001", "Icon": "input-keyboard"},
			0x05ac, 0x0267, false, false, battery.KindKeyboard},
		// A pair of headphones with no icon, known by its audio sink.
		{map[string]any{"UUIDs": []string{"0000110B-0000-1000-8000-00805F9B34FB"}},
			0, 0, false, true, battery.KindHeadset},
		{map[string]any{"Icon": "phone"}, 0, 0, false, false, battery.KindOther},
	} {
		props := map[string]dbus.Variant{
			"Alias":   dbus.MakeVariant("A Device"),
			"Address": dbus.MakeVariant("AA:BB:CC:DD:EE:FF"),
		}
		for k, v := range c.props {
			props[k] = dbus.MakeVariant(v)
		}
		level := map[string]map[string]dbus.Variant{"org.bluez.Battery1": {"Percentage": dbus.MakeVariant(byte(47))}}

		d := describe(props, level)

		assert.Equal(t, c.vendor, d.Vendor, "%v", c.props)
		assert.Equal(t, c.product, d.Product, "%v", c.props)
		assert.Equal(t, c.apple, d.Apple, "%v", c.props)
		assert.Equal(t, c.audio, d.Audio, "%v", c.props)
		assert.Equal(t, c.kind, d.Kind, "%v", c.props)
		assert.Equal(t, "A Device", d.Name)
		assert.True(t, d.HasLevel)
		assert.Equal(t, 47, d.Level)
	}
}
