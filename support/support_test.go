package support_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ushineko/sanshoku/support"
)

// Lookup prefers the device that confirmed a protocol to the protocol family
// it belongs to, falls back to the family, matches a hwmon chip by name, falls
// back last to a driver-wide family with no vendor, and finds nothing for a
// device no entry covers.
func TestLookupPrecedence(t *testing.T) {
	entries := []support.Entry{
		{Driver: "logitech", Device: "any HID++ device", Vendor: 0x046d},
		{Driver: "logitech", Device: "G502 via Lightspeed", Vendor: 0x046d, Products: []uint16{0xc547}},
		{Driver: "hwmon", Device: "Intel CPU package", Chips: []string{"coretemp"}},
		{Driver: "bluez", Device: "any connected device with Battery1"},
		{Driver: "bluez", Device: "A Keyboard", Vendor: 0x05ac, Products: []uint16{0x0267}},
	}
	cases := []struct {
		name    string
		driver  string
		vendor  uint16
		product uint16
		chip    string
		want    string
		found   bool
	}{
		{"the exact product wins over the family", "logitech", 0x046d, 0xc547, "Logitech USB Receiver", "G502 via Lightspeed", true},
		{"another product of the vendor is the family", "logitech", 0x046d, 0xc548, "Logitech USB Receiver", "any HID++ device", true},
		{"a hwmon chip by name", "hwmon", 0, 0, "coretemp", "Intel CPU package", true},
		{"nothing covers it", "hwmon", 0, 0, "nvme", "", false},
		{"a driver-wide family covers any vendor of its driver", "bluez", 0x054c, 0x0f8a, "WH-1000XM6", "any connected device with Battery1", true},
		{"a driver-wide family covers a device with no vendor", "bluez", 0, 0, "Bluetooth device", "any connected device with Battery1", true},
		{"the exact product wins over the driver-wide family", "bluez", 0x05ac, 0x0267, "A Keyboard", "A Keyboard", true},
		{"a driver-wide family covers no other driver's devices", "logitech", 0x1234, 0x0001, "Something", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e, ok := support.Lookup(entries, c.driver, c.vendor, c.product, c.chip)

			assert.Equal(t, c.found, ok)
			assert.Equal(t, c.want, e.Device)
		})
	}
}
