package logitech

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/battery"
	"github.com/ushineko/sanshoku/hidraw"
)

// solaarID is solaar's software ID, which a reply to solaar carries in the low
// nibble of its function byte.
const solaarID = 0x0B

// The feature indices the fake device keeps its features at. A real device
// numbers them its own way; these are hayami's fake's.
const (
	fakeBatteryIdx = 0x06
	fakeNameIdx    = 0x07
)

/*
fake is a HID++ node that is not one, carrying only the behaviours spec 002
R7.2 lists and hayami measured:

  - The device at index 1 answers its battery read with a **long** report to a
    short request, as the G502 X PLUS on a Lightspeed receiver does.
  - Every other index is refused in the **HID++ 1.0 error form**, sub-id 0x8F,
    "unknown device", as the receiver does for an empty slot. With old set, the
    device at index 1 refuses every 2.0 request in the same form with "invalid
    sub-id" and answers register 0x07 only, as the K800 does.
  - With solaar set, a reply to solaar's request for the same feature and
    function, carrying another level, arrives ahead of each battery answer.
  - The first silent battery reads get no answer at all, as a mouse waking from
    idle does not.
  - With truncate set, the first name chunk is one stray byte, the shape of a
    reply from somebody else's conversation.

Silence costs nothing here: the fake reports the deadline at once rather than
waiting it out.
*/
type fake struct {
	level    byte
	name     string
	old      bool
	solaar   bool
	silent   int
	truncate bool

	pending  [][]byte
	batteryN int
}

// Write is a request arriving at the node: what the device would say to it is
// queued behind anything already waiting.
func (f *fake) Write(req []byte) error {
	f.pending = append(f.pending, f.respond(append([]byte(nil), req...))...)
	return nil
}

// Read hands over the next queued report, or reports the deadline as
// Handle.Read does when nothing is there. hidraw.Exchange does the skipping.
func (f *fake) Read(ctx context.Context, buf []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if len(f.pending) == 0 {
		return 0, fmt.Errorf("no report from the fake: %w", context.DeadlineExceeded)
	}
	r := f.pending[0]
	f.pending = f.pending[1:]
	return copy(buf, r), nil
}

func (f *fake) respond(req []byte) [][]byte {
	device, feature, function := req[1], req[2], req[3]
	refuse := func(code byte) [][]byte {
		return [][]byte{{reportShort, device, errorSub10, feature, function, code, 0x00}}
	}
	if device != 1 {
		return refuse(err10UnknownDevice)
	}
	if f.old {
		if feature == subGetRegister && function == registerBatteryStatus {
			return [][]byte{{reportShort, device, subGetRegister, registerBatteryStatus, 0x05, 0x00, 0x00}}
		}
		if feature == subGetRegister {
			return refuse(err10InvalidAddress)
		}
		return refuse(err10InvalidSubID)
	}

	switch {
	case feature == rootFeature:
		switch uint16(req[4])<<8 | uint16(req[5]) {
		case featureUnifiedBattery:
			return [][]byte{reportOf(reportShort, device, feature, function, fakeBatteryIdx)}
		case featureDeviceName:
			return [][]byte{reportOf(reportShort, device, feature, function, fakeNameIdx)}
		default:
			return [][]byte{reportOf(reportShort, device, feature, function, 0x00)}
		}

	case feature == fakeBatteryIdx:
		f.batteryN++
		if f.batteryN <= f.silent {
			return nil
		}
		var out [][]byte
		if f.solaar {
			out = append(out, reportOf(reportLong, device, feature, function&0xF0|solaarID, 0x51, 0x08, 0x00, 0x00))
		}
		return append(out, reportOf(reportLong, device, feature, function, f.level, 0x08, 0x00, 0x00))

	case feature == fakeNameIdx && function>>4 == functionDeviceType:
		return [][]byte{reportOf(reportShort, device, feature, function, typeMouse)}

	case feature == fakeNameIdx && function>>4 == functionDeviceName:
		return [][]byte{reportOf(reportShort, device, feature, function, byte(len(f.name)))}

	case feature == fakeNameIdx && function>>4 == functionDeviceNameChunk:
		if f.truncate {
			return [][]byte{reportOf(reportShort, device, feature, function, f.level)}
		}
		chunk := make([]byte, 16)
		copy(chunk, f.name[min(int(req[4]), len(f.name)):])
		return [][]byte{reportOf(reportLong, device, feature, function, chunk...)}
	}
	return [][]byte{{reportShort, device, errorSub20, feature, function, err20UnsupportedFeature, 0x00}}
}

// reportOf builds a reply: the kind, the device, the feature, the function
// byte and the parameters, padded to the kind's width.
func reportOf(kind, device, feature, function byte, params ...byte) []byte {
	width := 7
	if kind == reportLong {
		width = 20
	}
	r := make([]byte, width)
	r[0], r[1], r[2], r[3] = kind, device, feature, function
	copy(r[4:], params)
	return r
}

// onFake is a device on a node with the given HID_PHYS and kernel name,
// speaking through f.
func onFake(f *fake, phys, name string) *device {
	return &device{
		id:       sanshoku.Identity{Name: name},
		node:     hidraw.Node{Phys: phys, Name: name},
		timeout:  20 * time.Millisecond,
		rd:       f,
		presence: Presence{Nodes: 1},
	}
}

const (
	receiverPhys = "usb-0000:00:14.0-3/input2"
	childPhys    = "usb-0000:00:14.0-3/input2:1"
)

// A short request answered with a long report is read, and every empty index
// is refused in the 1.0 form rather than waited on.
func TestALongReplyToAShortRequestIsRead(t *testing.T) {
	d := onFake(&fake{level: 0x56, name: "G502 X PLUS"}, receiverPhys, "Logitech USB Receiver")

	found, err := d.Batteries(context.Background())

	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, "G502 X PLUS", found[0].Name)
	assert.Equal(t, 86, found[0].Level)
	assert.True(t, found[0].HasLevel)
	assert.Equal(t, battery.KindMouse, found[0].Kind)
}

/*
A reading survives a reply to solaar and four silent attempts.

A mouse idle for six seconds needed four attempts on hayami's receiver, and
solaar's replies share the node; the software ID is how the one is told from
ours. The level read is the device's, not the 81 % solaar was sent.
*/
func TestAReadingSurvivesASolaarReplyAndFourSilentAttempts(t *testing.T) {
	f := &fake{level: 0x56, name: "G502 X PLUS", solaar: true, silent: 4}
	d := onFake(f, receiverPhys, "Logitech USB Receiver")

	found, err := d.Batteries(context.Background())

	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, 86, found[0].Level, "solaar's reply was read as ours")
	assert.Equal(t, 5, f.batteryN, "four silent attempts and the fifth answered")
}

// A fifth silent attempt withholds the reading, and says nothing: a mouse
// asleep is a reading withheld, not an error.
func TestAFifthSilentAttemptWithholdsTheReadingWithoutAnError(t *testing.T) {
	f := &fake{level: 0x56, name: "G502 X PLUS", silent: requestAttempts}
	d := onFake(f, receiverPhys, "Logitech USB Receiver")

	found, err := d.Batteries(context.Background())

	require.NoError(t, err)
	assert.Empty(t, found)
	assert.Equal(t, requestAttempts, f.batteryN, "the request was not sent five times")
}

/*
A name shorter than the device declared is refused (hayami issue #58).

The receiver declares a name of eleven characters and hands back one: a battery
level of 81, 0x51, which is "Q". The reading carries defaultName, as hayami's
does, and never the stray byte.
*/
func TestANameShorterThanTheDeviceDeclaredIsRefused(t *testing.T) {
	d := onFake(&fake{level: 0x51, name: "G502 X PLUS", truncate: true}, receiverPhys, "Logitech USB Receiver")

	found, err := d.Batteries(context.Background())

	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, 81, found[0].Level)
	assert.Equal(t, defaultName, found[0].Name, "a stray byte was taken for a device name")
}

/*
A HID++ 1.0 device on its own node is read through its band register and named
by the kernel; the same answers on the receiver's node are not a device.

A receiver node answers for every device paired to it, so an answer there
carries the receiver's name: the K800 was reported twice in hayami, once as
itself and once as "Logitech USB Receiver".
*/
func TestAnOldDeviceIsReadOnItsOwnNodeOnly(t *testing.T) {
	child := onFake(&fake{old: true}, childPhys, "Logitech K800")
	found, err := child.Batteries(context.Background())
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, "Logitech K800", found[0].Name)
	assert.True(t, found[0].HasBand)
	assert.Equal(t, battery.BandGood, found[0].Band)
	assert.False(t, found[0].HasLevel)

	receiver := onFake(&fake{old: true}, receiverPhys, "Logitech USB Receiver")
	found, err = receiver.Batteries(context.Background())
	require.NoError(t, err)
	assert.Empty(t, found)
}
