package steelseries

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku/battery"
	"github.com/ushineko/sanshoku/hidraw"
	"github.com/ushineko/sanshoku/lighting"
)

// apexAck is the keyboard's acknowledgement of an 85-key frame as the GG
// capture of 2026-09-30 recorded it, up to where the bytes stop changing.
var apexAck = []byte{0x61, 0x00, 0x55, 0x34, 0x2e, 0x31}

// The frame is GG's: a zero report number, 0x61, the count, then
// (key r g b) for each pixel and zeros to 641 bytes.
func TestAFrameIsEncodedAsGGSentIt(t *testing.T) {
	got, err := encodeFrame([]lighting.Pixel{{ID: 0x29, R: 0xff}, {ID: 0x04, G: 0x80, B: 0x01}})

	require.NoError(t, err)
	require.Len(t, got, 1+frameSize, "a report number and the 641 bytes the descriptor declares")
	assert.Equal(t, []byte{0x00, 0x61, 0x02, 0x29, 0xff, 0x00, 0x00, 0x04, 0x00, 0x80, 0x01}, got[:11])
	assert.Equal(t, make([]byte, 1+frameSize-11), got[11:], "the rest of the report is zero")
}

// 159 pixels fill the report; 160 are an error, not a frame that silently
// leaves keys out.
func TestAFrameHoldsAtMost159Pixels(t *testing.T) {
	full := make([]lighting.Pixel, 159)
	for i := range full {
		full[i] = lighting.Pixel{ID: byte(i), R: 1, G: 2, B: 3}
	}
	got, err := encodeFrame(full)
	require.NoError(t, err)
	assert.Equal(t, byte(159), got[2])
	assert.Equal(t, []byte{158, 1, 2, 3}, got[3+4*158:3+4*159], "the last pixel is the report's last whole one")

	_, err = encodeFrame(append(full, lighting.Pixel{}))
	require.Error(t, err)
}

// The Apex is a canvas: a frame goes out as one feature report and is
// acknowledged, and the battery still reads on the same handle afterwards.
func TestTheApexAcknowledgesAFrameAndStillReadsItsBattery(t *testing.T) {
	f := &fake{
		ack:   apexAck,
		reply: map[byte][]byte{batteryCommand: {batteryCommand, 0x15}},
	}
	c := candidate(apexNode, 20*time.Millisecond, func(string) (node, error) { return f, nil })
	dev, err := c.Open(context.Background())
	require.NoError(t, err)

	canvas, ok := dev.(lighting.Canvas)
	require.True(t, ok, "the Apex is a lighting.Canvas")
	assert.Len(t, canvas.Keys(), 85)
	require.NoError(t, canvas.Frame(context.Background(), []lighting.Pixel{{ID: 0x29, R: 0xff}}))
	require.Len(t, f.features, 1)
	assert.Equal(t, []byte{0x00, 0x61, 0x01, 0x29, 0xff, 0x00, 0x00}, f.features[0][:7])

	found, err := dev.(battery.Source).Batteries(context.Background())
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, 100, found[0].Level)
}

// A frame the keyboard does not acknowledge is an error, so a consumer
// streaming to a board that stopped listening finds out.
func TestAnUnacknowledgedFrameIsSilent(t *testing.T) {
	f := &fake{}
	c := candidate(apexNode, 20*time.Millisecond, func(string) (node, error) { return f, nil })
	dev, err := c.Open(context.Background())
	require.NoError(t, err)

	err = dev.(lighting.Canvas).Frame(context.Background(), nil)
	require.ErrorIs(t, err, hidraw.ErrSilent)
}

// A late acknowledgement of an earlier frame is drained, not taken for the
// next frame's.
func TestAStaleAcknowledgementIsNotTakenForTheFrames(t *testing.T) {
	f := &fake{queued: [][]byte{apexAck}}
	c := candidate(apexNode, 20*time.Millisecond, func(string) (node, error) { return f, nil })
	dev, err := c.Open(context.Background())
	require.NoError(t, err)

	err = dev.(lighting.Canvas).Frame(context.Background(), nil)
	require.ErrorIs(t, err, hidraw.ErrSilent, "the queued acknowledgement was read as this frame's")
	assert.Equal(t, 1, f.drained)
}

// Only the Apex is a canvas: the Arctis base station and the mice open as
// battery sources and nothing more.
func TestAProductWithNoCanvasIsNotOne(t *testing.T) {
	for _, n := range []hidraw.Node{novaProNode, {Path: "/dev/hidraw9", Name: "SteelSeries Aerox 3 Wireless", Vendor: steelseriesVendor, Product: 0x1838}} {
		f := &fake{}
		c := candidate(n, 20*time.Millisecond, func(string) (node, error) { return f, nil })
		dev, err := c.Open(context.Background())
		require.NoError(t, err)
		_, ok := dev.(lighting.Canvas)
		assert.False(t, ok, "%s is not a canvas", n.Name)
		assert.Empty(t, f.features)
	}
}
