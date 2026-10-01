package steelseries

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/battery"
	"github.com/ushineko/sanshoku/hidraw"
)

/*
fake is a SteelSeries control endpoint that is not one, carrying only the
behaviours spec 003 R3.2 and spec 006 R4.2 list:

  - It answers only the commands in reply, keyed by the request's second
    byte (the command, after the Apex's report number or the base station's
    report ID), with the reply as given. A command with no entry is silence,
    which is what the Apex does on the form its connection does not take.
  - queued holds a packet that is not an answer to anything, as a late reply
    to an earlier question is: the echo is how the answer is told from it.
  - A feature report is the Apex's lighting frame (spec 010): it is kept in
    features, and answered with ack when ack is set, as the keyboard answers
    every frame with an input report that starts 0x61.
  - With t set, any write or feature report fails the test. "Wrote and got nothing back" is the
    outcome that hid hayami's Arctis being sent 0x92 on every poll.

Silence costs nothing here: the fake reports the deadline at once rather than
waiting it out.
*/
type fake struct {
	t       *testing.T
	reply   map[byte][]byte
	queued  [][]byte
	asked   []byte
	closed  bool
	drained int

	ack      []byte
	features [][]byte
}

func (f *fake) SetFeature(_ context.Context, report []byte) error {
	if f.t != nil {
		f.t.Fatalf("a device this driver may not speak to was sent a feature report: % x", report[:2])
	}
	f.features = append(f.features, append([]byte(nil), report...))
	if f.ack != nil {
		f.queued = append(f.queued, f.ack)
	}
	return nil
}

func (f *fake) Write(req []byte) error {
	if f.t != nil {
		f.t.Fatalf("a device this driver may not speak to was written to: % x", req[:2])
	}
	cmd := req[1]
	f.asked = append(f.asked, cmd)
	if answer, ok := f.reply[cmd]; ok {
		f.queued = append(f.queued, answer)
	}
	return nil
}

func (f *fake) Read(ctx context.Context, buf []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if len(f.queued) == 0 {
		return 0, fmt.Errorf("no report from the fake: %w", context.DeadlineExceeded)
	}
	next := f.queued[0]
	f.queued = f.queued[1:]
	return copy(buf, next), nil
}

func (f *fake) Close() error { f.closed = true; return nil }

// Drain discards what is queued, as the kernel's queue is emptied. drained
// counts the calls so a test can say the driver asked for it.
func (f *fake) Drain(int) { f.drained++; f.queued = nil }

// apexNode is the Apex's control endpoint as Nodes reports it.
var apexNode = hidraw.Node{
	Path: "/dev/hidraw5", Name: "SteelSeries Apex Pro TKL Wireless Gen 3",
	Vendor: steelseriesVendor, Product: 0x1644, Bus: hidraw.BusUSB,
}

/*
A device that does not answer the wired form is asked the wireless one, and a
stale packet ahead of the answer is not read as it.

The two forms differ by rivalcfg's `_WIRELESS_FLAG`, and which one a device
wants depends on whether it is on the cable or behind the dongle, which the
driver cannot tell from the product ID, because that moves too. The region
reply queued ahead is the shape of the late `d2 14` that cost hayami's spec 016
an afternoon.
*/
func TestTheWirelessFormIsAskedWhenTheWiredOneIsNotAnswered(t *testing.T) {
	f := &fake{
		reply:  map[byte][]byte{batteryCommand | wirelessFlag: {batteryCommand | wirelessFlag, 0x14}},
		queued: [][]byte{{0xf5, 0x00, 0x03}},
	}
	c := candidate(apexNode, 20*time.Millisecond, func(string) (node, error) { return f, nil })

	dev, err := c.Open(context.Background())
	require.NoError(t, err)
	found, err := dev.(battery.Source).Batteries(context.Background())

	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, 95, found[0].Level)
	assert.Equal(t, battery.Discharging, found[0].State)
	assert.Equal(t, "Apex Pro TKL Wireless Gen 3", found[0].Name)
	assert.Equal(t, battery.KindOther, found[0].Kind)
	assert.Equal(t, []byte{batteryCommand, batteryCommand | wirelessFlag}, f.asked,
		"the wired form is asked first and the wireless one only after it fails")
	require.NoError(t, dev.Close())
	assert.True(t, f.closed)
}

// novaProNode is the Arctis Nova Pro Wireless X base station's 0xFFC0
// endpoint as Nodes reports it.
var novaProNode = hidraw.Node{
	Path: "/dev/hidraw14", Name: "SteelSeries Arctis Nova Pro Wireless",
	Vendor: steelseriesVendor, Product: 0x12E5, Bus: hidraw.BusUSB,
}

/*
The base station is asked `06 b0`, once, and the reply that echoes it is read
past a report that does not.

The stale report is 0x07 with 0xb0 after it: the base station's descriptor
declares report 0x07 on the same node, so a match on the second byte alone
would take it. Nothing but the one 31-byte battery request is written.
*/
func TestTheNovaProIsAskedItsOwnQuestionAndMatchedOnTheEcho(t *testing.T) {
	f := &fake{
		reply:  map[byte][]byte{novaProBattery: novaProProbe},
		queued: [][]byte{{0x07, novaProBattery, 0x00, 0x00, 0x00, 0x00, 0x08}},
	}
	c := candidate(novaProNode, 20*time.Millisecond, func(string) (node, error) { return f, nil })

	dev, err := c.Open(context.Background())
	require.NoError(t, err)
	found, err := dev.(battery.Source).Batteries(context.Background())

	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.True(t, found[0].HasLevel)
	assert.Equal(t, 75, found[0].Level)
	assert.Equal(t, battery.Discharging, found[0].State)
	assert.Equal(t, battery.KindHeadset, found[0].Kind)
	assert.Equal(t, "Arctis Nova Pro Wireless", found[0].Name)
	assert.Equal(t, []byte{novaProBattery}, f.asked, "one question, and only the battery")
	require.NoError(t, dev.Close())
}

/*
A SteelSeries product the allow-list does not name is found, refused by name,
and never written to; so is one of the listed and unimplemented legacy family.

What this guards is how the Arctis Nova Pro Wireless, 1038:12E5, was treated
before spec 006 named it: a real device that declares the 0xFFC0 page and
speaks a different protocol entirely, which hayami sent 0x92 on every poll
(hayami issue #62). It never answered, which is not the same as the command
being safe. The Nova Pro is in the allow-list now, so the product here is an ID
the table does not name. The fake fails the test on any write, and the open
function on being called at all.
*/
func TestAnUnlistedProductIsFoundAndNeverWrittenTo(t *testing.T) {
	root := t.TempDir()
	sys := hidraw.SysRoot
	hidraw.SysRoot = root
	t.Cleanup(func() { hidraw.SysRoot = sys })
	dir := filepath.Join(root, "hidraw14", "device")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	uevent := "HID_ID=0003:00001038:00001234\nHID_NAME=SteelSeries Unlisted Device\nHID_PHYS=usb-0000:00:14.0-2/input4\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "uevent"), []byte(uevent), 0o600))
	// Usage Page (0xFFC0), Usage (1), Collection (Application), End Collection.
	desc := []byte{0x06, 0xc0, 0xff, 0x09, 0x01, 0xa1, 0x01, 0xc0}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "report_descriptor"), desc, 0o600))

	found, err := Driver{}.Find(context.Background())
	require.NoError(t, err)
	require.Len(t, found, 1, "an unlisted product is still a candidate, so it can be seen")

	nodes, err := hidraw.Nodes(steelseriesVendor, hidraw.UsagePage(controlPage))
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	legacy := nodes[0]
	legacy.Product, legacy.Name = 0x1830, "SteelSeries Rival 3 Wireless"

	for _, n := range []hidraw.Node{nodes[0], legacy} {
		f := &fake{t: t}
		c := candidate(n, 20*time.Millisecond, func(string) (node, error) {
			t.Fatalf("product %04x was opened", n.Product)
			return f, nil
		})
		dev, err := c.Open(context.Background())
		require.ErrorIs(t, err, sanshoku.ErrUnsupported, "product %04x", n.Product)
		assert.Nil(t, dev)
		assert.Contains(t, err.Error(), n.Name, "the refusal names the device")
		assert.Empty(t, f.asked)
	}
}

/*
A handle held across polls receives every other program's replies too, so a
same-prefix report from before the question would be read as its answer and
the reading frozen at the state of the headset when the handle was opened
(issue #24). The driver drains before it asks.
*/
func TestAHeldBaseStationIsDrainedBeforeItIsAsked(t *testing.T) {
	stale := append([]byte{novaProReport, novaProBattery, 0, 0, 1, 0, 0, 8, 0x0a, 0, 0, 0x0a, 4, 0, 4, novaProOffline}, make([]byte, 48)...)
	fresh := append([]byte{novaProReport, novaProBattery, 0, 0, 1, 0, 6, 8, 0x0a, 0, 0, 0x0a, 4, 0, 8, 0x08}, make([]byte, 48)...)
	f := &fake{reply: map[byte][]byte{novaProBattery: fresh}, queued: [][]byte{stale, stale, stale}}
	d := &device{id: sanshoku.Identity{Name: "SteelSeries Arctis Nova Pro Wireless"}, family: novaPro, timeout: time.Second, rd: f}

	got, err := d.Batteries(context.Background())

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.True(t, got[0].HasLevel, "the stale off report was read as the answer")
	assert.Equal(t, 75, got[0].Level)
	assert.Equal(t, 1, f.drained, "the driver did not drain before asking")
}
