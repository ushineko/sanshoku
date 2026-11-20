package razer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/hidraw"
	"github.com/ushineko/sanshoku/lighting"
)

/*
lightFake is a Razer control node in memory: it answers the pairing question
with paired, echoes every other report back with status success, as a device
does once it has taken it, and records every report sent. A device not a
receiver (paired nil) refuses the pairing question.
*/
type lightFake struct {
	mu     sync.Mutex
	paired []uint16
	sent   [][]byte
	last   []byte
	asked  int
	closed bool
}

func (f *lightFake) SetFeature(_ context.Context, r []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(r) != reportSize+1 {
		return fmt.Errorf("a report of %d bytes", len(r))
	}
	f.sent = append(f.sent, append([]byte(nil), r...))
	reply := append([]byte(nil), r...)
	body := reply[1:]
	body[0] = statusOK
	if body[6] == classGeneral && body[7] == commandPaired {
		f.asked++
		if f.paired == nil {
			body[0] = statusNotSupported
		} else {
			body[argsAt] = byte(len(f.paired))
			for i, pid := range f.paired {
				body[argsAt+i*3+1] = 1
				body[argsAt+i*3+2], body[argsAt+i*3+3] = byte(pid>>8), byte(pid)
			}
		}
	}
	f.last = reply
	return nil
}

func (f *lightFake) GetFeature(_ context.Context, r []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.last == nil {
		return errors.New("nothing to read")
	}
	copy(r, f.last)
	return nil
}

func (f *lightFake) Close() error { f.closed = true; return nil }

// reports is every report sent after the first n, as class/command and size.
func (f *lightFake) reports(n int) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, r := range f.sent[n:] {
		b := r[1:]
		out = append(out, fmt.Sprintf("%02x %02x/%02x %02x", b[1], b[6], b[7], b[5]))
	}
	return out
}

var dockNode = hidraw.Node{Vendor: razerVendor, Product: productMouseDockPro, Name: "Razer Razer Mouse Dock Pro", Path: "/dev/hidraw-test7"}

// withNodes points the lighting driver at nodes, each opening to its fake.
func withNodes(t *testing.T, fakes map[string]*lightFake, nodes ...hidraw.Node) {
	t.Helper()
	wasNodes, wasOpen := lightNodes, openLights
	lightNodes = func() ([]hidraw.Node, error) { return nodes, nil }
	openLights = func(path string) (lightNode, error) { return fakes[path], nil }
	pairedSeen.Lock()
	pairedSeen.found = map[string][]uint16{}
	pairedSeen.Unlock()
	t.Cleanup(func() { lightNodes, openLights = wasNodes, wasOpen })
}

func openFound(t *testing.T, c sanshoku.Candidate) *lightDevice {
	t.Helper()
	dev, err := c.Open(context.Background())
	require.NoError(t, err)
	return dev.(*lightDevice)
}

// The dock is a candidate, and so is the mouse it lists, named after both;
// the dock is asked once per process.
func TestFindListsTheDockAndTheMouseOnIt(t *testing.T) {
	f := &lightFake{paired: []uint16{0x00AB, 0xFFFF}}
	withNodes(t, map[string]*lightFake{dockNode.Path: f}, dockNode)
	found, err := Lights{}.Find(context.Background())
	require.NoError(t, err)
	require.Len(t, found, 2)
	assert.Equal(t, "Razer Mouse Dock Pro", found[0].Name)
	assert.Equal(t, "Razer Basilisk V3 Pro (Wireless) (Razer Mouse Dock Pro)", found[1].Name)
	assert.Equal(t, uint16(0x00AB), found[1].Product)
	assert.Equal(t, []string{"ff 00/bf 50"}, f.reports(0), "the pairing question on the dock's own transaction")

	_, err = Lights{}.Find(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, f.asked, "the dock was asked again")
}

// A product not in lit is not a candidate; a device that is not a receiver
// lists only itself.
func TestFindTakesOnlyLitProducts(t *testing.T) {
	mouse := hidraw.Node{Vendor: razerVendor, Product: 0x00AA, Path: "/dev/hidraw-test8"}
	other := hidraw.Node{Vendor: razerVendor, Product: 0x0F1F, Path: "/dev/hidraw-test9"}
	withNodes(t, map[string]*lightFake{mouse.Path: {}, other.Path: {}}, mouse, other)
	found, err := Lights{}.Find(context.Background())
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, "Razer Basilisk V3 Pro (Wired)", found[0].Name)
}

func TestTheMouseKeysAndZonesAreOpenRGBs(t *testing.T) {
	f := &lightFake{paired: []uint16{0x00AB}}
	withNodes(t, map[string]*lightFake{dockNode.Path: f}, dockNode)
	found, err := Lights{}.Find(context.Background())
	require.NoError(t, err)
	d := openFound(t, found[1])
	keys := d.Keys()
	require.Len(t, keys, 13)
	assert.Equal(t, "Scroll Wheel", keys[0].Name)
	assert.Equal(t, "Logo", keys[1].Name)
	assert.Equal(t, "LED Strip LED 1", keys[2].Name)
	assert.Equal(t, "LED Strip LED 11", keys[12].Name)
	assert.Equal(t, []lighting.Zone{
		{Name: "Scroll Wheel", First: 0, Count: 1, Shape: lighting.ShapeSingle},
		{Name: "Logo", First: 1, Count: 1, Shape: lighting.ShapeSingle},
		{Name: "LED Strip", First: 2, Count: 11, Shape: lighting.ShapeLine},
	}, d.Zones())
}

/*
The dock's frame is one extended custom-frame row and the custom mode, on
its transaction; the mouse's goes on the mouse's transaction and waits for
the relay's answer. An unchanged frame sends nothing.
*/
func TestAFrameIsRowsThenTheCustomMode(t *testing.T) {
	f := &lightFake{paired: []uint16{0x00AB}}
	withNodes(t, map[string]*lightFake{dockNode.Path: f}, dockNode)
	found, err := Lights{}.Find(context.Background())
	require.NoError(t, err)
	dock, mouse := openFound(t, found[0]), openFound(t, found[1])
	ctx := context.Background()

	n := len(f.sent)
	require.NoError(t, dock.Frame(ctx, []lighting.Pixel{{ID: 0, R: 0x11, G: 0x22, B: 0x33}}))
	assert.Equal(t, []string{"ff 0f/03 20", "ff 0f/02 0c"}, f.reports(n))
	row := f.sent[n][1:]
	assert.Equal(t, []byte{0, 0, 0, 0, 8, 0x11, 0x22, 0x33}, row[argsAt:argsAt+8])
	assert.Equal(t, crc(row), row[crcAt])
	mode := f.sent[n+1][1:]
	assert.Equal(t, []byte{0, 0, effectCustomFrame}, mode[argsAt:argsAt+3])

	n = len(f.sent)
	require.NoError(t, dock.Frame(ctx, []lighting.Pixel{{ID: 0, R: 0x11, G: 0x22, B: 0x33}}))
	assert.Len(t, f.sent, n, "an unchanged frame sent")

	require.NoError(t, mouse.Frame(ctx, []lighting.Pixel{{ID: 2, G: 0xFF}}))
	assert.Equal(t, []string{"1f 0f/03 2c", "1f 0f/02 0c"}, f.reports(n))
}

// A standard-matrix device takes 03:0B rows and 03:0A, storing nothing.
func TestAStandardFrame(t *testing.T) {
	var pid uint16
	for p, d := range lit {
		if d.matrix == matrixStandard && d.rows == 1 {
			pid = p
			break
		}
	}
	require.NotZero(t, pid)
	node := hidraw.Node{Vendor: razerVendor, Product: pid, Path: "/dev/hidraw-test10"}
	f := &lightFake{}
	withNodes(t, map[string]*lightFake{node.Path: f}, node)
	found, err := Lights{}.Find(context.Background())
	require.NoError(t, err)
	d := openFound(t, found[0])
	n := len(f.sent)
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 0, R: 1}}))
	sent := f.sent[n:]
	require.Len(t, sent, 2)
	assert.Equal(t, []byte{classStandard, commandFrameStd}, sent[0][7:9])
	assert.Equal(t, []byte{0xFF, 0, 0, byte(lit[pid].cols - 1), 1}, sent[0][1+argsAt:1+argsAt+5])
	assert.Equal(t, []byte{classStandard, commandModeStd, modeCustom, storageNone}, sent[1][7:11])
}

// A linear frame is declared 0x32 bytes, as OpenRGB declares it, even when
// its row is longer.
func TestALinearFrameIsDeclared0x32(t *testing.T) {
	node := hidraw.Node{Vendor: razerVendor, Product: 0x004C, Path: "/dev/hidraw-test11"} // Diamondback Chroma, 21 lights
	f := &lightFake{}
	withNodes(t, map[string]*lightFake{node.Path: f}, node)
	found, err := Lights{}.Find(context.Background())
	require.NoError(t, err)
	d := openFound(t, found[0])
	n := len(f.sent)
	require.NoError(t, d.Frame(context.Background(), []lighting.Pixel{{ID: 20, B: 9}}))
	assert.Equal(t, []string{"3f 03/0c 32", "3f 03/0a 02"}, f.reports(n))
	assert.Equal(t, byte(9), f.sent[n][1+argsAt+2+20*3+2], "the 21st light's blue")
}

func TestOnlyTheAllowedReportsAreBuilt(t *testing.T) {
	_, err := report(0xFF, 0x0F, 0x04, nil) // brightness, which stores
	require.ErrorIs(t, err, errNotAllowed)
	_, err = report(0xFF, 0x00, 0x04, []byte{3, 0}) // device mode
	require.ErrorIs(t, err, errNotAllowed)
}

func TestReleaseAndClose(t *testing.T) {
	f := &lightFake{}
	withNodes(t, map[string]*lightFake{dockNode.Path: f}, dockNode)
	found, err := Lights{}.Find(context.Background())
	require.NoError(t, err)
	d := openFound(t, found[0])
	ctx := context.Background()
	require.NoError(t, d.Frame(ctx, []lighting.Pixel{{ID: 0, R: 1}}))
	n := len(f.sent)
	require.NoError(t, d.Release(ctx))
	assert.Len(t, f.sent, n)
	require.Error(t, d.Frame(ctx, []lighting.Pixel{{ID: 9, R: 1}}))
	require.NoError(t, d.Close())
	require.ErrorIs(t, d.Frame(ctx, []lighting.Pixel{{ID: 0, R: 2}}), os.ErrClosed)
}

// Every device in lit is one the driver can draw.
func TestEveryLitDeviceFitsTheReports(t *testing.T) {
	require.Contains(t, lit, uint16(productMouseDockPro))
	require.Contains(t, lit, uint16(0x00AB))
	for pid, d := range lit {
		lights := 0
		for _, z := range d.zones {
			lights += z.rows * z.cols
			assert.NotEmpty(t, z.name, "%04x", pid)
		}
		assert.Equal(t, d.rows*d.cols, lights, "%s: zones do not fill the matrix", d.name)
		assert.LessOrEqual(t, lights, 256, d.name)
		assert.LessOrEqual(t, d.cols*3+5, argsMax, d.name)
		assert.NotEmpty(t, d.name)
	}
}
