package nzxt

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku/screen"
)

// slot builds a bucket-query reply: occupied at an address, of a size.
func slot(index, address, size int) []byte {
	r := make([]byte, reportLen)
	r[0], r[1] = 0x31, 0x04
	r[14] = byte(index)
	r[15] = byte(index + 1) // the asset index, which is what marks it used
	r[16] = 0x02
	r[17], r[18] = byte(address&0xFF), byte(address>>8)
	r[19], r[20] = byte(size&0xFF), byte(size>>8)
	r[21], r[22] = 0x01, 0x01
	return r
}

// blank is an unoccupied slot: everything from byte 15 is zero.
func blank() []byte {
	r := make([]byte, reportLen)
	r[0], r[1] = 0x31, 0x04
	return r
}

// Placement, from hotaru's screen_test.go: which slot is free, and where in the
// device's memory an image may go.
func TestPlacement(t *testing.T) {
	t.Run("an empty slot is preferred", func(t *testing.T) {
		assert.Equal(t, 2, free([][]byte{slot(0, 0, 7), slot(1, 22, 22), blank(), slot(3, 66, 22)}, -1))
	})
	t.Run("the slot on screen is never free", func(t *testing.T) {
		// The transfer takes about a second and the panel keeps showing its
		// current slot throughout; writing into it blanks the screen.
		assert.Equal(t, 2, free([][]byte{slot(0, 0, 7), blank(), blank()}, 1))
	})
	t.Run("a full screen reports no free slot", func(t *testing.T) {
		// -1 rather than 0: "all taken" leads to clearing one, and a caller
		// that cannot tell the two apart writes over an image nobody asked it to.
		assert.Equal(t, -1, free([][]byte{slot(0, 0, 7), slot(1, 22, 22)}, -1))
	})
	t.Run("a slot is vacant only when everything from byte 15 is zero", func(t *testing.T) {
		assert.True(t, vacant(blank()))
		assert.False(t, vacant(slot(0, 0, 7)))
	})

	cases := []struct {
		name    string
		slots   [][]byte
		bucket  int
		packets int
		address int
		ok      bool
	}{
		// The device refuses an address it did not arrive at itself (0x04),
		// so reusing a slot's own space is the first and cheapest answer.
		{"an image that still fits keeps its address", [][]byte{slot(0, 40, 20), slot(1, 100, 10)}, 0, 15, 40, true},
		{"a growing image goes after everything else", [][]byte{slot(0, 0, 10), slot(1, 10, 30)}, 0, 25, 40, true},
		{"an image high up that fits stays there", [][]byte{slot(0, capacity-20, 20)}, 0, 15, capacity - 20, true},
		{"room at the start is used before giving up", [][]byte{slot(0, 300, 5), slot(1, 305, capacity-310)}, 0, 100, 0, true},
		// Bigger than its own slot, overlapping its neighbours, with room
		// neither above nor below: the caller clears the screen.
		{"memory too full to place anything says so", [][]byte{slot(0, 0, 10), slot(1, 10, capacity-10)}, 0, capacity, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			address, ok := place(c.slots, c.bucket, c.packets)
			assert.Equal(t, c.ok, ok)
			assert.Equal(t, c.address, address)
		})
	}
}

// recorder is the panel's bulk interface, recording what was written.
type recorder struct {
	mu        sync.Mutex
	endpoints []byte
	lengths   []int
	first     []byte
	closed    bool
}

func (r *recorder) Bulk(_ context.Context, endpoint byte, data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.endpoints = append(r.endpoints, endpoint)
	r.lengths = append(r.lengths, len(data))
	if r.first == nil {
		r.first = append([]byte(nil), data...)
	}
	return nil
}

func (r *recorder) Close() error { r.closed = true; return nil }

// withPanel is an open panel device over a fake control channel and a
// recording bulk interface.
func withPanel(f *fake, r *recorder) *panelDevice {
	d := withFake(f)
	d.usbPath = krakenNode.USBPath
	d.claim = func(string) (bulk, error) { return r, nil }
	return &panelDevice{d}
}

// card is a one-frame GIF of the given size.
func card(w, h int) *gif.GIF {
	pal := color.Palette{color.RGBA{10, 10, 20, 255}, color.RGBA{240, 200, 60, 255}}
	img := image.NewPaletted(image.Rect(0, 0, w, h), pal)
	for y := range h {
		for x := range w {
			img.SetColorIndex(x, y, uint8((x/8+y/8)%2)) //nolint:gosec // 0 or 1
		}
	}
	return &gif.GIF{Image: []*image.Paletted{img}, Delay: []int{10}}
}

// commands is the two-byte prefix, and the slot byte where there is one, of
// every command written.
func commands(f *fake) []string {
	var out []string
	for _, r := range f.told(3) {
		switch {
		case r[0] == 0x30 && r[1] == 0x04, r[0] == 0x32, r[0] == 0x36 && r[1] == 0x01, r[0] == 0x38:
			out = append(out, hexs(r[:3]))
		default:
			out = append(out, hexs(r[:2]))
		}
	}
	return out
}

func hexs(b []byte) string { return fmt.Sprintf("% x", b) }

/*
The send sequence, as hotaru's spec 012 records it, and the double buffer.

`36 03` comes first, before the slots are asked about; a setup out of sequence
is refused with `04`. The image goes to a vacant slot, is reserved, begun,
written over endpoint 0x02 as a 20-byte preamble and a payload padded to whole
1024-byte packets, ended, and shown. The second image goes to another slot, and
only once it is shown is the first deleted: the slot on screen is never
written or cleared while it is on screen.
*/
func TestImageSendsInTheDevicesOrderAndNeverTouchesTheSlotOnScreen(t *testing.T) {
	f := newFake()
	r := &recorder{}
	p := withPanel(f, r)

	require.NoError(t, p.Image(context.Background(), card(640, 640)))

	want := []string{"36 03"}
	for i := range buckets {
		want = append(want, hexs([]byte{0x30, 0x04, byte(i)}))
	}
	want = append(want, "32 02 00", "32 01 00", "36 01 00", "36 02", "38 01 04")
	assert.Equal(t, want, commands(f))
	reserve := f.Told[len(f.Told)-4]
	assert.Equal(t, []byte{0x32, 0x01, 0x00, 0x01, 0x00, 0x00}, reserve[:6], "slot 0, slot+1, at the address place chose")
	assert.Equal(t, byte(0x01), reserve[8], "the reserve's trailing 01")
	assert.Equal(t, []byte{0x38, 0x01, 0x04, 0x00}, f.Told[len(f.Told)-1][:4], "slot 0 is shown")

	assert.Equal(t, []byte{bulkEndpoint, bulkEndpoint}, r.endpoints)
	require.Len(t, r.first, 20)
	assert.Equal(t, header, r.first[:12])
	assert.Equal(t, byte(0x01), r.first[12], "a GIF")
	assert.Zero(t, (r.lengths[0]+r.lengths[1])%packetLen, "the payload is padded to whole packets")

	f.mu.Lock()
	f.Told = nil
	f.mu.Unlock()
	require.NoError(t, p.Image(context.Background(), card(640, 640)))

	got := commands(f)
	assert.NotContains(t, got[:len(got)-1], "32 02 00", "the slot on screen was cleared before the new image was up")
	assert.Contains(t, got, "32 02 01")
	assert.Contains(t, got, "32 01 01")
	assert.Equal(t, "32 02 00", got[len(got)-1], "the previous slot is deleted last, after the new one is shown")
	assert.Equal(t, 1, p.showing)
}

/*
Close hands the panel back to the firmware's readout, then releases the
interface, then closes the control channel; and a device nobody drew on is
closed without a write.
*/
func TestCloseReturnsThePanelToItsReadout(t *testing.T) {
	f := newFake()
	r := &recorder{}
	p := withPanel(f, r)
	require.NoError(t, p.Image(context.Background(), card(640, 640)))

	require.NoError(t, p.Close())

	last := f.Told[len(f.Told)-1]
	assert.Equal(t, []byte{0x38, 0x01, modeLiquid, 0x00}, last[:4], "the readout was not restored")
	assert.True(t, r.closed, "the interface was not released")
	assert.True(t, f.closed)

	quiet := newFake()
	q := withPanel(quiet, &recorder{})
	require.NoError(t, q.Close())
	assert.Empty(t, quiet.Told, "a panel nobody drew on was written to on Close")
}

// Appearance is written and not asked: the device acknowledges it with
// nothing, and waiting would time out after the full deadline.
func TestAppearanceIsOneCommandAndRangeChecked(t *testing.T) {
	f := newFake()
	p := withPanel(f, &recorder{})

	require.NoError(t, p.Appearance(context.Background(), 60, 270))
	require.Len(t, f.Told, 1)
	assert.Equal(t, []byte{0x30, 0x02, 0x01, 60, 0x00, 0x00, 0x01, 3}, f.Told[0][:8])

	require.Error(t, p.Appearance(context.Background(), 101, 0))
	require.Error(t, p.Appearance(context.Background(), 50, 45))
	assert.Len(t, f.Told, 1)
}

/*
A GIF of the wrong size is scaled to the panel, and one of the right size is
sent at that size. A wrong-size GIF displays blank, so every image is fitted on
its way to the panel. The caller's GIF is left as it was.
*/
func TestFitScalesToThePanel(t *testing.T) {
	panel := Known[0x3012].Screen
	for _, size := range []int{480, 640} {
		g := card(size, size)
		data, err := fit(g, panel)
		require.NoError(t, err)
		decoded, err := gif.DecodeAll(bytes.NewReader(data))
		require.NoError(t, err)
		assert.Equal(t, 640, decoded.Config.Width)
		assert.Equal(t, 640, decoded.Config.Height)
		assert.Equal(t, image.Rect(0, 0, 640, 640), decoded.Image[0].Bounds())
		assert.Equal(t, image.Rect(0, 0, size, size), g.Image[0].Bounds(), "the caller's GIF was modified")
	}
	_, err := fit(&gif.GIF{}, panel)
	require.Error(t, err)
}

// The panel reports its model's size.
func TestSizeIsTheModels(t *testing.T) {
	var p screen.Panel = withPanel(newFake(), &recorder{})
	w, h := p.Size()
	assert.Equal(t, 640, w)
	assert.Equal(t, 640, h)
}
