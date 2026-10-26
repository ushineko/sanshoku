package aula

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/hidraw"
	"github.com/ushineko/sanshoku/lighting"
)

/*
board is the keyboard behind the receiver, holding its two blocks. It answers
the model question, streams a block when asked for one, and acknowledges each
written packet, storing a block once its last packet has arrived. It can be
asleep, withhold one acknowledgement, flag one as a checksum error, or take a
write and keep the old block (lies).
*/
type board struct {
	model    byte
	cfg, pal []byte
	asleep   bool
	withhold int // the packet index whose acknowledgement never comes; -1 for none
	badSum   int // the packet index acknowledged as a checksum error; -1 for none
	lies     bool

	pending  [][]byte
	incoming []byte
	requests [][]byte
}

func newBoard() *board {
	return &board{model: modelF75, cfg: config(12, 0), pal: palette(), withhold: -1, badSum: -1}
}

func (b *board) Write(req []byte) error {
	noteBoard(req[1])
	b.requests = append(b.requests, bytes.Clone(req))
	if b.asleep {
		return nil
	}
	switch command, index := req[1], int(req[3]); command {
	case commandModel:
		b.pending = append(b.pending, modelReply(b.model))
	case commandReadConfig:
		b.stream(command, b.cfg)
	case commandReadPalette:
		b.stream(command, b.pal)
	case commandWriteConfig, commandWritePalette:
		b.incoming = append(b.incoming, req[5:5+int(req[4]&0x0F)]...)
		if index+1 == int(req[2]) {
			if !b.lies {
				if command == commandWriteConfig {
					b.cfg = b.incoming
				} else {
					b.pal = b.incoming
				}
			}
			b.incoming = nil
		}
		switch index {
		case b.withhold:
		case b.badSum:
			b.pending = append(b.pending, packet(command|ackError, req[2], req[3], operationWrite, nil))
		default:
			b.pending = append(b.pending, packet(command, req[2], req[3], operationWrite, nil))
		}
	}
	return nil
}

// stream queues a block as the keyboard sends it, with the receiver's own
// status frame among the packets.
func (b *board) stream(command byte, block []byte) {
	total := (len(block) + bodyLen - 1) / bodyLen
	for i := range total {
		if i == 1 {
			b.pending = append(b.pending, status)
		}
		chunk := block[i*bodyLen : min((i+1)*bodyLen, len(block))]
		b.pending = append(b.pending, packet(command, byte(total), byte(i), operationRead, chunk))
	}
}

func (b *board) Read(ctx context.Context, buf []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if len(b.pending) == 0 {
		return 0, fmt.Errorf("no report from the fake: %w", context.DeadlineExceeded)
	}
	r := b.pending[0]
	b.pending = b.pending[1:]
	return copy(buf, r), nil
}

func (b *board) Drain(int)    { b.pending = nil }
func (b *board) Close() error { return nil }

// written is every block command the board was written, in order, one entry
// per packet.
func (b *board) written() []byte {
	var out []byte
	for _, r := range b.requests {
		if r[1] == commandWriteConfig || r[1] == commandWritePalette {
			out = append(out, r[1])
		}
	}
	return out
}

// config is a config block running effect n, with every effect's colour on
// slot.
func config(n, slot byte) []byte {
	c := make([]byte, configLen)
	c[configEffect] = n
	for e := byte(firstColoured); e <= lastColoured; e++ {
		c[configTable+2*int(e)] = 4
		c[slotAt(e)] = 2<<4 | slot
	}
	copy(c[configLen-2:], configEnd)
	return c
}

// factory are the colours every palette row starts with.
var factory = []lighting.Colour{
	{R: 0xff}, {G: 0xff}, {B: 0xff}, {R: 0xff, G: 0xff}, {R: 0xff, B: 0xff}, {G: 0xff, B: 0xff}, {R: 0xff, G: 0xff, B: 0xff},
}

// palette is a palette with the factory colours in every row.
func palette() []byte {
	p := make([]byte, paletteLen)
	for row := range paletteLen / rowLen {
		for s, c := range factory {
			copy(p[row*rowLen+s*3:], []byte{c.R, c.G, c.B})
		}
	}
	return p
}

var purple = lighting.Colour{R: 0x80, B: 0xff}

func openBoard(t *testing.T, b *board) sanshoku.Device {
	t.Helper()
	c := candidate(receiver, 20*time.Millisecond, func(string) (node, error) { return b, nil })
	dev, err := c.Open(context.Background())
	require.NoError(t, err)
	return dev
}

func onboardOf(t *testing.T, dev sanshoku.Device) lighting.Onboard {
	t.Helper()
	ob, ok := dev.(lighting.Onboard)
	require.True(t, ok)
	return ob
}

// differences is the offsets at which two blocks differ.
func differences(a, b []byte) []int {
	var out []int
	for i := range a {
		if a[i] != b[i] {
			out = append(out, i)
		}
	}
	return out
}

// A write packet is the one l65ctl.py sends: 13, the command, the count, the
// index, operation 1 and the length, the body, and the sum.
func TestAWritePacketIsTheOneL65ctlSends(t *testing.T) {
	body := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14}
	want := []byte{0x13, 0x09, 35, 2, 0x1e, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 0}
	want[19] = checksum(want)

	assert.Equal(t, want, packet(commandWritePalette, 35, 2, operationWrite, body))
	assert.Equal(t, []byte{0x13, 0x44, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x58},
		packet(commandReadConfig, 1, 0, operationRead, nil), "the config read")
}

// The two blocks name the running effect and its colour.
func TestTheBlocksDescribeTheRunningEffect(t *testing.T) {
	assert.Equal(t, lighting.Effect{Name: "Reaction", Coloured: true, Colour: &lighting.Colour{B: 0xff}},
		describe(config(12, 2), palette()))
	assert.Equal(t, lighting.Effect{Name: "Reaction", Coloured: true},
		describe(config(12, slotColourful), palette()), "colourful has no one colour")
	assert.Equal(t, lighting.Effect{Name: "Off"}, describe(config(0, 0), palette()))

	self := config(12, 0)
	self[configSelfDefine] = 1
	assert.Equal(t, "Self-define", describe(self, palette()).Name, "the flag wins over the effect byte")
}

// Each case of spec 020 R2.3, and nothing outside the change moves.
func TestRecolourWritesTheLeast(t *testing.T) {
	n := byte(12)
	row := int(n) * rowLen

	t.Run("already shown", func(t *testing.T) {
		_, _, cfgChanged, palChanged, err := recolour(config(n, 2), palette(), lighting.Colour{B: 0xff})
		require.NoError(t, err)
		assert.False(t, cfgChanged)
		assert.False(t, palChanged)
	})
	t.Run("another slot holds it", func(t *testing.T) {
		cfg := config(n, 2)
		newCfg, newPal, cfgChanged, palChanged, err := recolour(cfg, palette(), lighting.Colour{G: 0xff, B: 0xff})
		require.NoError(t, err)
		assert.True(t, cfgChanged)
		assert.False(t, palChanged)
		assert.Equal(t, []int{slotAt(n)}, differences(cfg, newCfg))
		assert.Equal(t, byte(2<<4|5), newCfg[slotAt(n)], "the speed is kept")
		assert.Equal(t, palette(), newPal)
	})
	t.Run("a slot from 0 to 6", func(t *testing.T) {
		newCfg, newPal, cfgChanged, palChanged, err := recolour(config(n, 2), palette(), purple)
		require.NoError(t, err)
		assert.False(t, cfgChanged)
		assert.True(t, palChanged)
		assert.Equal(t, config(n, 2), newCfg)
		assert.Equal(t, []int{row + 6}, differences(palette(), newPal), "the red of slot 2 in the effect's row")
		assert.Equal(t, purple, paletteColour(newPal, n, 2))
	})
	t.Run("colourful", func(t *testing.T) {
		cfg := config(n, slotColourful)
		newCfg, newPal, cfgChanged, palChanged, err := recolour(cfg, palette(), purple)
		require.NoError(t, err)
		assert.True(t, cfgChanged)
		assert.True(t, palChanged)
		assert.Equal(t, []int{slotAt(n)}, differences(cfg, newCfg))
		assert.Equal(t, byte(2<<4), newCfg[slotAt(n)])
		assert.Equal(t, purple, paletteColour(newPal, n, 0))
		assert.Equal(t, []int{row, row + 2}, differences(palette(), newPal), "slot 0, red to purple")
	})
}

// An effect that takes no colour, and a block that is not whole, are refused.
func TestRecolourRefuses(t *testing.T) {
	_, _, _, _, err := recolour(config(0, 0), palette(), purple)
	require.ErrorIs(t, err, lighting.ErrNoColour)
	assert.Contains(t, err.Error(), "Off")

	self := config(12, 0)
	self[configSelfDefine] = 1
	_, _, _, _, err = recolour(self, palette(), purple)
	require.ErrorIs(t, err, lighting.ErrNoColour)

	broken := config(12, 0)
	broken[configLen-1] = 0
	_, _, _, _, err = recolour(broken, palette(), purple)
	require.ErrorIs(t, err, errBadBlock)

	_, _, _, _, err = recolour(config(12, 0), palette()[:100], purple)
	require.ErrorIs(t, err, errBadBlock)
}

// The receiver's device draws frames and sets its effect's colour, on one
// handle.
func TestTheReceiverIsAlsoOnboard(t *testing.T) {
	dev := openBoard(t, newBoard())
	assert.Equal(t, []string{"battery", "lighting", "onboard"}, sanshoku.Capabilities(dev))

	e, err := onboardOf(t, dev).Effect(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Reaction", e.Name)
	assert.Equal(t, &lighting.Colour{R: 0xff}, e.Colour)
}

// cyan is in every factory row, at slot 5.
var cyan = lighting.Colour{G: 0xff, B: 0xff}

// configPackets is how many packets a config block write takes.
const configPackets = (configLen + bodyLen - 1) / bodyLen

// A colour the effect's row holds: the config block is written, packet by
// packet, pointing the effect at it, and read back. The palette is not.
func TestSetColourPointsTheEffectAtTheColourAndReadsItBack(t *testing.T) {
	b := newBoard()
	ob := onboardOf(t, openBoard(t, b))

	wrote, err := ob.SetColour(context.Background(), cyan)

	require.NoError(t, err)
	assert.True(t, wrote)
	assert.Equal(t, bytes.Repeat([]byte{commandWriteConfig}, configPackets), b.written())
	e, err := ob.Effect(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Reaction", e.Name)
	assert.Equal(t, &cyan, e.Colour)
}

/*
A colour the row does not hold would need the palette, and a palette written
through the receiver was gone at the next power-on, with and without the
config block after it (spec 020 E4). It is refused and nothing is written, on
a slot or on colourful.
*/
func TestAColourThatNeedsThePaletteIsNotStoredYet(t *testing.T) {
	for _, slot := range []byte{0, slotColourful} {
		b := newBoard()
		b.cfg = config(12, slot)
		wrote, err := onboardOf(t, openBoard(t, b)).SetColour(context.Background(), purple)

		require.ErrorIs(t, err, ErrPaletteWrite, "slot %d", slot)
		assert.False(t, wrote)
		assert.Empty(t, b.written())
	}
}

// A restore that changes the palette writes the config block after it,
// changed or not.
func TestARestoredPaletteIsFollowedByTheConfigBlock(t *testing.T) {
	b := newBoard()
	saved := append(config(12, 0), palette()...)
	saved[configLen+int(12)*rowLen] = 0x80
	saver, ok := openBoard(t, b).(Saver)
	require.True(t, ok)

	require.NoError(t, saver.Restore(context.Background(), saved))

	want := append(bytes.Repeat([]byte{commandWritePalette}, paletteLen/bodyLen),
		bytes.Repeat([]byte{commandWriteConfig}, configPackets)...)
	assert.Equal(t, want, b.written())
}

// The colour already shown writes nothing.
func TestTheColourAlreadyShownWritesNothing(t *testing.T) {
	b := newBoard()
	ob := onboardOf(t, openBoard(t, b))

	wrote, err := ob.SetColour(context.Background(), lighting.Colour{R: 0xff})

	require.NoError(t, err)
	assert.False(t, wrote)
	assert.Empty(t, b.written())
}

// A restore that changes both blocks writes the palette first and the config
// block after it.
func TestARestoreWritesThePaletteBeforeTheConfigBlock(t *testing.T) {
	b := newBoard()
	saved := append(config(12, slotColourful), palette()...)
	saved[configLen+int(12)*rowLen] = 0x80
	saver, ok := openBoard(t, b).(Saver)
	require.True(t, ok)

	require.NoError(t, saver.Restore(context.Background(), saved))

	want := append(bytes.Repeat([]byte{commandWritePalette}, paletteLen/bodyLen),
		bytes.Repeat([]byte{commandWriteConfig}, configPackets)...)
	assert.Equal(t, want, b.written())
}

// An effect with no colour, and a config block that is not whole, write
// nothing.
func TestARefusedEffectOrABadBlockWritesNothing(t *testing.T) {
	off := newBoard()
	off.cfg = config(0, 0)
	_, err := onboardOf(t, openBoard(t, off)).SetColour(context.Background(), purple)
	require.ErrorIs(t, err, lighting.ErrNoColour)
	assert.Empty(t, off.written())

	broken := newBoard()
	broken.cfg[configLen-1] = 0
	_, err = onboardOf(t, openBoard(t, broken)).SetColour(context.Background(), purple)
	require.ErrorIs(t, err, errBadBlock)
	assert.Empty(t, broken.written())
}

// A missing acknowledgement stops the write at that packet, and says so.
func TestAMissingAcknowledgementStopsTheWrite(t *testing.T) {
	b := newBoard()
	b.withhold = 3
	ob := onboardOf(t, openBoard(t, b))

	wrote, err := ob.SetColour(context.Background(), cyan)

	require.ErrorIs(t, err, hidraw.ErrSilent)
	assert.Contains(t, err.Error(), "packet 3 of 10")
	assert.True(t, wrote, "part of the config block was written")
	assert.Len(t, b.written(), 4, "no packet after the one that went unacknowledged")
}

// An acknowledgement flagged as a checksum error stops the write too.
func TestAChecksumErrorStopsTheWrite(t *testing.T) {
	b := newBoard()
	b.badSum = 0
	_, err := onboardOf(t, openBoard(t, b)).SetColour(context.Background(), cyan)

	require.ErrorContains(t, err, "checksum")
	assert.Len(t, b.written(), 1)
}

// A block that reads back different from what was written is an error.
func TestAReadBackThatDiffersIsAnError(t *testing.T) {
	b := newBoard()
	b.lies = true
	_, err := onboardOf(t, openBoard(t, b)).SetColour(context.Background(), cyan)

	require.ErrorContains(t, err, "config block read back")
}

// Another keyboard on the receiver is never read or written.
func TestAnotherKeyboardHasNoOnboardEffect(t *testing.T) {
	b := newBoard()
	b.model = 0xA4
	ob := onboardOf(t, openBoard(t, b))

	_, err := ob.Effect(context.Background())
	require.ErrorIs(t, err, sanshoku.ErrUnsupported)
	_, err = ob.SetColour(context.Background(), purple)
	require.ErrorIs(t, err, sanshoku.ErrUnsupported)

	for _, r := range b.requests {
		assert.Equal(t, byte(commandModel), r[1])
	}
}

// A save restores what was there, writing only the block that changed; a
// save that is not whole is refused before anything is sent.
func TestASaveRestoresWhatWasThere(t *testing.T) {
	b := newBoard()
	dev := openBoard(t, b)
	saver, ok := dev.(Saver)
	require.True(t, ok)
	saved, err := saver.Save(context.Background())
	require.NoError(t, err)
	require.Len(t, saved, SaveLen)
	_, err = onboardOf(t, dev).SetColour(context.Background(), cyan)
	require.NoError(t, err)
	b.requests = nil

	require.NoError(t, saver.Restore(context.Background(), saved))

	assert.Equal(t, bytes.Repeat([]byte{commandWriteConfig}, configPackets), b.written())
	assert.Equal(t, palette(), b.pal)
	assert.Equal(t, config(12, 0), b.cfg)

	b.requests = nil
	require.ErrorIs(t, saver.Restore(context.Background(), saved[:100]), errBadBlock)
	assert.Empty(t, b.requests)
}

// A keyboard that is asleep is silent, and nothing is written.
func TestASleepingKeyboardIsSilentAndNothingIsWritten(t *testing.T) {
	b := newBoard()
	b.asleep = true
	_, err := onboardOf(t, openBoard(t, b)).SetColour(context.Background(), purple)

	require.ErrorIs(t, err, hidraw.ErrSilent)
	assert.Empty(t, b.written())
}
