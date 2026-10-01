package main

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
