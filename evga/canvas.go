package evga

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ushineko/sanshoku/lighting"
)

// frameFloor is the shortest interval at which frames are sent. A frame of
// four zones took 11 to 14 ms to write at the median, 21 ms at most, and a
// breathe and a wave at 50 ms looked smooth (spec 041).
const frameFloor = 50 * time.Millisecond

var _ lighting.Canvas = (*device)(nil)

// Keys is each lit zone, named as OpenRGB names it: "Front Logo".
func (d *device) Keys() []lighting.Key {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]lighting.Key, len(d.zones))
	for i, z := range d.zones {
		out[i] = lighting.Key{ID: byte(i), Name: zoneNames[z.index]} //nolint:gosec // at most four
	}
	return out
}

var _ lighting.Zoned = (*device)(nil)

/*
Zones are the card's lit zones, one light each, under the names OpenRGB gives
them, so scenes and rules made through OpenRGB find them (#118).
*/
func (d *device) Zones() []lighting.Zone {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]lighting.Zone, len(d.zones))
	for i, z := range d.zones {
		out[i] = lighting.Zone{Name: zoneNames[z.index], First: i, Count: 1, Shape: lighting.ShapeSingle}
	}
	return out
}

// Resize refuses: each zone is one light.
func (d *device) Resize(zone string, _ int) error {
	return fmt.Errorf("%q on %s: %w", zone, d.id, lighting.ErrNoZone)
}

// Floor is the shortest interval at which frames are sent.
func (d *device) Floor() time.Duration { return frameFloor }

/*
Frame draws px over the held colours and sends each zone that changed.

Before the first frame on this handle the card is unlocked and every zone set
to its static mode, as OpenRGB's Direct mode does. A zone is its static
register: a length byte, full brightness, then red, green, blue. A key not in
Keys is an error and nothing is sent.
*/
func (d *device) Frame(_ context.Context, px []lighting.Pixel) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.bus == nil {
		return fmt.Errorf("lighting %s: %w", d.id.Path, os.ErrClosed)
	}
	for _, p := range px {
		if int(p.ID) >= len(d.zones) {
			return fmt.Errorf("a frame to %s: key %d is not a light on it", d.id, p.ID)
		}
	}
	for _, p := range px {
		z := &d.zones[p.ID]
		if next := (colour{p.R, p.G, p.B}); z.held != next {
			z.held, z.sent = next, false
		}
	}
	if !d.claimed {
		if err := d.claim(); err != nil {
			return fmt.Errorf("a frame to %s: %w", d.id, err)
		}
	}
	for i := range d.zones {
		z := &d.zones[i]
		if z.sent {
			continue
		}
		if err := d.write(regStatic+byte(z.index), []byte{0x04, 0xFF, z.held.r, z.held.g, z.held.b}); err != nil { //nolint:gosec // index < 4
			return fmt.Errorf("a frame to %s: %w", d.id, err)
		}
		z.sent = true
	}
	return nil
}

/*
claim unlocks the card and sets every zone to static, leaving the LED counts
as they are and the zones in step, as OpenRGB's Direct mode does. The caller
holds mu.
*/
func (d *device) claim() error {
	if err := d.write(regUnlock, unlock); err != nil {
		return err
	}
	modes := []byte{0x09, modeStatic, modeStatic, modeStatic, modeStatic, keep, keep, keep, keep, 0x01}
	if err := d.write(regModes, modes); err != nil {
		return err
	}
	d.claimed = true
	return nil
}

/*
Release puts back what the card showed when it was opened: each lit zone's
static colour and the modes register, so a card that was running an effect
of its own runs it again. A handle that drew nothing sends nothing.
*/
func (d *device) Release(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.bus == nil {
		return fmt.Errorf("releasing %s: %w", d.id.Path, os.ErrClosed)
	}
	if !d.claimed {
		return nil
	}
	if err := d.write(regUnlock, unlock); err != nil {
		return fmt.Errorf("releasing %s: %w", d.id, err)
	}
	for i := range d.zones {
		z := &d.zones[i]
		if err := d.write(regStatic+byte(z.index), restored(z.found)); err != nil { //nolint:gosec // index < 4
			return fmt.Errorf("releasing %s: %w", d.id, err)
		}
		z.held, z.sent = colour{}, false
	}
	if err := d.write(regModes, restored(d.found)); err != nil {
		return fmt.Errorf("releasing %s: %w", d.id, err)
	}
	d.claimed = false
	return nil
}

// restored is a register as it was read, ready to write back: its length
// byte is the bytes after it, which is how the card reports it and takes it.
func restored(read []byte) []byte {
	out := append([]byte(nil), read...)
	out[0] = byte(len(out) - 1) //nolint:gosec // at most 10
	return out
}
