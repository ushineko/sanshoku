package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ushineko/sanshoku"
)

// The bench never prints a Bluetooth address. A BlueZ object path ends in one,
// so its segment is masked; a hidraw path is printed as it is.
func TestTheBenchMasksTheAddressInABlueZPath(t *testing.T) {
	bt := sanshoku.Identity{Bus: sanshoku.BusBluetooth, Path: "/org/bluez/hci0/dev_A1_B2_C3_d4_e5_F6"}
	assert.Equal(t, "/org/bluez/hci0/dev_XX_XX_XX_XX_XX_XX", shownPath(bt))

	hid := sanshoku.Identity{Bus: sanshoku.BusUSB, Path: "/dev/hidraw5"}
	assert.Equal(t, "/dev/hidraw5", shownPath(hid))
}
