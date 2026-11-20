package msi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/lighting"
	"github.com/ushineko/sanshoku/pawnio"
)

/*
lpc is PawnIO's LpcIO module in memory with a Nuvoton chip behind it: a slot
select, a device ID, and registers per logical device, selected by register
07. Every write is recorded as "ldn:reg=val".
*/
type lpc struct {
	devID  uint16
	slot   uint64
	ldn    byte
	regs   map[byte]map[byte]byte
	writes []string
	closed bool
}

func newLPC(devID uint16) *lpc {
	return &lpc{devID: devID, regs: map[byte]map[byte]byte{
		ldnGPIO: {regGPIOSetup: 0x0A},
		ldnRGB:  {regRGBEnable: 0x00},
	}}
}

func (l *lpc) Execute(function string, in []uint64, _ int) ([]uint64, error) {
	switch function {
	case "ioctl_select_slot":
		l.slot = in[0]
		return nil, nil
	case "ioctl_superio_inb":
		reg := byte(in[0])
		switch reg {
		case sioDeviceID:
			return []uint64{uint64(l.devID >> 8)}, nil
		case sioDeviceID + 1:
			return []uint64{uint64(l.devID & 0xFF)}, nil
		case sioLogicalDevice:
			return []uint64{uint64(l.ldn)}, nil
		}
		return []uint64{uint64(l.regs[l.ldn][reg])}, nil
	case "ioctl_superio_outb":
		reg, val := byte(in[0]), byte(in[1])
		if reg == sioLogicalDevice {
			l.ldn = val
		} else {
			if l.regs[l.ldn] == nil {
				l.regs[l.ldn] = map[byte]byte{}
			}
			l.regs[l.ldn][reg] = val
		}
		l.writes = append(l.writes, fmt.Sprintf("%02x:%02x=%02x", l.ldn, reg, val))
		return nil, nil
	}
	return nil, errors.New("no such function")
}

func (l *lpc) Close() error { l.closed = true; return nil }

// withBoard points the board driver at a DMI name and a chip.
func withBoard(t *testing.T, maker, name string, chip *lpc) {
	t.Helper()
	wasDMI, wasLPC := readDMI, openLPC
	readDMI = func() (string, string) { return maker, name }
	openLPC = func() (pawnio.Executor, error) {
		if chip == nil {
			return nil, pawnio.ErrNoPawnIO
		}
		return chip, nil
	}
	t.Cleanup(func() { readDMI, openLPC = wasDMI, wasLPC })
}

const tomahawk = "B450 TOMAHAWK MAX (MS-7C02)"

func TestFindNamesTheBoardAsOpenRGBDoes(t *testing.T) {
	withBoard(t, "Micro-Star International Co., Ltd.", tomahawk, newLPC(0xD352))
	found, err := Board{}.Find(context.Background())
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, "MSI B450 TOMAHAWK MAX (MS-7C02)", found[0].Name)

	withBoard(t, "ASUSTeK COMPUTER INC.", tomahawk, nil)
	_, err = Board{}.Find(context.Background())
	require.ErrorIs(t, err, sanshoku.ErrAbsent, "another maker")

	withBoard(t, "Micro-Star International Co., Ltd.", "MAG X870 TOMAHAWK WIFI (MS-7E51)", nil)
	_, err = Board{}.Find(context.Background())
	require.ErrorIs(t, err, sanshoku.ErrAbsent, "a board not driven this way")
}

func openTheBoard(t *testing.T, chip *lpc) (*boardDevice, error) {
	t.Helper()
	withBoard(t, "MSI", tomahawk, chip)
	found, err := Board{}.Find(context.Background())
	require.NoError(t, err)
	dev, err := found[0].Open(context.Background())
	if err != nil {
		return nil, err
	}
	return dev.(*boardDevice), nil
}

// Opening finds the chip by its device ID and writes nothing.
func TestOpenFindsTheChipAndWritesNothing(t *testing.T) {
	chip := newLPC(0xD352)
	d, err := openTheBoard(t, chip)
	require.NoError(t, err)
	assert.Empty(t, chip.writes)
	assert.Equal(t, []lighting.Key{{ID: 0, Name: "MSI LED"}}, d.Keys())
}

func TestAnotherChipIsRefused(t *testing.T) {
	chip := newLPC(0xC333) // an NCT5577D
	_, err := openTheBoard(t, chip)
	require.ErrorIs(t, err, errNoChip)
	assert.True(t, chip.closed)
	assert.Empty(t, chip.writes)

	_, err = openTheBoard(t, nil)
	require.ErrorIs(t, err, sanshoku.ErrUnsupported, "no PawnIO")
}

/*
The first frame readies the chip as OpenRGB does, then writes the colour as
four bits a channel, repeated, on the RGB logical device; the next sends only
the colour, and an unchanged one nothing.
*/
func TestAFrameReadiesTheChipThenWritesTheColour(t *testing.T) {
	chip := newLPC(0xD352)
	d, err := openTheBoard(t, chip)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, d.Frame(ctx, []lighting.Pixel{{ID: 0, R: 0xF0, G: 0x80, B: 0x10}}))
	assert.Equal(t, []string{
		"09:07=09", "09:2c=12", "12:07=12", "12:e0=e0", "12:e4=00", "12:ff=fe", "12:fd=00",
		"12:07=12",
		"12:f0=ff", "12:f1=ff", "12:f2=ff", "12:f3=ff",
		"12:f4=88", "12:f5=88", "12:f6=88", "12:f7=88",
		"12:f8=11", "12:f9=11", "12:fa=11", "12:fb=11",
	}, chip.writes)

	chip.writes = nil
	require.NoError(t, d.Frame(ctx, []lighting.Pixel{{ID: 0, R: 0xF0, G: 0x80, B: 0x10}}))
	assert.Empty(t, chip.writes)
	require.NoError(t, d.Frame(ctx, []lighting.Pixel{{ID: 0, R: 0x20}}))
	assert.Equal(t, "12:07=12", chip.writes[0], "the colour alone, its logical device first")
	assert.Len(t, chip.writes, 13)
}

// A board without inverted colours leaves the colour-order bits clear.
func TestABoardNotInvertedLeavesTheBitsClear(t *testing.T) {
	chip := newLPC(0xD452)
	withBoard(t, "MSI", "B450 GAMING PRO CARBON AC (MS-7B85)", chip)
	boards["7B85"] = false
	t.Cleanup(func() { delete(boards, "7B85") })
	found, err := Board{}.Find(context.Background())
	require.NoError(t, err)
	dev, err := found[0].Open(context.Background())
	require.NoError(t, err)
	require.NoError(t, dev.(*boardDevice).Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 1}}))
	assert.Contains(t, chip.writes, "12:ff=e2")
}

func TestReleaseAndClose(t *testing.T) {
	chip := newLPC(0xD352)
	d, err := openTheBoard(t, chip)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, d.Frame(ctx, []lighting.Pixel{{ID: 0, R: 0x10}}))
	chip.writes = nil
	require.NoError(t, d.Release(ctx))
	assert.Empty(t, chip.writes)
	require.Error(t, d.Frame(ctx, []lighting.Pixel{{ID: 1, R: 1}}))
	require.NoError(t, d.Close())
	assert.True(t, chip.closed)
	require.ErrorIs(t, d.Frame(ctx, []lighting.Pixel{{ID: 0, R: 2}}), os.ErrClosed)
}
