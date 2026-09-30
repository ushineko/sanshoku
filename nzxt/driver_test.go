package nzxt

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/cooling"
	"github.com/ushineko/sanshoku/hidraw"
	"github.com/ushineko/sanshoku/screen"
)

// krakenNode is the Kraken Elite's control node as Nodes reports it.
var krakenNode = hidraw.Node{
	Path: "/dev/hidraw6", Name: "NZXT Kraken Elite V2",
	Vendor: vendor, Product: 0x3012, Bus: hidraw.BusUSB,
	USBPath: "/dev/bus/usb/001/013",
}

// The descriptors' first items, written by hand: a vendor-defined page and
// the generic desktop page a keyboard collection declares.
var (
	vendorPage  = []byte{0x06, 0x00, 0xff, 0x09, 0x01, 0xa1, 0x01, 0xc0}
	desktopPage = []byte{0x05, 0x01, 0x09, 0x06, 0xa1, 0x01, 0xc0}
)

// sysfs points hidraw at a tree the test writes, with one USB device above
// every node.
func sysfs(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	sys := hidraw.SysRoot
	hidraw.SysRoot = root
	t.Cleanup(func() { hidraw.SysRoot = sys })
	return root
}

func writeNode(t *testing.T, root, name, product string, desc []byte) {
	t.Helper()
	dir := filepath.Join(root, name, "device")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	uevent := "HID_ID=0003:00001E71:0000" + product + "\nHID_NAME=NZXT Kraken Elite V2\nHID_PHYS=usb-0000:00:14.0-10/input1\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "uevent"), []byte(uevent), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "report_descriptor"), desc, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, name, "busnum"), []byte("1\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, name, "devnum"), []byte("13\n"), 0o600))
}

/*
Find tries the vendor-defined interface first, whatever its number.

sysfs cannot tell one of a device's hidraw nodes from another, and the listing
is lexical, so hidraw10 would be tried before hidraw7. The vendor-defined page
is the discriminator HID provides. An ordering, not a filter: the page says what
an interface is for, and only the device can say which one will answer.
*/
func TestFindTriesTheVendorDefinedInterfaceFirst(t *testing.T) {
	root := sysfs(t)
	writeNode(t, root, "hidraw10", "3012", desktopPage)
	writeNode(t, root, "hidraw7", "3012", vendorPage)

	found, err := Driver{}.Find(context.Background())

	require.NoError(t, err)
	require.Len(t, found, 2)
	assert.Equal(t, "/dev/hidraw7", found[0].Path, "the vendor-defined interface was not tried first")
	assert.Equal(t, "/dev/hidraw10", found[1].Path)
	assert.Equal(t, driverName, found[0].Driver)
}

// A machine with no NZXT node is absence, not a fault.
func TestFindWithNoCoolerIsAbsent(t *testing.T) {
	sysfs(t)
	_, err := Driver{}.Find(context.Background())
	require.ErrorIs(t, err, sanshoku.ErrAbsent)
}

/*
A product Known does not name is found, refused by name, and never opened.

The protocol was read off one product on one firmware. A cooler that merely
shares NZXT's vendor ID is a different device, and guessing at somebody's pump
is worse than saying so.
*/
func TestAnUnknownProductIsFoundAndNeverOpened(t *testing.T) {
	root := sysfs(t)
	writeNode(t, root, "hidraw7", "2007", vendorPage) // a Kraken X53

	found, err := Driver{}.Find(context.Background())
	require.NoError(t, err)
	require.Len(t, found, 1, "an unknown product is still a candidate, so it can be seen")

	n := krakenNode
	n.Product = 0x2007
	c := candidate(n, Driver{}.options(), func(string) (node, error) {
		t.Fatal("an unknown product was opened")
		return nil, nil
	}, nil)
	dev, err := c.Open(context.Background())
	require.ErrorIs(t, err, sanshoku.ErrUnsupported)
	assert.Nil(t, dev)
}

/*
A node that does not answer the status probe is closed and reported absent,
naming the node.

This is how the PSU case is refused: a Corsair power supply answered hotaru's
probe with its own prefix, which is not a `75 01`, which is silence here.
*/
func TestASilentNodeIsClosedAndAbsent(t *testing.T) {
	f := newFake()
	f.Silent = true
	c := candidate(krakenNode, Driver{}.options(), func(string) (node, error) { return f, nil }, nil)

	dev, err := c.Open(context.Background())

	require.ErrorIs(t, err, sanshoku.ErrAbsent)
	assert.ErrorContains(t, err, "/dev/hidraw6")
	assert.Nil(t, dev)
	assert.True(t, f.closed, "a node that did not answer was left open")
}

// A device satisfies screen.Panel only when its model has a panel and its USB
// node was found; either way it satisfies cooling.Source.
func TestThePanelIsOfferedOnlyWhereItCanBeReached(t *testing.T) {
	for name, c := range map[string]struct {
		usb   string
		panel bool
	}{
		"with the USB node":    {krakenNode.USBPath, true},
		"without the USB node": {"", false},
	} {
		t.Run(name, func(t *testing.T) {
			n := krakenNode
			n.USBPath = c.usb
			cand := candidate(n, Driver{}.options(), func(string) (node, error) { return newFake(), nil }, nil)
			dev, err := cand.Open(context.Background())
			require.NoError(t, err)
			_, isSource := dev.(cooling.Source)
			_, isPanel := dev.(screen.Panel)
			assert.True(t, isSource)
			assert.Equal(t, c.panel, isPanel)
			assert.Equal(t, n.Path, dev.Identity().Path)
		})
	}
}

/*
A panel that cannot be claimed is screen.ErrNoPanel and sanshoku.ErrAbsent, and
telemetry carries on.

The claim is lazy: nothing is claimed at Open, so a machine without the udev
rule for the USB node still reads its coolant.
*/
func TestAPanelThatCannotBeClaimedIsNoPanelAndTelemetryWorks(t *testing.T) {
	claimed := 0
	cand := candidate(krakenNode, Driver{}.options(), func(string) (node, error) { return newFake(), nil },
		func(string) (bulk, error) { claimed++; return nil, syscall.EACCES })

	dev, err := cand.Open(context.Background())
	require.NoError(t, err)
	assert.Zero(t, claimed, "the panel was claimed at Open")

	err = dev.(screen.Panel).Readout(context.Background())
	require.ErrorIs(t, err, screen.ErrNoPanel)
	require.ErrorIs(t, err, sanshoku.ErrAbsent)
	assert.True(t, sanshoku.IsPermission(err), "the kernel's reason is kept underneath")

	_, err = dev.(cooling.Source).Status(context.Background())
	assert.NoError(t, err)
	require.NoError(t, dev.Close())
}

/*
Concurrent readers share one exchange.

hotaru's API, dashboard and tray asked in the same second and want the same
number, and the device should be written to once. Superseding is wrong here: a
reader whose request was dropped still wants an answer.
*/
func TestConcurrentReadersShareOneExchange(t *testing.T) {
	f := newFake()
	d := withFake(f)

	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			_, err := d.Status(context.Background())
			assert.NoError(t, err)
		})
	}
	wg.Wait()

	assert.Len(t, f.Told, 1, "the cooler was asked more than once for one moment's reading")
}

// Freshness is a coalescing window, not a cache: past it, the device is asked
// again.
func TestAStaleReadingIsTakenAgain(t *testing.T) {
	f := newFake()
	d := withFake(f)
	d.fresh = time.Millisecond

	_, err := d.Status(context.Background())
	require.NoError(t, err)
	time.Sleep(5 * time.Millisecond)
	_, err = d.Status(context.Background())
	require.NoError(t, err)

	assert.Len(t, f.Told, 2)
}

// A failed reading is not remembered: the next caller may be somebody who has
// just plugged the cooler back in.
func TestAFailedReadingIsNotRemembered(t *testing.T) {
	f := newFake()
	f.Silent = true
	d := withFake(f)

	_, err := d.Status(context.Background())
	require.Error(t, err)

	f.mu.Lock()
	f.Silent = false
	f.mu.Unlock()
	s, err := d.Status(context.Background())
	require.NoError(t, err, "a transient failure was cached")
	assert.InDelta(t, 37.5, s.Coolant, 0.05)
}

// Two callers writing to one interrupt endpoint interleave control transfers,
// which corrupts rather than merely delaying, so every exchange is serialised.
// Overlapping exchanges on the fake drain each other's replies and fail.
func TestReadsAreSerialised(t *testing.T) {
	f := newFake()
	d := withFake(f)
	d.fresh = time.Nanosecond

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			_, err := d.Status(context.Background())
			assert.NoError(t, err, "an exchange was lost, which means two overlapped")
		})
	}
	wg.Wait()

	assert.Len(t, f.Told, 50)
}

// A closed device says so rather than writing to a closed file.
func TestAClosedDeviceIsClosed(t *testing.T) {
	f := newFake()
	d := withFake(f)
	require.NoError(t, d.Close())
	require.NoError(t, d.Close(), "a second Close is not an error")

	_, err := d.Status(context.Background())
	require.True(t, errors.Is(err, os.ErrClosed))
	assert.True(t, f.closed)
}
