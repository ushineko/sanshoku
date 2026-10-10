package bluez

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku/battery"
)

// The class-of-device values measured in spec 016, and two built from the
// Assigned Numbers for the kinds the desk did not have.
const (
	classQC35      = 0x240418 // audio/video, headphones
	classDualSense = 0x002508 // peripheral, gamepad
	classKeyboard  = 0x000540 // peripheral, keyboard
	classMouse     = 0x000580 // peripheral, pointing device
	classSpeaker   = 0x240414 // audio/video, loudspeaker
	classPhone     = 0x5a020c // phone, smartphone
)

func TestTheClassOfDeviceGivesTheKind(t *testing.T) {
	for _, c := range []struct {
		name  string
		class uint32
		want  battery.Kind
	}{
		{"headphones", classQC35, battery.KindHeadset},
		{"gamepad", classDualSense, battery.KindGamepad},
		{"keyboard", classKeyboard, battery.KindKeyboard},
		{"mouse", classMouse, battery.KindMouse},
		{"a loudspeaker is not worn", classSpeaker, battery.KindOther},
		{"phone", classPhone, battery.KindOther},
	} {
		assert.Equal(t, c.want, classKind(c.class), c.name)
	}
}

// headphones are the nodes Windows made for a pair of headphones: the device's
// own node, which is connected and has no battery, and the Hands-Free AG
// profile node, which has the battery and the Device ID and is not connected.
// The shape measured on a QC35 in spec 016.
func headphones(level int, connected bool) []btNode {
	return []btNode{
		{id: `BTHENUM\DEV_AABBCCDDEE01\8&0&BLUETOOTHDEVICE_AABBCCDDEE01`, container: "{c1}",
			name: "Some Headphones", address: "AABBCCDDEE01", class: classQC35, hasClass: true, connected: connected},
		{id: `BTHENUM\{0000111E-0000-1000-8000-00805F9B34FB}_VID&0001009E_PID&400C\8&0&AABBCCDDEE01_C00000000`,
			container: "{c1}", name: "Some Headphones Hands-Free AG", address: "AABBCCDDEE01",
			class: classQC35, hasClass: true, level: level, hasLevel: true, vendor: 0x009e, product: 0x400c},
	}
}

// A device's battery is on a profile node and its name, class and connection
// on its own; they are one Device, joined by container, with the device's own
// name rather than the profile's.
func TestTheBatteryOnAProfileNodeIsTheDevices(t *testing.T) {
	got := joinNodes(headphones(90, true))
	require.Len(t, got, 1)
	d := got[0]
	assert.Equal(t, "Some Headphones", d.Name)
	assert.True(t, d.HasLevel)
	assert.Equal(t, 90, d.Level)
	assert.Equal(t, battery.KindHeadset, d.Kind)
	assert.True(t, d.Audio)
	assert.False(t, d.Apple, "the accessory protocol is not spoken on Windows")
	assert.Equal(t, uint16(0x009e), d.Vendor)
	assert.Equal(t, uint16(0x400c), d.Product)
	assert.Contains(t, d.Path, `\DEV_`, "the path is the device's node, which a read finds it again by")
}

// Windows keeps a paired device's nodes, and its last level, while it is
// switched off. A device that is not connected is not listed, as BlueZ lists
// only connected devices.
func TestADeviceNotConnectedIsNotListed(t *testing.T) {
	assert.Empty(t, joinNodes(headphones(90, false)))
}

// A connected device with no battery anywhere is listed without a level, as
// on Linux; the driver leaves it out.
func TestAConnectedDeviceWithNoBatteryHasNoLevel(t *testing.T) {
	nodes := []btNode{
		{id: `BTHENUM\DEV_AABBCCDDEE02\8&0&BLUETOOTHDEVICE_AABBCCDDEE02`, container: "{c2}",
			name: "A Gamepad", address: "AABBCCDDEE02", class: classDualSense, hasClass: true, connected: true},
		{id: `BTHENUM\{00001124-0000-1000-8000-00805F9B34FB}_VID&0002054C_PID&0CE6\8&0&AABBCCDDEE02_C00000000`,
			container: "{c2}", vendor: 0x054c, product: 0x0ce6},
	}
	got := joinNodes(nodes)
	require.Len(t, got, 1)
	assert.False(t, got[0].HasLevel)
	assert.Equal(t, uint16(0x054c), got[0].Vendor)
}

// Two devices are kept apart by their containers: one's battery is never
// another's.
func TestContainersKeepDevicesApart(t *testing.T) {
	other := []btNode{{id: `BTHENUM\DEV_AABBCCDDEE03\8&0&BLUETOOTHDEVICE_AABBCCDDEE03`, container: "{c3}",
		name: "A Keyboard", class: classKeyboard, hasClass: true, connected: true}}
	got := joinNodes(append(headphones(40, true), other...))
	require.Len(t, got, 2)
	for _, d := range got {
		if d.Name == "A Keyboard" {
			assert.False(t, d.HasLevel)
			assert.Equal(t, battery.KindKeyboard, d.Kind)
		}
	}
}

// A level outside 0-100 is no level: the property is a byte, and a byte that
// is not a percentage says nothing true.
func TestALevelOutOfRangeIsNoLevel(t *testing.T) {
	got := joinNodes(headphones(255, true))
	require.Len(t, got, 1)
	assert.False(t, got[0].HasLevel)
}
