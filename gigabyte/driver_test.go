package gigabyte

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/lighting"
)

// openReports are the commands open sends after the question, as OpenRGB
// sends them at start: the effect registers cleared, committed, beat off.
var openReports = []string{
	"60 00 00", "20 00 00", "21 00 00", "22 00 00", "23 00 00",
	"24 00 00", "25 00 00", "26 00 00", "27 00 00", "28 ff 00", "31 00 00",
}

// Controller 0 of the Z790 AORUS MASTER X has two headers of no lights yet
// and LED_C, and is named after the board and its chip.
func TestOpenLaysOutControllerZero(t *testing.T) {
	f := newFake(0)
	d, err := opened(f)
	require.NoError(t, err)
	assert.Equal(t, openReports, f.told(3))
	assert.Equal(t, "Z790 AORUS MASTER X (0x5701ABCD)", d.Identity().Name)
	assert.Equal(t, "IT5701-GIGABYTE V7.0.18.1", d.firmware)
	assert.Equal(t, []lighting.Zone{
		{Name: "ARGB_V2_1/ARGB_V2_3", First: 0, Max: 256},
		{Name: "ARGB_V2_2", First: 0, Max: 256},
		{Name: "LED_C", First: 0, Count: 1, Shape: lighting.ShapeSingle},
	}, d.Zones())
	assert.Equal(t, []lighting.Key{{ID: 0, Name: "LED_C"}}, d.Keys())
}

func TestOpenLaysOutControllerOne(t *testing.T) {
	d, err := opened(newFake(1))
	require.NoError(t, err)
	assert.Equal(t, []lighting.Zone{{Name: "I/O Cover", First: 0, Count: 1, Shape: lighting.ShapeSingle}}, d.Zones())
}

// A controller with a lamp-array mode has it turned off first.
func TestALampArrayIsTurnedOff(t *testing.T) {
	f := newFake(0)
	f.Info = infoReport(0, flagLampArray, 0x5701ABCD)
	_, err := opened(f)
	require.NoError(t, err)
	assert.Equal(t, append([]string{"60 00 00", "48 00 00"}, openReports[1:]...), f.told(3))
}

func TestAControllerWithNoAnswerIsNotOpened(t *testing.T) {
	f := newFake(0)
	f.Silent = true
	_, err := opened(f)
	require.ErrorIs(t, err, errNoAnswer)
	assert.True(t, f.closed)
}

// A board not in the table is asked its report and given nothing else.
func TestAnUnmeasuredBoardIsRefused(t *testing.T) {
	f := newFake(0)
	c := candidate(masterXNode, func(string) (node, error) { return f, nil }, "Z790 AORUS ELITE AX")
	_, err := c.Open(context.Background())
	require.ErrorIs(t, err, sanshoku.ErrUnsupported)
	assert.Equal(t, []string{"60"}, f.told(1))
	assert.True(t, f.closed)
}

// A controller number the board's layout does not have is refused alike.
func TestAnUnknownControllerNumberIsRefused(t *testing.T) {
	f := newFake(2)
	_, err := opened(f)
	require.ErrorIs(t, err, sanshoku.ErrUnsupported)
	assert.Equal(t, []string{"60"}, f.told(1))
}

func TestAnUnmeasuredProductIsNeverOpened(t *testing.T) {
	n := masterXNode
	n.Product = 0x5711
	c := candidate(n, func(string) (node, error) { t.Fatal("opened"); return nil, nil }, masterX)
	_, err := c.Open(context.Background())
	require.ErrorIs(t, err, sanshoku.ErrUnsupported)
}

// Without the board's name there is no layout, and nothing is opened.
func TestNoBoardNameIsNeverOpened(t *testing.T) {
	c := candidate(masterXNode, func(string) (node, error) { t.Fatal("opened"); return nil, nil }, "")
	_, err := c.Open(context.Background())
	require.ErrorIs(t, err, sanshoku.ErrUnsupported)
}

func TestTheBoardIsNamedFromDMI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board_name")
	require.NoError(t, os.WriteFile(path, []byte(masterX+"\n"), 0o600))
	was := dmiBoard
	dmiBoard = path
	t.Cleanup(func() { dmiBoard = was })
	assert.Equal(t, masterX, boardName())
}

func TestResizeAndItsRefusals(t *testing.T) {
	d, err := opened(newFake(0))
	require.NoError(t, err)
	require.NoError(t, d.Resize("ARGB_V2_2", 30))
	assert.Equal(t, 30, d.Zones()[1].Count)
	require.ErrorIs(t, d.Resize("LED_C", 2), lighting.ErrNoZone)
	require.ErrorIs(t, d.Resize("ARGB_V2_9", 2), lighting.ErrNoZone)
	require.Error(t, d.Resize("ARGB_V2_2", 257))
	require.Error(t, d.Resize("ARGB_V2_2", -1))
	require.NoError(t, d.Resize("ARGB_V2_1/ARGB_V2_3", 200))
	require.Error(t, d.Resize("ARGB_V2_2", 60), "more lights in all than a key can name")
	assert.Equal(t, 30, d.Zones()[1].Count, "a refused resize changed the zone")
}

func every(d *device, c colour) []lighting.Pixel {
	var px []lighting.Pixel
	for _, k := range d.Keys() {
		px = append(px, lighting.Pixel{ID: k.ID, R: c.r, G: c.g, B: c.b})
	}
	return px
}

/*
A frame: the length classes and the headers' own effect off first, then each
header 19 lights a packet with byte offsets and counts, in GRB order, then
LED_C's static effect in blue, green, red, and one commit with its bit.
*/
func TestAFrameIsHeadersThenLightsThenACommit(t *testing.T) {
	f := newFake(0)
	d, err := opened(f)
	require.NoError(t, err)
	require.NoError(t, d.Resize("ARGB_V2_1/ARGB_V2_3", 20))
	require.NoError(t, d.Resize("ARGB_V2_2", 3))
	f.reset()
	require.NoError(t, d.Frame(context.Background(), every(d, colour{0x11, 0x22, 0x33})))
	sent := f.sent()
	require.Len(t, sent, 7)
	assert.Equal(t, []byte{cmdLengths, 0x00, 0x00, 0x00}, sent[0][:4])
	assert.Equal(t, []byte{cmdHeaderEffect, 0x03}, sent[1][:2])

	assert.Equal(t, []byte{0x58, 0, 0, 57}, sent[2][:4])
	assert.Equal(t, []byte{0x22, 0x11, 0x33}, sent[2][4:7], "GRB")
	assert.Equal(t, []byte{0x22, 0x11, 0x33}, sent[2][58:61], "the 19th light")
	assert.Equal(t, []byte{0x58, 57, 0, 3}, sent[3][:4])
	assert.Equal(t, []byte{0, 0, 0}, sent[3][7:10], "nothing past the last light")
	assert.Equal(t, []byte{0x59, 0, 0, 9}, sent[4][:4])

	assert.Equal(t, []byte{0x24, 0x10, 0, 0, 0}, sent[5][:5])
	assert.Equal(t, byte(effectStatic), sent[5][10])
	assert.Equal(t, byte(0xFF), sent[5][11])
	assert.Equal(t, []byte{0x33, 0x22, 0x11}, sent[5][13:16], "blue, green, red")
	assert.Equal(t, []byte{cmdCommit, 0x10, 0, 0, 0}, sent[6][:5])
}

// Only what changed is sent; the headers are prepared once.
func TestOnlyChangedZonesAreSent(t *testing.T) {
	f := newFake(0)
	d, err := opened(f)
	require.NoError(t, err)
	require.NoError(t, d.Resize("ARGB_V2_2", 3))
	require.NoError(t, d.Frame(context.Background(), every(d, colour{1, 2, 3})))
	f.reset()
	keys := d.Keys()
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: keys[len(keys)-1].ID, R: 9}}))
	assert.Equal(t, []string{"24", "28"}, f.told(1), "only LED_C and its commit")
	f.reset()
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 9}}))
	assert.Equal(t, []string{"59"}, f.told(1), "a header alone needs no commit and no preparing again")
	f.reset()
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 9}}))
	assert.Empty(t, f.told(1), "an unchanged frame sends nothing")
}

// A header growing past 32 lights changes its length class, which is sent
// again; the headers' own effect stays off.
func TestALengthClassChangeIsSent(t *testing.T) {
	f := newFake(0)
	d, err := opened(f)
	require.NoError(t, err)
	require.NoError(t, d.Resize("ARGB_V2_2", 3))
	require.NoError(t, d.Frame(context.Background(), every(d, colour{1, 2, 3})))
	require.NoError(t, d.Resize("ARGB_V2_2", 40))
	f.reset()
	require.NoError(t, d.Frame(context.Background(), every(d, colour{1, 2, 3})))
	sent := f.sent()
	assert.Equal(t, []byte{cmdLengths, 0x10, 0, 0}, sent[0][:4])
	assert.NotEqual(t, byte(cmdHeaderEffect), sent[1][0])
}

func TestAKeyNotOnTheControllerSendsNothing(t *testing.T) {
	f := newFake(0)
	d, err := opened(f)
	require.NoError(t, err)
	f.reset()
	require.Error(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 1, R: 1}}))
	assert.Empty(t, f.sent())
}

// Release forgets the frame and sends nothing; the next frame is sent whole.
func TestReleaseForgetsAndSendsNothing(t *testing.T) {
	f := newFake(0)
	d, err := opened(f)
	require.NoError(t, err)
	require.NoError(t, d.Frame(context.Background(), every(d, colour{1, 2, 3})))
	f.reset()
	require.NoError(t, d.Release(context.Background()))
	assert.Empty(t, f.sent())
	require.NoError(t, d.Frame(context.Background(), every(d, colour{1, 2, 3})))
	assert.Equal(t, []string{"24", "28"}, f.told(1))
	require.NoError(t, d.Close())
	require.ErrorIs(t, d.Frame(context.Background(), every(d, colour{1, 2, 3})), os.ErrClosed)
}

// Nothing the driver does sends a command that changes what the board keeps
// (spec 039 R1.5), and write refuses one outright.
func TestNothingTheBoardKeepsIsEverSent(t *testing.T) {
	f := newFake(0)
	d, err := opened(f)
	require.NoError(t, err)
	require.NoError(t, d.Resize("ARGB_V2_1/ARGB_V2_3", 100))
	require.NoError(t, d.Resize("ARGB_V2_2", 40))
	ctx := context.Background()
	require.NoError(t, d.Frame(ctx, every(d, colour{1, 2, 3})))
	require.NoError(t, d.Frame(ctx, every(d, colour{4, 5, 6})))
	require.NoError(t, d.Release(ctx))
	require.NoError(t, d.Frame(ctx, every(d, colour{7, 8, 9})))
	require.NoError(t, d.Close())
	for _, r := range f.sent() {
		assert.False(t, kept(r[0]), "sent % x", r[:4])
	}

	d2, err := opened(newFake(0))
	require.NoError(t, err)
	for _, cmd := range []byte{0x33, 0x38, 0x3B, 0x3D, 0x47, 0x5E} {
		require.Error(t, d2.write(ctx, cmd, 0x01), "%02x", cmd)
	}
}

func TestColourOrders(t *testing.T) {
	assert.Equal(t, order{r: 1, g: 0, b: 2}, orderFrom(0x00010002), "GRB")
	assert.Equal(t, order{r: 0, g: 1, b: 2}, orderFrom(0x00000102), "RGB")
	assert.Equal(t, order{r: 2, g: 1, b: 0}, orderFrom(0x00020100), "BGR")
	assert.Equal(t, order{r: 1, g: 0, b: 2}, orderFrom(0), "none reported: GRB")
	assert.Equal(t, order{r: 1, g: 0, b: 2}, orderFrom(0x01010002), "a stray top byte: GRB")
}
