package nzxt

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/hidraw"
	"github.com/ushineko/sanshoku/lighting"
)

// TestMain fails the run if any fake in the package was sent a lighting
// command outside spec 024 R1.11 (R4.4).
func TestMain(m *testing.M) {
	code := m.Run()
	misuse.Lock()
	seen := misuse.seen
	misuse.Unlock()
	if code == 0 && len(seen) > 0 {
		fmt.Fprintln(os.Stderr, "lighting commands outside spec 024 R1.11:")
		for _, s := range seen {
			fmt.Fprintln(os.Stderr, "  "+s)
		}
		code = 1
	}
	os.Exit(code)
}

// lights is the lighting and screen-switch commands written, as their first
// three bytes: `20 03`, `26 xx <ch>` and `38 01 <mode>`.
func lights(f *fake) []string {
	var out []string
	for _, r := range f.told(3) {
		switch r[0] {
		case 0x20, 0x26, 0x38:
			out = append(out, hexs(r))
		}
	}
	return out
}

// sentSince is the reports written after the first n.
func sentSince(f *fake, n int) [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]byte(nil), f.Told[n:]...)
}

func written(f *fake) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Told)
}

// The guard catches what R1.11 forbids, so a clean run means something.
func TestTheGuardCatchesForbiddenLightingCommands(t *testing.T) {
	misuse.Lock()
	before := len(misuse.seen)
	misuse.Unlock()

	f := newFake()
	require.NoError(t, f.Write(lightReport(0x22, 0x10, 0x01)))
	require.NoError(t, f.Write(lightReport(0x26, 0x06, 0x01)))
	require.NoError(t, f.Write(lightReport(0x26, 0x04, 0x01)))
	require.NoError(t, f.Write(lightReport(0x26, 0x06, 0x01)))

	misuse.Lock()
	caught := misuse.seen[before:]
	misuse.seen = misuse.seen[:before]
	misuse.Unlock()
	assert.Len(t, caught, 2, "the 22 write and the bare 26 06, not the 26 06 after a 26 04")
}

/*
The reports byte for byte: 512 bytes, G, R, B order, the channel numbered from
1, and the fixed mode with the channel's LED count, as CAM sends it.
*/
func TestLightingReportsAreTheCoolers(t *testing.T) {
	cs := []colour{{r: 0x11, g: 0x22, b: 0x33}, {r: 0xAA, g: 0xBB, b: 0xCC}}

	f := liveReport(0, cs)
	require.Len(t, f, lightReportLen)
	assert.Equal(t, []byte{0x26, 0x14, 0x01, 0x01, 0x22, 0x11, 0x33, 0xBB, 0xAA, 0xCC, 0x00}, f[:11])
	assert.Equal(t, []byte{0x26, 0x14, 0x02, 0x02}, liveReport(1, cs)[:4], "the fans' colours, shown on the fans")

	b := bufferReport(1, cs)
	require.Len(t, b, lightReportLen)
	assert.Equal(t, []byte{0x26, 0x04, 0x02, 0x00, 0x22, 0x11, 0x33, 0xBB, 0xAA, 0xCC, 0x00}, b[:11])

	a := applyReport(0, 24)
	require.Len(t, a, lightReportLen)
	assert.Equal(t, []byte{0x26, 0x06, 0x01, 0x00, 0x01, 0x00, 0x00, 0x18, 0x00, 0x00, 0x80, 0x00, 0x32, 0x00, 0x00, 0x01, 0x00}, a[:17])
}

// A cooler with a panel is a screen and a canvas: the two interfaces must not
// share a method of one name and two signatures.
func TestTheCoolerIsAScreenAndACanvas(t *testing.T) {
	p := withPanel(newFake(), &recorder{})
	assert.Equal(t, []string{"cooling", "screen", "lighting", "onboard"}, sanshoku.Capabilities(p))
}

// This desk: the ring's 24 LEDs, then the F360's 24. The list is read once per
// handle.
func TestKeysAreTheRingThenTheFans(t *testing.T) {
	f := newFake()
	d := withFake(f)

	keys := d.Keys()
	require.Len(t, keys, 48)
	assert.Equal(t, lighting.Key{ID: 0, Name: "ring 1"}, keys[0])
	assert.Equal(t, lighting.Key{ID: 23, Name: "ring 24"}, keys[23])
	assert.Equal(t, lighting.Key{ID: 24, Name: "fan 1"}, keys[24])
	assert.Equal(t, lighting.Key{ID: 47, Name: "fan 24"}, keys[47])

	assert.Len(t, d.Keys(), 48)
	assert.Equal(t, []string{"20 03 00"}, lights(f), "the accessory list is asked once per handle")
}

func TestAChannelsKeysComeFromItsAccessories(t *testing.T) {
	cases := map[string]struct {
		slots [channels][slotsPer]byte
		keys  int
		last  string
	}{
		"three F120 Core fans":       {[channels][slotsPer]byte{{0x1E}, {0x17, 0x17, 0x17}}, 48, "fan 24"},
		"an empty fan channel":       {[channels][slotsPer]byte{{0x1E}, {}}, 24, "ring 24"},
		"an accessory nobody knows":  {[channels][slotsPer]byte{{0x1E}, {0x17, 0x7F}}, 24, "ring 24"},
		"an unknown one on the ring": {[channels][slotsPer]byte{{0x7F}, {0x1D}}, 24, "fan 24"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFake()
			f.Accessories = c.slots
			keys := withFake(f).Keys()
			require.Len(t, keys, c.keys)
			assert.Equal(t, c.last, keys[len(keys)-1].Name)
			assert.Equal(t, byte(c.keys-1), keys[len(keys)-1].ID, "ids run from 0 with no gaps")
		})
	}
}

/*
hotaru #206: OpenRGB reads ten reports and parses the last. The reply here
arrives behind 25 status reports and is still found, because it is matched by
its prefix.
*/
func TestTheAccessoryListIsFoundBehindOtherReports(t *testing.T) {
	f := newFake()
	f.Chatter = 25
	assert.Len(t, withFake(f).Keys(), 48)
}

func TestNoAccessoryListIsSilentAndAskedAgain(t *testing.T) {
	f := newFake()
	f.Silent = true
	d := withFake(f)

	err := d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 255}})
	require.ErrorIs(t, err, hidraw.ErrSilent)
	assert.Empty(t, d.Keys())

	f.mu.Lock()
	f.Silent = false
	f.mu.Unlock()
	assert.Len(t, d.Keys(), 48)
	assert.Equal(t, []string{"20 03 00", "20 03 00", "20 03 00"}, lights(f), "asked at each call until answered, and no frame sent")
}

func TestAFrameSendsOnlyTheChannelsThatChanged(t *testing.T) {
	f := newFake()
	d := withFake(f)
	ctx := context.Background()

	require.NoError(t, d.Frame(ctx, []lighting.Pixel{{ID: 0, R: 255}}))
	assert.Equal(t, []string{"20 03 00", "26 14 01"}, lights(f), "the fans were never named")
	first := f.Told[len(f.Told)-1]
	assert.Equal(t, []byte{0x00, 0xFF, 0x00, 0x00, 0x00, 0x00}, first[4:10], "LED 1 red in G, R, B; LED 2 dark")

	n := written(f)
	require.NoError(t, d.Frame(ctx, []lighting.Pixel{{ID: 0, R: 255}}))
	assert.Empty(t, sentSince(f, n), "an unchanged frame is not sent")

	require.NoError(t, d.Frame(ctx, []lighting.Pixel{{ID: 24, B: 255}}))
	got := sentSince(f, n)
	require.Len(t, got, 1, "only the fans changed")
	assert.Equal(t, []byte{0x26, 0x14, 0x02, 0x02, 0x00, 0x00, 0xFF}, got[0][:7])

	n = written(f)
	require.NoError(t, d.Frame(ctx, []lighting.Pixel{{ID: 1, G: 255}}))
	got = sentSince(f, n)
	require.Len(t, got, 1)
	assert.Equal(t, []byte{0x00, 0xFF, 0x00, 0xFF, 0x00, 0x00}, got[0][4:10], "LED 1 is still red; LED 2 is green")
}

func TestAKeyThatIsNotALightSendsNothing(t *testing.T) {
	f := newFake()
	d := withFake(f)
	ctx := context.Background()
	require.NotEmpty(t, d.Keys())
	n := written(f)

	require.Error(t, d.Frame(ctx, []lighting.Pixel{{ID: 0, R: 255}, {ID: 48, R: 255}}))
	assert.Empty(t, sentSince(f, n))

	require.NoError(t, d.Frame(ctx, []lighting.Pixel{{ID: 1, G: 255}}))
	got := sentSince(f, n)
	require.Len(t, got, 1)
	assert.Equal(t, []byte{0x00, 0x00, 0x00, 0xFF, 0x00, 0x00}, got[0][4:10], "the refused frame's red was not held")
}

func TestAnUnacknowledgedFrameIsSilentAndSentAgain(t *testing.T) {
	f := newFake()
	d := withFake(f)
	ctx := context.Background()
	require.NotEmpty(t, d.Keys())

	f.mu.Lock()
	f.Silent = true
	f.mu.Unlock()
	require.ErrorIs(t, d.Frame(ctx, []lighting.Pixel{{ID: 0, R: 255}}), hidraw.ErrSilent)

	f.mu.Lock()
	f.Silent = false
	f.mu.Unlock()
	n := written(f)
	require.NoError(t, d.Frame(ctx, nil))
	got := sentSince(f, n)
	require.Len(t, got, 1, "the held colours are sent again")
	assert.Equal(t, []byte{0x26, 0x14, 0x01, 0x01, 0x00, 0xFF, 0x00}, got[0][:7])
}

func TestReleaseCommitsTheChannelsDrawnAndForgetsThem(t *testing.T) {
	f := newFake()
	d := withFake(f)
	ctx := context.Background()

	require.NoError(t, d.Release(ctx))
	assert.Empty(t, lights(f), "nothing drawn, nothing sent")

	require.NoError(t, d.Frame(ctx, []lighting.Pixel{{ID: 0, R: 255}}))
	n := written(f)
	require.NoError(t, d.Release(ctx))
	got := sentSince(f, n)
	require.Len(t, got, 2, "the ring's buffer and its apply; the fans were never drawn")
	assert.Equal(t, []byte{0x26, 0x04, 0x01, 0x00, 0x00, 0xFF, 0x00}, got[0][:7])
	assert.Equal(t, applyReport(0, 24), got[1])

	n = written(f)
	require.NoError(t, d.Frame(ctx, []lighting.Pixel{{ID: 1, G: 255}}))
	got = sentSince(f, n)
	require.Len(t, got, 1)
	assert.Equal(t, []byte{0x00, 0x00, 0x00, 0xFF, 0x00, 0x00}, got[0][4:10], "a frame after a release starts from dark")
}

/*
The readout re-applies each channel's mode, which would replace a live frame
with the buffer (spec 024 E5, E10). The held frame is committed before the
switch, once: a second readout with nothing new drawn commits nothing.
*/
func TestTheReadoutIsPrecededByACommitOfTheHeldFrame(t *testing.T) {
	f := newFake()
	p := withPanel(f, &recorder{})
	ctx := context.Background()

	require.NoError(t, p.Readout(ctx))
	assert.Equal(t, []string{"38 01 02"}, lights(f), "no frame held, no commit")

	require.NoError(t, p.Frame(ctx, []lighting.Pixel{{ID: 0, R: 255}, {ID: 24, B: 255}}))
	n := len(lights(f))
	require.NoError(t, p.Readout(ctx))
	assert.Equal(t, []string{"26 04 01", "26 06 01", "26 04 02", "26 06 02", "38 01 02"}, lights(f)[n:])

	n = len(lights(f))
	require.NoError(t, p.Readout(ctx))
	assert.Equal(t, []string{"38 01 02"}, lights(f)[n:], "already committed")

	require.NoError(t, p.Frame(ctx, []lighting.Pixel{{ID: 0, G: 255}}))
	n = len(lights(f))
	require.NoError(t, p.Close())
	assert.Equal(t, []string{"26 04 01", "26 06 01", "38 01 02"}, lights(f)[n:], "Close's handback commits too")
}

// blocking is a bulk interface whose first transfer waits to be let go.
type blocking struct {
	recorder
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (b *blocking) Bulk(ctx context.Context, endpoint byte, data []byte) error {
	b.once.Do(func() {
		close(b.entered)
		<-b.release
	})
	return b.recorder.Bulk(ctx, endpoint, data)
}

/*
A frame and a screen transfer never interleave on the node (R1.9): a frame
asked for during an upload is sent after the upload's `36 02`, not between
its `36 01` and `36 02`.
*/
func TestAFrameWaitsForAScreenTransfer(t *testing.T) {
	f := newFake()
	b := &blocking{entered: make(chan struct{}), release: make(chan struct{})}
	d := withFake(f)
	d.usbPath = krakenNode.USBPath
	d.claim = func(string) (bulk, error) { return b, nil }
	p := &panelDevice{d}
	ctx := context.Background()
	require.NotEmpty(t, p.Keys())

	upload := make(chan error, 1)
	go func() { upload <- p.Image(ctx, card(640, 640)) }()
	<-b.entered

	frame := make(chan error, 1)
	go func() { frame <- p.Frame(ctx, []lighting.Pixel{{ID: 0, R: 255}}) }()
	// Give the frame time to reach the lock the upload holds.
	time.Sleep(20 * time.Millisecond)
	close(b.release)
	require.NoError(t, <-upload)
	require.NoError(t, <-frame)

	got := commands(f)
	begin, end, drawn := -1, -1, -1
	for i, c := range got {
		switch c {
		case "36 01 00":
			begin = i
		case "36 02":
			end = i
		case "26 14":
			drawn = i
		}
	}
	require.True(t, begin >= 0 && end > begin && drawn >= 0, "%v", got)
	assert.Greater(t, drawn, end, "the frame went out during the upload: %v", got)
}
