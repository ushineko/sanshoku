package logitech

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/ushineko/sanshoku/lighting"
)

/*
The lights of a device behind the node, drawn as a lighting.Canvas over HID++
2.0 (spec 030): the G502 X PLUS first. docs/logitech-lights.md is the
protocol, from OpenRGB, whose authors took most of it from captures of the
vendor's program.
*/

const (
	featureRGB     = 0x8071
	featurePerKey  = 0x8081
	featureOnboard = 0x8100
	featureProfile = 0x8101

	// 0x8071's functions.
	rgbInfo        = 0x00
	rgbSetEffect   = 0x01
	rgbSWControl   = 0x05
	rgbPowerConfig = 0x07
	rgbPowerMode   = 0x08

	// 0x8081's functions.
	keyInfo       = 0x00
	keyIndividual = 0x01
	keyFrameEnd   = 0x07

	// zonesPerReport is how many `zone R G B` a long SetIndividual carries.
	zonesPerReport = 4

	// busyRetry and busyFor are how a FrameEnd answered busy is sent again:
	// OpenRGB's 10 ms, for at most 250 ms.
	busyRetry = 10 * time.Millisecond
	busyFor   = 250 * time.Millisecond

	// lightsFloor is the shortest interval at which frames are sent. A
	// frame was about 24 ms on the bench, and a breathe smooth at 50 ms
	// (spec 030 E2).
	lightsFloor = 50 * time.Millisecond

	mouseZone = "Mouse LEDs"
)

// lightsWait bounds one lighting request. Handing the effect engine back was
// answered in 371 ms and 508 ms on the bench, past the battery reads' 300 ms.
// A variable so that a test of a silent mouse does not wait it out.
var lightsWait = time.Second

type rgb struct{ r, g, b uint8 }

/*
lit is the device on the node whose lights are drawn: where it is, its
features, its zones, and what this handle has done to it.
*/
type lit struct {
	index                   byte
	rgbF, keyF, onboardF    byte
	profileF                byte
	zones                   []byte
	template                []byte // the effect card's two bytes, nil for none
	effects                 byte   // cluster 0's effect count
	claimed, prepped, upped bool
	held                    []rgb
	sent                    bool
}

var (
	_ lighting.Canvas = (*device)(nil)
	_ lighting.Zoned  = (*device)(nil)
)

/*
findLights looks, once per handle, for the first device on the node with
per-key lighting, and reads its zones, its effect card and how its onboard
profiles are switched. A node with none has an empty canvas. The caller holds
mu.
*/
func (d *device) findLights(ctx context.Context) *lit {
	if d.lights != nil {
		return d.lights
	}
	for _, index := range d.indices() {
		keyF, err := featureIndex(ctx, d.rd, lightsWait, index, featurePerKey)
		if err != nil {
			continue
		}
		l := &lit{index: index, keyF: keyF}
		if l.zones, err = d.zonesOf(ctx, l); err != nil || len(l.zones) == 0 {
			continue
		}
		l.rgbF, _ = featureIndex(ctx, d.rd, lightsWait, index, featureRGB)
		l.onboardF, _ = featureIndex(ctx, d.rd, lightsWait, index, featureOnboard)
		l.profileF, _ = featureIndex(ctx, d.rd, lightsWait, index, featureProfile)
		if l.rgbF != 0 {
			if c, err := request(ctx, d.rd, lightsWait, index, l.rgbF, rgbInfo, 0x00, 0xFF); err == nil && len(c) > 4 {
				l.effects = c[4]
			}
			if card, err := requestLong(ctx, d.rd, lightsWait, index, l.rgbF, rgbInfo, 0xFF, 0x00, 0x01, 0x00, 0x01); err == nil && len(card) > 11 {
				l.template = []byte{card[10], card[11]}
			}
		}
		l.held = make([]rgb, len(l.zones))
		d.lights = l
		return l
	}
	d.lights = &lit{}
	return d.lights
}

// zonesOf reads 0x8081's zone bitmap, pages 0 to 2: zone = page × 112 + bit,
// bit 0 of page 0 skipped, as OpenRGB reads it.
func (d *device) zonesOf(ctx context.Context, l *lit) ([]byte, error) {
	var zones []byte
	for page := range 3 {
		info, err := request(ctx, d.rd, lightsWait, l.index, l.keyF, keyInfo, 0x00, byte(page), 0x00) //nolint:gosec // 0 to 2
		if err != nil {
			return nil, err
		}
		if len(info) < 16 {
			continue
		}
		for i, b := range info[2:16] {
			for bit := range 8 {
				zone := page*112 + i*8 + bit
				if b&(1<<bit) == 0 || zone == 0 || zone > 255 {
					continue
				}
				zones = append(zones, byte(zone))
			}
		}
	}
	return zones, nil
}

// Keys is one per zone, named "LED A" on, as OpenRGB names the G502 X's.
func (d *device) Keys() []lighting.Key {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rd == nil {
		return nil
	}
	l := d.findLights(context.Background())
	out := make([]lighting.Key, len(l.zones))
	for i := range out {
		out[i] = lighting.Key{ID: byte(i), Name: "LED " + string(rune('A'+i))} //nolint:gosec // at most 255 zones
	}
	return out
}

// Zones is the one zone, "Mouse LEDs", as OpenRGB names it.
func (d *device) Zones() []lighting.Zone {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rd == nil {
		return nil
	}
	return []lighting.Zone{{Name: mouseZone, Count: len(d.findLights(context.Background()).zones), Shape: lighting.ShapeSet}}
}

// Resize refuses: the mouse's zones are its own.
func (d *device) Resize(zone string, _ int) error {
	return fmt.Errorf("%q on %s: %w", zone, d.id, lighting.ErrNoZone)
}

// Floor is the shortest interval at which frames are sent.
func (d *device) Floor() time.Duration { return lightsFloor }

/*
Frame draws px over the held frame and, where it changed, sends it: claimed
and prepared first on a handle, then four zones a report and FrameEnd. A frame
that fails leaves the device unclaimed, so the next one claims it again: a
mouse asleep or out of range answers nothing.
*/
func (d *device) Frame(ctx context.Context, px []lighting.Pixel) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rd == nil {
		return fmt.Errorf("lighting %s: %w", d.id.Path, os.ErrClosed)
	}
	l := d.findLights(ctx)
	for _, p := range px {
		if int(p.ID) >= len(l.zones) {
			return fmt.Errorf("a frame to %s: key %d is not a light on it", d.id, p.ID)
		}
	}
	changed := !l.sent
	for _, p := range px {
		if next := (rgb{p.R, p.G, p.B}); l.held[p.ID] != next {
			l.held[p.ID], changed = next, true
		}
	}
	if !changed {
		return nil
	}
	l.sent = false
	if err := d.drawLights(ctx, l); err != nil {
		l.claimed, l.prepped, l.upped = false, false, false
		return fmt.Errorf("a frame to %s: %w", d.id, err)
	}
	l.sent = true
	return nil
}

// drawLights claims and prepares the device where this handle has not, and
// sends the held frame. The caller holds mu.
func (d *device) drawLights(ctx context.Context, l *lit) error {
	if !l.claimed {
		if err := d.claimLights(ctx, l); err != nil {
			return err
		}
		l.claimed = true
	}
	if !l.prepped && l.template != nil && l.rgbF != 0 {
		if err := d.prepLights(ctx, l); err != nil {
			return err
		}
		l.prepped = true
	}
	for start := 0; start < len(l.zones); start += zonesPerReport {
		var p []byte
		for i := start; i < min(start+zonesPerReport, len(l.zones)); i++ {
			c := l.held[i]
			p = append(p, l.zones[i], c.r, c.g, c.b)
		}
		if _, err := requestLong(ctx, d.rd, lightsWait, l.index, l.keyF, keyIndividual, p...); err != nil {
			return err
		}
	}
	if err := d.frameEnd(ctx, l); err != nil {
		return err
	}
	if !l.upped && l.rgbF != 0 {
		if _, err := request(ctx, d.rd, lightsWait, l.index, l.rgbF, rgbSWControl, 0x01, 0x03, 0x05); err != nil {
			return err
		}
		l.upped = true
	}
	return nil
}

/*
claimLights takes the device's lights from its firmware, as OpenRGB does: the
effect engine to software (flags 06, which leaves the firmware drawing until
the first frame), power mode active, idle and sleep timers of 60 s and 300 s,
and the onboard profiles to host mode. The caller holds mu.
*/
func (d *device) claimLights(ctx context.Context, l *lit) error {
	if l.rgbF != 0 {
		if _, err := request(ctx, d.rd, lightsWait, l.index, l.rgbF, rgbSWControl, 0x01, 0x03, 0x06); err != nil {
			return err
		}
		if _, err := request(ctx, d.rd, lightsWait, l.index, l.rgbF, rgbPowerMode, 0x01, 0x01); err != nil {
			return err
		}
		if _, err := requestLong(ctx, d.rd, lightsWait, l.index, l.rgbF, rgbPowerConfig, 0x01, 0x00, 0x00, 0x00, 0x3C, 0x01, 0x2C); err != nil {
			return err
		}
	}
	return d.profiles(ctx, l, true)
}

// profiles switches the onboard profiles to host mode, or back to onboard.
// The caller holds mu.
func (d *device) profiles(ctx context.Context, l *lit, host bool) error {
	var err error
	switch {
	case l.profileF != 0:
		mode := byte(0x03)
		if host {
			mode = 0x05
		}
		_, err = request(ctx, d.rd, lightsWait, l.index, l.profileF, 0x06, mode)
	case l.onboardF != 0:
		mode := byte(0x01)
		if host {
			mode = 0x02
		}
		_, err = request(ctx, d.rd, lightsWait, l.index, l.onboardF, 0x01, mode)
	}
	return err
}

/*
prepLights is what the vendor's program sends before per-key lighting on a
mouse with an effect card, as OpenRGB captured it: effect 2 of cluster FF with
the card's template, then the first effect index past cluster 0's last, both
kept. Without it, OpenRGB's comment says, per-key does not engage. The caller
holds mu.
*/
func (d *device) prepLights(ctx context.Context, l *lit) error {
	first := make([]byte, 16)
	first[0], first[1], first[8], first[9], first[12] = 0xFF, 0x02, l.template[0], l.template[1], 0x01
	if _, err := requestLong(ctx, d.rd, lightsWait, l.index, l.rgbF, rgbSetEffect, first...); err != nil {
		return err
	}
	second := make([]byte, 16)
	second[0], second[1], second[12] = 0xFF, l.effects, 0x01
	_, err := requestLong(ctx, d.rd, lightsWait, l.index, l.rgbF, rgbSetEffect, second...)
	return err
}

// frameEnd shows the frame, sending FrameEnd again while the device answers
// busy, for at most busyFor. The caller holds mu.
func (d *device) frameEnd(ctx context.Context, l *lit) error {
	until := time.Now().Add(busyFor)
	for {
		_, err := requestLong(ctx, d.rd, lightsWait, l.index, l.keyF, keyFrameEnd)
		if !errors.Is(err, errBusy) || time.Now().After(until) {
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("a frame to %s: %w", d.id, ctx.Err())
		case <-time.After(busyRetry):
		}
	}
}

/*
Release hands the lights back: the effect engine to the firmware, and the
onboard profiles to onboard mode. A handle that never drew sends nothing.
*/
func (d *device) Release(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rd == nil {
		return fmt.Errorf("releasing %s: %w", d.id.Path, os.ErrClosed)
	}
	l := d.lights
	if l == nil || !l.claimed {
		return nil
	}
	if l.rgbF != 0 {
		if _, err := request(ctx, d.rd, lightsWait, l.index, l.rgbF, rgbSWControl, 0x01, 0x00, 0x00); err != nil {
			return fmt.Errorf("releasing %s: %w", d.id, err)
		}
	}
	if err := d.profiles(ctx, l, false); err != nil {
		return fmt.Errorf("releasing %s: %w", d.id, err)
	}
	clear(l.held)
	l.claimed, l.prepped, l.upped, l.sent = false, false, false, false
	return nil
}
