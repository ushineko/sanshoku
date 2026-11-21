package corsairdram

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ushineko/sanshoku/lighting"
)

// frameFloor is the shortest interval at which frames are sent. A frame took
// about 7 ms to write, and a breathe and a wave at 50 ms looked smooth on the
// four modules (spec 042).
const frameFloor = 50 * time.Millisecond

// zoneName is the module's one zone, as OpenRGB names it; the original
// Vengeance RGB's zone and light are named apart.
const (
	zoneName      = "Corsair DRAM"
	vengeanceZone = "Corsair Zone"
	vengeanceLED  = "Corsair LED"
)

var (
	_ lighting.Canvas = (*device)(nil)
	_ lighting.Zoned  = (*device)(nil)
)

// Keys is the module's LEDs, named as OpenRGB names them: "Corsair DRAM LED
// 0" upward.
func (d *device) Keys() []lighting.Key {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.family == familyVengeance {
		return []lighting.Key{{ID: 0, Name: vengeanceLED}}
	}
	out := make([]lighting.Key, len(d.held))
	for i := range d.held {
		out[i] = lighting.Key{ID: byte(i), Name: zoneName + " LED " + strconv.Itoa(i)} //nolint:gosec // at most 12
	}
	return out
}

// Zones is the module's LEDs as one line.
func (d *device) Zones() []lighting.Zone {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.family == familyVengeance {
		return []lighting.Zone{{Name: vengeanceZone, First: 0, Count: 1, Shape: lighting.ShapeSingle}}
	}
	return []lighting.Zone{{Name: zoneName, First: 0, Count: len(d.held), Shape: lighting.ShapeLine}}
}

// Resize refuses: a module's LEDs are its own.
func (d *device) Resize(zone string, _ int) error {
	return fmt.Errorf("%q on %s: %w", zone, d.id, lighting.ErrNoZone)
}

// Floor is the shortest interval at which frames are sent.
func (d *device) Floor() time.Duration {
	if d.family == familyVengeance {
		return vengeanceFloor
	}
	return frameFloor
}

// vengeanceFloor is the original Vengeance RGB's: a colour, five byte writes,
// takes 2.1 ms through PawnIO on an AMD B450's SMBus with the module busy
// waiting (#120; 157 ms when it slept, spec 046), so the floor is the DDR5
// modules' own.
const vengeanceFloor = frameFloor

/*
Frame draws px over the held frame and, if it changed, sends it whole: the
LED count, each LED's red, green and blue (last first on a module that lists
them so), and a CRC-8 of all of it, as SMBus blocks of 32 bytes to the first
frame register and the rest to the second. The module answers none of it. A
key not in Keys is an error and nothing is sent.
*/
func (d *device) Frame(_ context.Context, px []lighting.Pixel) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.bus == nil {
		return fmt.Errorf("lighting %s: %w", d.id.Path, os.ErrClosed)
	}
	for _, p := range px {
		if int(p.ID) >= len(d.held) {
			return fmt.Errorf("a frame to %s: key %d is not a light on it", d.id, p.ID)
		}
	}
	for _, p := range px {
		if next := (colour{p.R, p.G, p.B}); d.held[p.ID] != next {
			d.held[p.ID], d.sent = next, false
		}
	}
	if d.sent {
		return nil
	}
	if d.family == familyVengeance {
		if err := d.colour(d.held[0]); err != nil {
			return fmt.Errorf("a frame to %s: %w", d.id, err)
		}
		d.sent = true
		return nil
	}
	packet := framePacket(d.held, d.product.reverse)
	if err := d.block(regFrame1, packet[:min(32, len(packet))]); err != nil {
		return fmt.Errorf("a frame to %s: %w", d.id, err)
	}
	if len(packet) > 32 {
		if err := d.block(regFrame2, packet[32:]); err != nil {
			return fmt.Errorf("a frame to %s: %w", d.id, err)
		}
	}
	d.sent = true
	return nil
}

// framePacket is a direct frame: the LED count, the LEDs in R, G, B, and a
// CRC-8 of everything before it.
func framePacket(held []colour, reverse bool) []byte {
	n := len(held)
	p := make([]byte, 0, n*3+2)
	p = append(p, byte(n)) //nolint:gosec // at most 12
	for i := range n {
		c := held[i]
		if reverse {
			c = held[n-1-i]
		}
		p = append(p, c.r, c.g, c.b)
	}
	return append(p, crc8(p))
}

/*
Release forgets the frame and sends nothing: a module keeps its last frame,
for minutes at least (spec 042), and no command is sent that writes its
configuration.
*/
func (d *device) Release(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.bus == nil {
		return fmt.Errorf("releasing %s: %w", d.id.Path, os.ErrClosed)
	}
	clear(d.held)
	d.sent = false
	return nil
}

/*
colour sets an original Vengeance RGB's colour as OpenRGB does: no fade, the
red, green and blue registers, then the single-colour mode. The caller holds
mu.
*/
func (d *device) colour(c colour) error {
	for _, w := range [][2]byte{
		{regVengeanceFade, 0x00},
		{regVengeanceRed, c.r}, {regVengeanceGreen, c.g}, {regVengeanceBlue, c.b},
		{regVengeanceMode, 0x00},
	} {
		if err := d.write(w[0], w[1]); err != nil {
			return err
		}
	}
	return nil
}
