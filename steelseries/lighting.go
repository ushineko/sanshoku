package steelseries

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ushineko/sanshoku/hidraw"
	"github.com/ushineko/sanshoku/lighting"
)

/*
The Apex's direct frame, from a usbmon capture of SteelSeries GG streaming
its effects to the Apex Pro TKL Wireless Gen 3 through its receiver
(1038:1644) on 2026-09-30, spec 010.

GG renders every effect and preset on the host and sends each frame as one
feature report with no report ID (SET_REPORT wValue 0x0300) on interface 3:
`61 <n> (key r g b)×n`, zero to the end. The keyboard answers each with an
input report on the same interface that starts `61`. 2065 frames in 116
seconds, nothing else after GG's startup burst.
*/
const (
	// frameReport is the frame's first byte, as GG sent it through the
	// receiver and OpenRGB sends it on both products. Measured on
	// 2026-10-01 (spec 010 E1): 0x61 shows on the cable and through the
	// receiver; SignalRGB's 0x21 shows on the cable only, and through the
	// receiver is not acknowledged.
	frameReport = 0x61

	// frameSize is the feature report's width: what the 0xFFC0 collection
	// declares for usage 0xF2 (report count 0x281 on both captures of the
	// descriptor) and the length GG's SET_REPORT carried.
	frameSize = 641

	// framePixels is how many (key r g b) fit after the id and the count.
	framePixels = (frameSize - 2) / 4
)

/*
frameFloor is the shortest interval at which frames are shown.

Measured on 2026-10-01 (spec 010 E2), through the receiver: a moving wave at
16, 33, 56 and 100 ms for 30 s each, every frame acknowledged (median 17 ms,
worst 31 ms) and no tearing seen at any of them. A Frame call waits for its
acknowledgement, so at 16 ms the stream runs at the acknowledgement's pace,
about 57 frames a second; on the cable an acknowledgement takes about 4 ms.
GG streams at 56 ms.
*/
const frameFloor = 16 * time.Millisecond

/*
releaseCommand hands the lighting back to the firmware by rebooting the
keyboard: an output report of 0x41 alone, OpenRGB's "onboard" command.

The firmware does not take the lighting back on its own. On 2026-10-01
(spec 010 E4) a 10-second steady stream was stopped and the board held its
last frame, unchanged, for five minutes. A probe on 2026-09-30 found 0x41
(and 0x01) re-enumerate the keyboard, after which its onboard effect shows;
nothing gentler was found in a sweep of every bare command.
*/
const releaseCommand = 0x41

// canvasProducts are the products that are a lighting.Canvas: the Apex on
// both of its connections. No other product in the allow-list is.
var canvasProducts = map[uint16]bool{0x1644: true, 0x1646: true}

/*
apexKeys are the 85 lights GG addressed on this board, in the order its
frames carried them, named by HID usage.

0x32 (the non-US hash), 0x46-0x48 and 0x64 were not in GG's frames, and
on 2026-10-01 (spec 010 E3) a frame naming them was acknowledged and lit
nothing; nor did key 0x00, which is not a broadcast on this board. A light a
frame does not name keeps its colour. A full-size board or another layout has more
keys; this list is one US TKL's.
*/
var apexKeys = []lighting.Key{
	{ID: 0x04, Name: "A"}, {ID: 0x05, Name: "B"}, {ID: 0x06, Name: "C"}, {ID: 0x07, Name: "D"},
	{ID: 0x08, Name: "E"}, {ID: 0x09, Name: "F"}, {ID: 0x0A, Name: "G"}, {ID: 0x0B, Name: "H"},
	{ID: 0x0C, Name: "I"}, {ID: 0x0D, Name: "J"}, {ID: 0x0E, Name: "K"}, {ID: 0x0F, Name: "L"},
	{ID: 0x10, Name: "M"}, {ID: 0x11, Name: "N"}, {ID: 0x12, Name: "O"}, {ID: 0x13, Name: "P"},
	{ID: 0x14, Name: "Q"}, {ID: 0x15, Name: "R"}, {ID: 0x16, Name: "S"}, {ID: 0x17, Name: "T"},
	{ID: 0x18, Name: "U"}, {ID: 0x19, Name: "V"}, {ID: 0x1A, Name: "W"}, {ID: 0x1B, Name: "X"},
	{ID: 0x1C, Name: "Y"}, {ID: 0x1D, Name: "Z"}, {ID: 0x1E, Name: "1"}, {ID: 0x1F, Name: "2"},
	{ID: 0x20, Name: "3"}, {ID: 0x21, Name: "4"}, {ID: 0x22, Name: "5"}, {ID: 0x23, Name: "6"},
	{ID: 0x24, Name: "7"}, {ID: 0x25, Name: "8"}, {ID: 0x26, Name: "9"}, {ID: 0x27, Name: "0"},
	{ID: 0x29, Name: "Escape"}, {ID: 0x2A, Name: "Backspace"}, {ID: 0x2B, Name: "Tab"}, {ID: 0x2D, Name: "-"},
	{ID: 0x2E, Name: "="}, {ID: 0x2F, Name: "["}, {ID: 0x30, Name: "]"}, {ID: 0x33, Name: ";"},
	{ID: 0x34, Name: "'"}, {ID: 0x35, Name: "`"}, {ID: 0x36, Name: ","}, {ID: 0x37, Name: "."},
	{ID: 0x38, Name: "/"}, {ID: 0x39, Name: "Caps Lock"}, {ID: 0x3A, Name: "F1"}, {ID: 0x3B, Name: "F2"},
	{ID: 0x3C, Name: "F3"}, {ID: 0x3D, Name: "F4"}, {ID: 0x3E, Name: "F5"}, {ID: 0x3F, Name: "F6"},
	{ID: 0x40, Name: "F7"}, {ID: 0x41, Name: "F8"}, {ID: 0x42, Name: "F9"}, {ID: 0x43, Name: "F10"},
	{ID: 0x44, Name: "F11"}, {ID: 0x45, Name: "F12"}, {ID: 0xFB, Name: "Media"}, {ID: 0x49, Name: "Insert"},
	{ID: 0x4A, Name: "Home"}, {ID: 0x4B, Name: "Page Up"}, {ID: 0x4C, Name: "Delete"}, {ID: 0x4D, Name: "End"},
	{ID: 0x4E, Name: "Page Down"}, {ID: 0x4F, Name: "Right"}, {ID: 0x50, Name: "Left"}, {ID: 0x51, Name: "Down"},
	{ID: 0x52, Name: "Up"}, {ID: 0xE0, Name: "Left Control"}, {ID: 0xE1, Name: "Left Shift"}, {ID: 0xE2, Name: "Left Alt"},
	{ID: 0xE3, Name: "Left GUI"}, {ID: 0xE4, Name: "Right Control"}, {ID: 0xE5, Name: "Right Shift"}, {ID: 0xE7, Name: "Right GUI"},
	{ID: 0xF0, Name: "SteelSeries"}, {ID: 0x2C, Name: "Space"}, {ID: 0x28, Name: "Enter"}, {ID: 0x31, Name: `\`},
	{ID: 0xE6, Name: "Right Alt"},
}

/*
encodeFrame is one frame as the feature report SetFeature sends: a leading
zero report number, then `61 <n> (key r g b)×n` and zeros to frameSize.

More pixels than fit is an error rather than a truncation, so a consumer
that addresses a bigger board than this one finds out.
*/
func encodeFrame(px []lighting.Pixel) ([]byte, error) {
	if len(px) > framePixels {
		return nil, fmt.Errorf("a frame of %d pixels; the Apex takes at most %d", len(px), framePixels)
	}
	report := make([]byte, 1+frameSize)
	report[1], report[2] = frameReport, byte(len(px)) //nolint:gosec // at most framePixels, checked above
	for i, p := range px {
		copy(report[3+4*i:], []byte{p.ID, p.R, p.G, p.B})
	}
	return report, nil
}

// apex is an open Apex control endpoint: the battery as every allow-listed
// product has it, and the canvas only this product has.
type apex struct {
	*device
}

// Keys is the 85 keys GG addressed on this board, in GG's order.
func (a *apex) Keys() []lighting.Key {
	return append([]lighting.Key(nil), apexKeys...)
}

// Floor is the shortest frame interval measured to show every frame: 16 ms.
func (a *apex) Floor() time.Duration { return frameFloor }

/*
Frame sends one frame and waits for the keyboard's acknowledgement.

The battery reader on the same handle drains before it asks, so an
acknowledgement nobody waited for cannot be read as a battery reply; this
drains before it sends for the same reason, so a late acknowledgement of the
previous frame is not taken for this one's. A frame that is not acknowledged
within the driver's Timeout is hidraw.ErrSilent; an unplugged keyboard is
sanshoku.ErrGone.
*/
func (a *apex) Frame(ctx context.Context, px []lighting.Pixel) error {
	report, err := encodeFrame(px)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rd == nil {
		return fmt.Errorf("lighting %s: %w", a.id.Path, os.ErrClosed)
	}

	actx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	matches := func(r []byte) bool { return len(r) > 0 && r[0] == frameReport }
	a.rd.Drain(hidraw.QueueDepth)
	if _, err := hidraw.Exchange(actx, featureWriter{actx, a.rd}, report, matches, reportSize); err != nil {
		return fmt.Errorf("a frame to %s: %w", a.id.Path, err)
	}
	return nil
}

/*
Release hands the lighting back to the firmware by rebooting the keyboard.

The keyboard holds the last frame it was sent indefinitely, so stopping the
stream is not enough to give the lighting back; Release sends 0x41, the
keyboard re-enumerates and comes back showing its onboard effect, as it does
when OpenRGB exits. **The device is gone afterwards**: its hidraw node moves,
every call on this handle returns sanshoku.ErrGone, and the consumer closes
it and scans again, battery reader included. A consumer that only wants the
board dark sends a black frame instead.
*/
func (a *apex) Release(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rd == nil {
		return fmt.Errorf("releasing %s: %w", a.id.Path, os.ErrClosed)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("releasing %s: %w", a.id.Path, err)
	}
	req := make([]byte, reportSize+1) // a leading report number of zero
	req[1] = releaseCommand
	if err := a.rd.Write(req); err != nil {
		return fmt.Errorf("releasing %s: %w", a.id.Path, err)
	}
	return nil
}

// featureWriter is a node whose Write sends a feature report, so that
// hidraw.Exchange can send a frame and match its acknowledgement.
type featureWriter struct {
	ctx context.Context
	node
}

// Write sends report as a feature report.
func (f featureWriter) Write(report []byte) error {
	return f.SetFeature(f.ctx, report) //nolint:wrapcheck // hidraw names the node
}
