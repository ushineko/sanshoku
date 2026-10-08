package hidraw

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"

	"github.com/ushineko/sanshoku"
)

/*
fakeChannel stands in for one collection. Its event is a real Windows event,
so Handle waits on it exactly as it waits on a device; the reports are the
test's.
*/
type fakeChannel struct {
	t        *testing.T
	event    windows.Handle
	canRead  bool
	pending  bool
	reports  [][]byte
	writes   [][]byte
	ioctls   [][]byte
	reply    []byte
	readErr  error
	writeErr error
}

func newFake(t *testing.T, canRead bool) *fakeChannel {
	t.Helper()
	ev, err := windows.CreateEvent(nil, 1, 0, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = windows.CloseHandle(ev) })
	return &fakeChannel{t: t, event: ev, canRead: canRead}
}

// push makes a report arrive on the collection.
func (f *fakeChannel) push(report []byte) {
	f.reports = append(f.reports, report)
	if f.pending {
		require.NoError(f.t, windows.SetEvent(f.event))
	}
}

func (f *fakeChannel) readable() bool        { return f.canRead }
func (f *fakeChannel) ready() windows.Handle { return f.event }

func (f *fakeChannel) arm() error {
	if f.pending {
		return nil
	}
	f.pending = true
	if len(f.reports) > 0 || f.readErr != nil {
		return windows.SetEvent(f.event)
	}
	return windows.ResetEvent(f.event)
}

func (f *fakeChannel) take() ([]byte, error) {
	f.pending = false
	_ = windows.ResetEvent(f.event)
	if f.readErr != nil {
		return nil, f.readErr
	}
	r := f.reports[0]
	f.reports = f.reports[1:]
	return r, nil
}

func (f *fakeChannel) write(report []byte, _ time.Duration) error {
	f.writes = append(f.writes, append([]byte(nil), report...))
	return f.writeErr
}

func (f *fakeChannel) ioctl(code uint32, in, out []byte, _ time.Duration) (int, error) {
	f.ioctls = append(f.ioctls, append([]byte(nil), in...))
	if code == ioctlGetFeature {
		return copy(out, f.reply), nil
	}
	return len(in), nil
}

func (f *fakeChannel) close() error { return nil }

// receiverHandle is a Logitech receiver's HID++ interface over three fakes:
// short, long and very long, in that order.
func receiverHandle(t *testing.T) (*Handle, []*fakeChannel) {
	t.Helper()
	var fakes []*fakeChannel
	h := &Handle{name: "receiver", present: func() bool { return true }}
	for _, info := range hidpp() {
		f := newFake(t, true)
		fakes = append(fakes, f)
		h.colls = append(h.colls, &collection{info: info, ch: f})
	}
	// hidpp lists long, very long, short; reorder the fakes to short, long,
	// very long for the tests' reading.
	return h, []*fakeChannel{fakes[2], fakes[0], fakes[1]}
}

/*
A write goes to the collection that declares its report ID, padded to that
collection's length.

A HID++ short request is seven bytes and goes to the short collection; a long
one goes to the long collection. A report ID that no collection declares is
refused rather than sent somewhere it means something else.
*/
func TestWriteGoesToTheCollectionThatDeclaresTheReport(t *testing.T) {
	h, fakes := receiverHandle(t)
	short, long, veryLong := fakes[0], fakes[1], fakes[2]

	require.NoError(t, h.Write([]byte{0x10, 0x01, 0x81, 0x07, 0, 0, 0}))
	require.NoError(t, h.Write([]byte{0x11, 0xFF, 0x83, 0xB5, 0x40}))

	require.Len(t, short.writes, 1)
	assert.Equal(t, []byte{0x10, 0x01, 0x81, 0x07, 0, 0, 0}, short.writes[0])
	require.Len(t, long.writes, 1)
	assert.Len(t, long.writes[0], 20, "the long report was not padded to its collection's length")
	assert.Equal(t, []byte{0x11, 0xFF, 0x83, 0xB5, 0x40}, long.writes[0][:5])
	assert.Empty(t, veryLong.writes)

	err := h.Write([]byte{0x12, 0x01})
	require.Error(t, err)
}

/*
A collection that does not number its reports takes a write framed as hidraw
frames one: a leading zero is the "no ID" byte, and a report that starts with
anything else is all data and is given the zero in front. Too long a report is
refused.
*/
func TestWriteToAnUnnumberedCollection(t *testing.T) {
	f := newFake(t, true)
	h := &Handle{name: "keyboard", colls: []*collection{{
		info: collectionInfo{outLen: 5, inLen: 5, reports: []Report{{Kind: ReportOutput, Len: 5}}},
		ch:   f,
	}}}

	require.NoError(t, h.Write([]byte{0, 0x92, 0x01}))
	require.NoError(t, h.Write([]byte{0x92, 0x01}))
	require.Error(t, h.Write([]byte{1, 2, 3, 4, 5}))

	require.Len(t, f.writes, 2)
	assert.Equal(t, []byte{0, 0x92, 0x01, 0, 0}, f.writes[0])
	assert.Equal(t, []byte{0, 0x92, 0x01, 0, 0}, f.writes[1])
}

/*
A read takes the first report from any collection.

The request goes to the short collection and the answer comes back on the
long one, as a HID++ 2.0 reply on a Lightspeed receiver does; reading only the
collection written to would wait out the deadline. A report that arrived on
another collection meanwhile is not lost.
*/
func TestReadTakesAReportFromAnyCollection(t *testing.T) {
	h, fakes := receiverHandle(t)
	short, long := fakes[0], fakes[1]
	long.push([]byte{0x11, 0x01, 0x08, 0x10, 0x5f})
	short.push([]byte{0x10, 0x01, 0x8f, 0x81, 0x07, 0x09, 0})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	buf := make([]byte, 32)
	var got [][]byte
	for range 2 {
		n, err := h.Read(ctx, buf)
		require.NoError(t, err)
		got = append(got, append([]byte(nil), buf[:n]...))
	}

	assert.ElementsMatch(t, [][]byte{
		{0x11, 0x01, 0x08, 0x10, 0x5f},
		{0x10, 0x01, 0x8f, 0x81, 0x07, 0x09, 0},
	}, got)
}

// Silence ends at the deadline as context.DeadlineExceeded, and Exchange
// reports it as ErrSilent, as on Linux.
func TestReadWithSilenceEndsAtTheDeadline(t *testing.T) {
	h, _ := receiverHandle(t)
	const deadline = 100 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()

	start := time.Now()
	_, err := Exchange(ctx, h, []byte{0x10, 0xFF, 0, 0x10, 0, 0, 0}, func([]byte) bool { return false }, 20)

	require.ErrorIs(t, err, ErrSilent)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), deadline+200*time.Millisecond)
}

// A collection that does not number its input reports has the zero Windows
// puts in front taken off, so a driver reads what hidraw hands over.
func TestReadUnframesAnUnnumberedReport(t *testing.T) {
	f := newFake(t, true)
	h := &Handle{name: "headset", colls: []*collection{{
		info: collectionInfo{inLen: 5, reports: []Report{{Kind: ReportInput, Len: 5}}},
		ch:   f,
	}}}
	f.push([]byte{0, 0xB0, 0x02, 0x64, 0})

	buf := make([]byte, 8)
	n, err := h.Read(context.Background(), buf)

	require.NoError(t, err)
	assert.Equal(t, []byte{0xB0, 0x02, 0x64, 0}, buf[:n])
}

/*
A feature report goes to the collection that has one long enough, padded to
its length, even on a collection opened with no access, which cannot be read:
the Razer mouse's interface 0 is that collection, and the battery is read
through it.
*/
func TestFeatureReportsGoToTheCollectionWithThem(t *testing.T) {
	keys := newFake(t, true)
	mouse := newFake(t, false)
	mouse.reply = append([]byte{0, 0x02, 0x1F}, make([]byte, 88)...)
	h := &Handle{name: "razer", colls: []*collection{
		{info: collectionInfo{inLen: 16, reports: []Report{{Kind: ReportInput, ID: 1, Len: 16}}}, ch: keys},
		{info: collectionInfo{inLen: 9, featLen: 91, reports: []Report{{Kind: ReportFeature, ID: 0, Len: 91}}}, ch: mouse},
	}}
	req := make([]byte, 91)
	req[2] = 0x1F

	require.NoError(t, h.SetFeature(context.Background(), req))
	got := make([]byte, 91)
	require.NoError(t, h.GetFeature(context.Background(), got))

	require.Len(t, mouse.ioctls, 2)
	assert.Len(t, mouse.ioctls[0], 91)
	assert.Empty(t, keys.ioctls)
	assert.Equal(t, byte(0x02), got[1])
	assert.Equal(t, byte(0x1F), got[2])
}

// An unplugged interface is sanshoku.ErrGone: directly where Windows says
// the device is gone, and for a general failure only once the collection is
// no longer listed.
func TestAnUnpluggedInterfaceIsGone(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		present bool
		gone    bool
	}{
		{"not connected", windows.ERROR_DEVICE_NOT_CONNECTED, true, true},
		{"general failure, still listed", windows.ERROR_GEN_FAILURE, true, false},
		{"general failure, no longer listed", windows.ERROR_GEN_FAILURE, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFake(t, true)
			f.readErr = c.err
			present := c.present
			h := &Handle{name: "gone", present: func() bool { return present }, colls: []*collection{{
				info: collectionInfo{inLen: 7, reports: []Report{{Kind: ReportInput, ID: 0x10, Len: 7}}}, ch: f,
			}}}

			_, err := h.Read(context.Background(), make([]byte, 7))

			require.Error(t, err)
			assert.Equal(t, c.gone, errors.Is(err, sanshoku.ErrGone))
		})
	}
}
