package logitech

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku/lighting"
)

// mouse is the G502 X PLUS behind its receiver, as the bench read it, with
// no wait on a silent request.
func mouse(t *testing.T) (*device, *fake) {
	t.Helper()
	was := lightsWait
	lightsWait = 5 * time.Millisecond
	t.Cleanup(func() { lightsWait = was })
	f := &fake{level: 80, name: "G502 X PLUS", lights: true}
	return onFake(f, receiverPhys, "Logitech USB Receiver"), f
}

// sent is the lighting requests written since n, as their feature index,
// function and first parameters.
func (f *fake) sent(n int) []string {
	var out []string
	for _, r := range f.requests[n:] {
		if r[2] == 9 || r[2] == 10 || r[2] == 11 {
			out = append(out, fmt.Sprintf("%d/%d % x", r[2], r[3]>>4, r[4:7]))
		}
	}
	return out
}

func TestALongRequestIsTwentyBytes(t *testing.T) {
	f := &fake{lights: true}
	_, err := requestLong(context.Background(), f, time.Millisecond, 1, 10, keyIndividual, 0x01, 0xFF, 0x00, 0x00)
	require.NoError(t, err)
	r := f.requests[0]
	require.Len(t, r, 20)
	assert.Equal(t, []byte{reportLong, 1, 10, keyIndividual<<4 | softwareID, 0x01, 0xFF, 0x00, 0x00}, r[:8])
}

// The lit device is found by its per-key feature, and its eight zones from
// the bitmap, named as OpenRGB names the G502 X's.
func TestKeysAreTheLitDevicesZones(t *testing.T) {
	d, _ := mouse(t)
	keys := d.Keys()
	require.Len(t, keys, 8)
	assert.Equal(t, lighting.Key{ID: 0, Name: "LED A"}, keys[0])
	assert.Equal(t, lighting.Key{ID: 7, Name: "LED H"}, keys[7])
	assert.Equal(t, []lighting.Zone{{Name: "Mouse LEDs", Count: 8, Shape: lighting.ShapeSet}}, d.Zones())
	assert.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8}, d.lights.zones)
	assert.Equal(t, []byte{0x20, 0x64}, d.lights.template)
	assert.Equal(t, byte(4), d.lights.effects)
}

func TestANodeWithNoLitDeviceHasNoKeys(t *testing.T) {
	f := &fake{level: 80, name: "G502 X PLUS"}
	assert.Empty(t, onFake(f, receiverPhys, "Logitech USB Receiver").Keys())
}

/*
The first frame claims, prepares with the card's template and draws: the
effect engine to software, power, the timers, host mode, the two prep
effects, two reports of four zones, FrameEnd, then the engine fully the
host's.
*/
func TestTheFirstFrameClaimsPreparesAndDraws(t *testing.T) {
	d, f := mouse(t)
	require.NotEmpty(t, d.Keys())
	n := len(f.requests)
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 0xFF}, {ID: 7, B: 0xFF}}))
	assert.Equal(t, []string{
		"9/5 01 03 06",  // SetSWControl
		"9/8 01 01 00",  // SetRgbPowerMode
		"9/7 01 00 00",  // power configuration
		"11/1 02 00 00", // host mode
		"9/1 ff 02 00",  // prep: effect 2
		"9/1 ff 04 00",  // prep: effect 4, past cluster 0's last
		"10/1 01 ff 00", // zones 1-4, zone 1 red
		"10/1 05 00 00", // zones 5-8
		"10/7 00 00 00", // FrameEnd
		"9/5 01 03 05",  // the engine fully the host's
	}, f.sent(n))
	prep := f.requests[n+4]
	assert.Equal(t, []byte{0x20, 0x64}, prep[12:14], "the card's template")
	assert.Equal(t, byte(0x01), prep[16], "kept")
	last := f.requests[n+7]
	assert.Equal(t, []byte{0x08, 0x00, 0x00, 0xFF}, last[16:20], "zone 8 blue")

	n = len(f.requests)
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 0xFF}}))
	assert.Empty(t, f.sent(n), "an unchanged frame was sent")

	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 1, G: 0xFF}}))
	assert.Equal(t, []string{"10/1 01 ff 00", "10/1 05 00 00", "10/7 00 00 00"}, f.sent(n), "no second claim or prep")
}

func TestAFrameEndAnsweredBusyIsSentAgain(t *testing.T) {
	d, f := mouse(t)
	f.busy = 2
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 1}}))
	ends := 0
	for _, s := range f.sent(0) {
		if s[:4] == "10/7" {
			ends++
		}
	}
	assert.Equal(t, 3, ends)
}

// A mouse that answers nothing (asleep) fails the frame, and the next one
// claims it again.
func TestAFailedFrameClaimsAgain(t *testing.T) {
	d, f := mouse(t)
	require.NotEmpty(t, d.Keys())
	f.lightsSilent = true
	require.Error(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 1}}))
	f.lightsSilent = false
	n := len(f.requests)
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 1}}))
	assert.Equal(t, "9/5 01 03 06", f.sent(n)[0], "the claim again")
}

func TestReleaseHandsTheLightsBack(t *testing.T) {
	d, f := mouse(t)
	require.NoError(t, d.Release(context.Background()))
	assert.Empty(t, f.sent(0), "a handle that never drew sends nothing")
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 1}}))
	n := len(f.requests)
	require.NoError(t, d.Release(context.Background()))
	assert.Equal(t, []string{"9/5 01 00 00", "11/1 01 00 00"}, f.sent(n))
}

func TestAKeyThatIsNotALightSendsNothing(t *testing.T) {
	d, f := mouse(t)
	require.NotEmpty(t, d.Keys())
	n := len(f.requests)
	require.Error(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 8}}))
	assert.Empty(t, f.sent(n))
}

// The battery still reads on a node with lights.
func TestTheBatteryStillReads(t *testing.T) {
	d, _ := mouse(t)
	require.NotEmpty(t, d.Keys())
	b, err := d.Batteries(context.Background())
	require.NoError(t, err)
	require.Len(t, b, 1)
	assert.Equal(t, 80, b[0].Level)
}

// A HID++ 2.0 error is read from its code byte, and only an error for this
// request is this request's: another client's error is skipped.
func TestAnErrorIsMatchedToItsRequest(t *testing.T) {
	ours := []byte{reportShort, 1, errorSub20, 10, 0x7C, err20Busy, 0x00}
	theirs := []byte{reportShort, 1, errorSub20, 10, 0x77, err20Busy, 0x00}
	_, matched, err := reply(ours, 1, 10, 0x7C)
	assert.True(t, matched)
	require.ErrorIs(t, err, errBusy)
	_, matched, _ = reply(theirs, 1, 10, 0x7C)
	assert.False(t, matched, "OpenRGB's error was taken for ours")
}
