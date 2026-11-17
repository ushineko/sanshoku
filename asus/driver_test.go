package asus

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

// Open reads the version and the table and sends the packet OpenRGB sends
// next; the Hero has its own light and four headers of no lights yet.
func TestOpenLaysOutTheBoardFromItsTable(t *testing.T) {
	f := newFake()
	d, err := opened(f)
	require.NoError(t, err)
	assert.Equal(t, []string{"82 00", "b0 00", "52 53"}, f.told(2))
	assert.Equal(t, "AULA3-6K75-0309", d.version)
	assert.Equal(t, "ASUS ROG MAXIMUS Z790 HERO", d.Identity().Name)
	assert.Equal(t, []lighting.Key{{ID: 0, Name: "Aura Mainboard, LED 1"}}, d.Keys())
	assert.Equal(t, []lighting.Zone{
		{Name: "Aura Mainboard", First: 0, Count: 1, Shape: lighting.ShapeSingle},
		{Name: "Addressable RGB Header 1", First: 1, Max: 500},
		{Name: "Addressable RGB Header 2", First: 1, Max: 500},
		{Name: "Addressable RGB Header 3", First: 1, Max: 500},
		{Name: "Addressable RGB Header 4", First: 1, Max: 500},
	}, d.Zones())
}

func TestABoardWithNoAnswerIsNotOpened(t *testing.T) {
	f := newFake()
	f.Silent = true
	_, err := opened(f)
	require.ErrorIs(t, err, errNoAnswer)
	assert.True(t, f.closed)
}

func TestAProductOffTheListIsNeverOpened(t *testing.T) {
	n := heroNode
	n.Product = 0x1939
	c := candidate(n, func(string) (node, error) { t.Fatal("opened"); return nil, nil }, "")
	_, err := c.Open(context.Background())
	require.ErrorIs(t, err, sanshoku.ErrUnsupported)
}

// The board's name is OpenRGB's: "ASUS" and the DMI board name.
func TestTheBoardIsNamedFromDMI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board_name")
	require.NoError(t, os.WriteFile(path, []byte("ROG MAXIMUS Z790 HERO\n"), 0o600))
	was := dmiBoard
	dmiBoard = path
	t.Cleanup(func() { dmiBoard = was })
	assert.Equal(t, "ASUS ROG MAXIMUS Z790 HERO", boardName())

	dmiBoard = filepath.Join(t.TempDir(), "none")
	assert.Empty(t, boardName())
}

// A board that says it has more RGB headers than lights is taken to have
// none, as OpenRGB takes it; one that has them names them apart.
func TestRGBHeadersAreNamedAsOpenRGBNamesThem(t *testing.T) {
	f := newFake()
	f.Table[tableBoardLEDs], f.Table[tableRGBHeaders] = 4, 3
	d, err := opened(f)
	require.NoError(t, err)
	keys := d.Keys()
	require.Len(t, keys, 4)
	assert.Equal(t, "Aura Mainboard, LED 1", keys[0].Name)
	assert.Equal(t, "Aura Mainboard, RGB Header 1", keys[1].Name)
	assert.Equal(t, "Aura Mainboard, RGB Header 3", keys[3].Name)
}

func TestResizeGivesAHeaderItsLights(t *testing.T) {
	d, err := opened(newFake())
	require.NoError(t, err)
	require.NoError(t, d.Resize("Addressable RGB Header 2", 16))
	keys := d.Keys()
	require.Len(t, keys, 17)
	assert.Equal(t, lighting.Key{ID: 1, Name: "Addressable RGB Header 2, LED 1"}, keys[1])
	assert.Equal(t, lighting.Key{ID: 16, Name: "Addressable RGB Header 2, LED 16"}, keys[16])
	zones := d.Zones()
	assert.Equal(t, lighting.Zone{Name: "Addressable RGB Header 2", First: 1, Count: 16, Max: 500}, zones[2])
	assert.Equal(t, 17, zones[3].First)

	require.ErrorIs(t, d.Resize("Aura Mainboard", 2), lighting.ErrNoZone)
	require.ErrorIs(t, d.Resize("Header 9", 2), lighting.ErrNoZone)
	require.Error(t, d.Resize("Addressable RGB Header 1", 501))
	require.Error(t, d.Resize("Addressable RGB Header 1", 250), "more lights in all than a key can number")
	assert.Len(t, d.Keys(), 17, "a refused resize changed something")
}

/*
A first frame puts each lit channel in direct mode once and writes it, the
last packet applying it; an unchanged frame sends nothing; a change sends
only its channel.
*/
func TestAFrameClaimsDirectModeOnceAndSendsWhatChanged(t *testing.T) {
	f := newFake()
	d, err := opened(f)
	require.NoError(t, err)
	require.NoError(t, d.Resize("Addressable RGB Header 2", 16))
	f.reset()

	px := []lighting.Pixel{{ID: 0, R: 0xFF}}
	for id := byte(1); id <= 16; id++ {
		px = append(px, lighting.Pixel{ID: id, G: 0xFF})
	}
	require.NoError(t, d.Frame(context.Background(), px))
	assert.Equal(t, []string{
		"35 00 00 00 ff", // the board's lights, direct mode
		"40 84 00 01 ff", // channel 04, applied, one light, red
		"35 02 00 00 ff", // header 2 is effect channel 2
		"40 81 00 10 00", // direct channel 01, applied, 16 lights
	}, f.told(5))

	f.reset()
	require.NoError(t, d.Frame(context.Background(), px))
	assert.Empty(t, f.Told, "an unchanged frame was sent")

	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 16, B: 0xFF}}))
	assert.Equal(t, []string{"40 81 00 10"}, f.told(4), "only header 2, and no second claim")
	assert.Equal(t, []byte{0x00, 0x00, 0xFF}, f.Told[0][4+15*3:4+16*3])

	require.Error(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 17}}))
}

func TestDirectPacketsSplitAtTwentyAndPast255(t *testing.T) {
	held := make([]colour, 45)
	ps := directPackets(0x02, held)
	require.Len(t, ps, 3)
	assert.Equal(t, []byte{0x40, 0x02, 0, 20}, ps[0][:4])
	assert.Equal(t, []byte{0x40, 0x02, 20, 20}, ps[1][:4])
	assert.Equal(t, []byte{0x40, 0x82, 40, 5}, ps[2][:4])
	assert.Len(t, ps[2], 4+5*3)

	long := directPackets(0x01, make([]colour, 300))
	require.Len(t, long, 15)
	assert.Equal(t, []byte{0x40, 0x01, 240, 20}, long[12][:4])
	assert.Equal(t, []byte{0x40, 0x11, 4, 20}, long[13][:4], "light 260: the flag, and 4 past 256")
	assert.Equal(t, []byte{0x40, 0x91, 24, 20}, long[14][:4])
}

func TestReleaseAndCloseSendNothing(t *testing.T) {
	f := newFake()
	d, err := opened(f)
	require.NoError(t, err)
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 1}}))
	f.reset()
	require.NoError(t, d.Release(context.Background()))
	require.NoError(t, d.Close())
	assert.Empty(t, f.Told)
	assert.True(t, f.closed)
}
