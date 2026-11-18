package nzxt

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"
)

/*
The streaming path, as NZXT CAM 4.76.9 drives it on firmware 1.2.12 (issue #27).

A PROBE, not API: these two calls exist so sanshoku-bench can answer whether a
streamed frame stays on the panel, before anything is designed around it.

Per frame, on the HID interface and the bulk endpoint:

	36 01 00 01 <format>                 answered 37 01 in about 2 ms
	bulk: header, <format> 00 00 00, <payload length LE32>
	bulk: the payload
	36 02                                answered 37 02 in 18 to 43 ms

CAM sends `36 04` and `36 03` once when it starts, hands the screen to the
readout, clears all sixteen buckets, and then streams with no mode switch.
*/

// Stream formats seen from CAM.
const (
	StreamRGB888 = 0x09 // raw, 3 bytes a pixel, row by row
	StreamQ565   = 0x08 // compressed RGB565, "q565" header
)

/*
StreamOpen does what CAM does once before its first frame: `36 04`, `36 03`,
the readout, and every bucket emptied.
*/
func (p *panelDevice) StreamOpen(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.panel(); err != nil {
		return err
	}
	if _, err := p.ask(ctx, 0x36, 0x04); err != nil {
		return fmt.Errorf("36 04: %w", err)
	}
	if _, err := p.ask(ctx, 0x36, 0x03); err != nil {
		return fmt.Errorf("36 03: %w", err)
	}
	return p.clear(ctx)
}

/*
StreamFrame sends one frame and returns how long the device took to answer
`36 02`, which is when CAM starts the next one. Each command is asked once,
without exchange's retries, so a refusal shows as it happened.
*/
func (p *panelDevice) StreamFrame(ctx context.Context, format byte, payload []byte) (time.Duration, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.panel(); err != nil {
		return 0, err
	}
	reply, err := p.ask(ctx, 0x36, 0x01, 0x00, 0x01, format)
	if err != nil {
		return 0, fmt.Errorf("36 01: %w", err)
	}
	if reply[resultByte] != resultOK {
		return 0, fmt.Errorf("36 01 refused: %#02x", reply[resultByte])
	}
	info := make([]byte, 8)
	info[0] = format
	binary.LittleEndian.PutUint32(info[4:], uint32(len(payload))) //nolint:gosec // a probe's frame
	if err := p.usb.Bulk(ctx, bulkEndpoint, append(append([]byte(nil), header...), info...)); err != nil {
		return 0, err
	}
	if err := p.usb.Bulk(ctx, bulkEndpoint, payload); err != nil {
		return 0, err
	}
	start := time.Now()
	reply, err = p.ask(ctx, 0x36, 0x02)
	took := time.Since(start)
	if err != nil {
		return took, fmt.Errorf("36 02: %w", err)
	}
	if reply[resultByte] != resultOK {
		return took, fmt.Errorf("36 02 refused: %#02x", reply[resultByte])
	}
	p.showing = -1
	return took, nil
}
