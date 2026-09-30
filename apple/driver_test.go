package apple

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/battery"
	"github.com/ushineko/sanshoku/bluez"
)

// anAddress is a printed Bluetooth address, in either separator BlueZ uses.
var anAddress = regexp.MustCompile(`(?i)[0-9a-f]{2}([:_][0-9a-f]{2}){5}`)

/*
fakeChannel answers the exchange the way a device does, carrying only the
behaviours spec 004 R4.2 lists and hayami measured: the three-step handshake,
the chatter the device sends between the steps, and silence. A refused dial is
the dialer's, below.

With no reply left, or silent, it waits for the context's deadline and reports
it, as l2cap.Conn.Receive does.
*/
type fakeChannel struct {
	sent    [][]byte
	replies [][]byte
	silent  bool
	closed  bool
}

func (f *fakeChannel) Send(p []byte) error {
	f.sent = append(f.sent, append([]byte(nil), p...))
	return nil
}

func (f *fakeChannel) Receive(ctx context.Context) ([]byte, error) {
	if f.silent || len(f.replies) == 0 {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	r := f.replies[0]
	f.replies = f.replies[1:]
	return r, nil
}

func (f *fakeChannel) Close() error { f.closed = true; return nil }

// answering is a device that completes the exchange with the given packet.
func answering(t *testing.T, hexPacket string) (dialer, *fakeChannel) {
	t.Helper()
	c := &fakeChannel{replies: [][]byte{
		packet(t, "01000400000001"),
		packet(t, "040004002b000144"),
		packet(t, hexPacket),
	}}
	return func(context.Context, [6]byte, uint16) (channel, error) { return c, nil }, c
}

// refusing is a device that will not open a channel, which is what AirPods in
// a pocket do.
func refusing(context.Context, [6]byte, uint16) (channel, error) {
	return nil, errors.New("connecting to l2cap port 0x1001: connection refused")
}

const airpodsPath = "/org/bluez/hci0/dev_AA_BB_CC_DD_EE_FF"

func airpods(level int, has bool) bluez.Device {
	return bluez.Device{
		Path: airpodsPath, Name: "Someone's AirPods", Address: "AA:BB:CC:DD:EE:FF",
		Vendor: 0x004c, Product: 0x200e, Apple: true, Audio: true,
		Level: level, HasLevel: has, Kind: battery.KindHeadset,
	}
}

// open finds the one Apple candidate over a fixed list and a dialer, and
// opens it.
func open(t *testing.T, devices *[]bluez.Device, dial dialer) battery.Source {
	t.Helper()
	list := func(context.Context) ([]bluez.Device, error) { return *devices, nil }
	found, err := find(context.Background(), 50*time.Millisecond, list, dial)
	require.NoError(t, err)
	require.Len(t, found, 1)
	dev, err := found[0].Open(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { _ = dev.Close() })
	return dev.(battery.Source)
}

/*
The exchange is walked through in order, and the chatter in between — the
device has a great deal to say about its firmware and its serial numbers — is
waited through rather than mistaken for an answer.
*/
func TestTheExchangeIsWalkedThroughAndTheChatterIgnored(t *testing.T) {
	c := &fakeChannel{replies: [][]byte{
		packet(t, "01000400000001"),           // handshake ack
		packet(t, "040004000c00d443010101"),   // chatter
		packet(t, "040004002b000144000b0705"), // features ack
		packet(t, "0400040009000d03000000"),   // more chatter
		packet(t, airpodsPacket),
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cells, err := readAAP(ctx, c)
	require.NoError(t, err)
	require.Len(t, cells, 2)

	require.Len(t, c.sent, 3, "the exchange did not send all three of its steps")
	assert.Equal(t, aapHandshake, c.sent[0])
	assert.Equal(t, aapSetFeatures, c.sent[1])
	assert.Equal(t, aapNotifications, c.sent[2])
}

// A device that connects and never reports gives up at the deadline, as
// silence, rather than holding the caller's poll loop.
func TestADeviceThatNeverReportsGivesUpAtItsDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, err := readAAP(ctx, &fakeChannel{silent: true})

	require.ErrorIs(t, err, ErrNoBatteryPacket)
	assert.Less(t, time.Since(started), 2*time.Second)
}

// A device that has both is read once, from the accessory protocol, because
// that is the reading with the cells in it; the channel is closed after.
func TestADeviceWithBothIsReadFromTheAccessoryProtocol(t *testing.T) {
	devices := []bluez.Device{airpods(42, true)}
	dial, c := answering(t, airpodsPacket)
	src := open(t, &devices, dial)

	got, err := src.Batteries(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 80, got[0].Level, "the BlueZ percentage won over the accessory protocol")
	assert.Len(t, got[0].Cells, 2)
	assert.Equal(t, battery.KindHeadset, got[0].Kind)
	assert.True(t, c.closed)
}

// When the channel is refused but BlueZ has a level, the level is the
// reading. Better a percentage than nothing.
func TestARefusedDialFallsBackToBatteryOne(t *testing.T) {
	devices := []bluez.Device{airpods(42, true)}
	src := open(t, &devices, refusing)

	got, err := src.Batteries(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 42, got[0].Level)
	assert.Empty(t, got[0].Cells)
}

// A device that answers neither is no reading. A refusal is reported, and
// silence is not.
func TestADeviceThatAnswersNeitherIsNoReading(t *testing.T) {
	devices := []bluez.Device{airpods(0, false)}
	src := open(t, &devices, refusing)
	got, err := src.Batteries(context.Background())
	assert.Empty(t, got)
	require.Error(t, err)
	assert.NotErrorIs(t, err, sanshoku.ErrAbsent)

	silent := func(context.Context, [6]byte, uint16) (channel, error) { return &fakeChannel{silent: true}, nil }
	src = open(t, &devices, silent)
	got, err = src.Batteries(context.Background())
	assert.Empty(t, got)
	require.NoError(t, err, "silence is a reading withheld, not a failure")
}

// Only Apple audio is a candidate, so nothing else is ever dialled: opening
// an L2CAP channel to a keyboard is a thing to not do.
func TestOnlyAppleAudioIsACandidate(t *testing.T) {
	devices := []bluez.Device{
		{Name: "A Keyboard", Address: "AA:BB:CC:DD:EE:01", Level: 54, HasLevel: true},
		{Name: "An Apple Phone", Address: "AA:BB:CC:DD:EE:02", Apple: true},
		{Name: "Some Speakers", Address: "AA:BB:CC:DD:EE:03", Audio: true, Level: 20, HasLevel: true},
	}
	list := func(context.Context) ([]bluez.Device, error) { return devices, nil }
	_, err := find(context.Background(), time.Second, list, refusing)
	require.ErrorIs(t, err, sanshoku.ErrAbsent)

	devices = append(devices, airpods(0, false))
	found, err := find(context.Background(), time.Second, list, refusing)
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, sanshoku.Identity{
		Vendor: 0x004c, Product: 0x200e, Bus: sanshoku.BusBluetooth,
		Name: "Someone's AirPods", Phys: "AA:BB:CC:DD:EE:FF", Path: airpodsPath,
	}, found[0].Identity, "Apple keys on vendor 004c; the address is in Phys")
}

/*
No error this driver returns names the device or carries its address.

The repository and its bug tracker are public, and an alias is often a name
somebody chose. Every error path is driven — a refused dial with no fallback, a
failed listing, a device gone, a closed device, an address BlueZ gave that does
not parse — and its message checked for an address and the alias.
*/
func TestNoErrorNamesTheDeviceOrItsAddress(t *testing.T) {
	var errs []error
	collect := func(src battery.Source) {
		_, err := src.Batteries(context.Background())
		errs = append(errs, err)
	}

	devices := []bluez.Device{airpods(0, false)}
	collect(open(t, &devices, refusing))

	gone := []bluez.Device{airpods(0, false)}
	src := open(t, &gone, refusing)
	gone = nil
	collect(src)

	broken := []bluez.Device{airpods(0, false)}
	src = open(t, &broken, refusing)
	src.(*device).list = func(context.Context) ([]bluez.Device, error) {
		return nil, errors.New("the bus went away")
	}
	collect(src)

	src = open(t, &devices, refusing)
	require.NoError(t, src.(sanshoku.Device).Close())
	collect(src)

	mangled := []bluez.Device{airpods(0, false)}
	mangled[0].Address = "AA:BB:CC:DD:EE:GG"
	collect(open(t, &mangled, refusing))

	for i, err := range errs {
		require.Error(t, err, "path %d", i)
		assert.NotRegexp(t, anAddress, err.Error(), "path %d", i)
		assert.NotContains(t, err.Error(), "AA:BB:CC:DD:EE", "path %d", i)
		assert.NotContains(t, err.Error(), "Someone's AirPods", "path %d", i)
	}
}
