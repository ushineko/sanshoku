package corsair

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/ushineko/sanshoku/lighting"
)

// lights is how many lights the MM700 has.
const lights = 3

// keys are the MM700's lights, in the order a frame carries them.
var keys = [lights]lighting.Key{{ID: 0, Name: "Left"}, {ID: 1, Name: "Right"}, {ID: 2, Name: "Logo"}}

/*
frameFloor is the shortest interval at which frames are sent. A frame is three
exchanges, about 17 ms; a breathe was smooth at 33 ms and at 16 ms, where the
frames ran back to back (spec 026).
*/
const frameFloor = 20 * time.Millisecond

type colour struct{ r, g, b uint8 }

var (
	_ lighting.Canvas = (*device)(nil)
	_ lighting.Zoned  = (*device)(nil)
)

// Keys is the pad's three lights: Left, Right, Logo.
func (d *device) Keys() []lighting.Key { return append([]lighting.Key(nil), keys[:]...) }

// Zones is each light as its own zone, as OpenRGB names them: Left, Right
// and Logo. A consumer's scenes written against OpenRGB address them so.
func (d *device) Zones() []lighting.Zone {
	out := make([]lighting.Zone, lights)
	for i, k := range keys {
		out[i] = lighting.Zone{Name: k.Name, First: i, Count: 1, Shape: lighting.ShapeSingle}
	}
	return out
}

// Resize refuses: the pad's zones are its own.
func (d *device) Resize(zone string, _ int) error {
	return fmt.Errorf("%q on %s: %w", zone, d.id, lighting.ErrNoZone)
}

// Floor is 20 ms.
func (d *device) Floor() time.Duration { return frameFloor }

/*
Frame draws px over the held frame and sends it where it changed.

The pad shows a frame only in software mode, and in hardware mode accepts one
without showing it, so the mode is read first and software mode claimed when
it reads hardware: OpenRGB's exit, a replug or anything else can put it back
without this driver knowing. A key not in Keys is an error and nothing is
sent.
*/
func (d *device) Frame(ctx context.Context, px []lighting.Pixel) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rd == nil {
		return fmt.Errorf("lighting %s: %w", d.id.Path, os.ErrClosed)
	}
	next := d.held
	for _, p := range px {
		if int(p.ID) >= lights {
			return fmt.Errorf("a frame to %s: key %d is not a light on it", d.id, p.ID)
		}
		next[p.ID] = colour{p.R, p.G, p.B}
	}
	if d.sent && next == d.held {
		return nil
	}
	d.held, d.sent = next, false
	if err := d.claim(ctx); err != nil {
		return fmt.Errorf("a frame to %s: %w", d.id, err)
	}
	if err := d.send(ctx); err != nil {
		return fmt.Errorf("a frame to %s: %w", d.id, err)
	}
	d.sent = true
	return nil
}

/*
Release hands the pad back to its stored effect (hardware mode) and forgets
the frame. A frame after a release starts from dark.
*/
func (d *device) Release(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rd == nil {
		return fmt.Errorf("releasing %s: %w", d.id.Path, os.ErrClosed)
	}
	if err := d.set(ctx, propMode, modeHardware); err != nil {
		return fmt.Errorf("releasing %s: %w", d.id, err)
	}
	d.held, d.sent = [lights]colour{}, false
	return nil
}

// claim puts the pad in software mode where it reads otherwise. The caller
// holds mu.
func (d *device) claim(ctx context.Context) error {
	mode, err := d.get(ctx, propMode)
	if err != nil {
		return err
	}
	if mode == modeSoftware {
		return nil
	}
	return d.set(ctx, propMode, modeSoftware)
}

/*
send writes the held frame: open the lighting endpoint, one block of the
lights' reds, then greens, then blues, close. The caller holds mu.

Once the open is answered the endpoint is closed whatever happens to the
block, on a time of its own if the frame's context has ended: a frame cut off
between the two, as a stopping service's is, left the pad refusing every
open until something closed it (spec 036 R1).
*/
func (d *device) send(ctx context.Context) error {
	if err := d.openEndpoint(ctx); err != nil {
		// Written and not answered, as when the frame is cut off while the
		// reply is on its way: the pad may have opened it, so it is closed.
		// A refusal opened nothing.
		if !errors.Is(err, errStatus) {
			_ = d.shut(ctx)
		}
		return err
	}
	_, sent := d.exchange(ctx, block(d.held)...)
	shut := d.shut(ctx)
	if sent != nil {
		return sent
	}
	return shut
}

// statusInUse is the status an open is answered with while the endpoint is
// already open, by another program or a handle that never closed it.
const statusInUse = 0x03

/*
openEndpoint opens the lighting endpoint. Refused as in use, it closes it and
opens it once more: the pad stayed refusing after a frame cut off between its
open and close, and after a program under Wine held it, and one close freed it
each time (spec 036 R2). The caller holds mu.
*/
func (d *device) openEndpoint(ctx context.Context) error {
	_, err := d.exchange(ctx, wired, cmdOpen, 0x00, lightingEndpoint)
	var status *refused
	if errors.As(err, &status) && status.status == statusInUse {
		if _, closeErr := d.exchange(ctx, wired, cmdClose, 0x01, 0x00); closeErr == nil {
			_, err = d.exchange(ctx, wired, cmdOpen, 0x00, lightingEndpoint)
		}
	}
	if err != nil {
		return err
	}
	d.open = true
	return nil
}

/*
shut closes the lighting endpoint, on a time of its own where ctx has ended,
so a cancelled frame still closes what it opened. The caller holds mu.
*/
func (d *device) shut(ctx context.Context) error {
	if ctx.Err() != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.WithoutCancel(ctx), replyWait)
		defer cancel()
	}
	if _, err := d.exchange(ctx, wired, cmdClose, 0x01, 0x00); err != nil {
		return err
	}
	d.open = false
	return nil
}

// block is the first (and, for three lights, only) block of a frame: its
// length, then the colours planar.
func block(held [lights]colour) []byte {
	data := make([]byte, 3*lights)
	for i, c := range held {
		data[i], data[lights+i], data[2*lights+i] = c.r, c.g, c.b
	}
	return append([]byte{wired, cmdBlock, 0x00, 3 * lights, 0x00, 0x00, 0x00}, data...)
}
