package msi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/lighting"
	"github.com/ushineko/sanshoku/pawnio"
)

/*
The MSI mainboards whose one RGB header is driven by the Super I/O chip, a
Nuvoton NCT6795D or NCT6797D, as OpenRGB 1.1's "MSI-RGB" controller drives it
(spec 045): the B450 Tomahawk Max (MS-7C02) first. Reached through PawnIO's
LpcIO module, so Windows only, and with Administrator rights.
*/

// boardName is the board driver's name.
const boardName = "msi-board"

// boards are the boards OpenRGB drives this way, by the MS- number in their
// DMI name, and whether a board's colours are inverted (OpenRGB 1.1's
// MSIRGBControllerDetect.cpp, facts only).
var boards = map[string]bool{
	"7A40": false, "7A34": false, "7A39": false, "7A38": false, "7B79": false, "7B73": false,
	"7B61": false, "7B54": false, "7B49": false, "7B48": false, "7B45": false, "7B44": false,
	"7A59": false, "7A57": false, "7A68": false, "7B40": false, "7A94": false, "7B06": false,
	"7B08": false, "7B09": false, "7A58": false, "7A62": false, "7A69": false, "7A70": false,
	"7A72": false, "7A78": false, "7A79": false, "7A37": false, "7D95": false,
	"7B89": true, "7B90": true, "7B19": true, "7C02": true, "7B75": true, "7B22": true,
	"7B23": true, "7B24": true, "7B27": true, "7B30": true, "7B31": true, "7B51": true,
	"7C04": true, "7C00": true, "7B98": true, "7C22": true, "7C24": true, "7C01": true,
	"7C39": true, "7B86": true, "7B87": true,
}

// msiMakers are how an MSI board names its maker in DMI.
var msiMakers = map[string]bool{
	"Micro-Star International Co., Ltd.": true,
	"Micro-Star International Co., Ltd":  true,
	"MSI":                                true,
}

// Super I/O registers (OpenRGB 1.1's super_io.h and MSIRGBController.h).
const (
	sioLogicalDevice = 0x07
	sioDeviceID      = 0x20
	sioIDMask        = 0xFFF8
	sioNCT6795       = 0xD350
	sioNCT6797       = 0xD450

	ldnGPIO         = 0x09 // the logical device of the setup step
	regGPIOSetup    = 0x2C
	ldnRGB          = 0x12
	regRGBEnable    = 0xE0
	rgbEnableMask   = 0xE0
	regRGBConfig1   = 0xE4
	regRGBRainbow   = 0xFD
	regRGBConfig3   = 0xFF
	regRGBRedFirst  = 0xF0 // red F0-F3, green F4-F7, blue F8-FB
	rgbConfig3      = 0xE2 // header on, no pulse
	rgbConfig3Swap  = 0x1C // the inverted boards' colour bits
	boardFrameFloor = 50 * time.Millisecond
)

// superIOPorts are the configuration ports a Super I/O chip answers on.
var superIOPorts = []int{0x2E, 0x4E}

// Seams for tests: the DMI names, and how the LpcIO module is loaded.
var (
	readDMI = dmiBoard
	openLPC = func() (pawnio.Executor, error) { return pawnio.Open("LpcIO.bin") }
)

/*
Board finds and opens the RGB header of an MSI mainboard driven by its Super
I/O chip.

The zero value is ready to use.
*/
type Board struct{}

// Name is "msi-board".
func (Board) Name() string { return boardName }

/*
Find returns the board as a candidate when its DMI maker is MSI and its name
carries an MS- number OpenRGB drives this way, named as OpenRGB names it
("MSI B450 TOMAHAWK MAX (MS-7C02)"). Nothing is opened. ErrAbsent otherwise.
*/
func (Board) Find(context.Context) ([]sanshoku.Candidate, error) {
	maker, name := readDMI()
	if !msiMakers[maker] {
		return nil, sanshoku.ErrAbsent
	}
	for number, invert := range boards {
		if strings.Contains(name, number) {
			id := sanshoku.Identity{Vendor: 0x1462, Bus: sanshoku.BusSuperIO, Name: "MSI " + name, Path: "superio"}
			return []sanshoku.Candidate{{Identity: id, Driver: boardName, Key: "superio", Open: openBoard(id, invert)}}, nil
		}
	}
	return nil, sanshoku.ErrAbsent
}

/*
openBoard loads LpcIO and finds the chip: an NCT6795D or NCT6797D at 0x2E or
0x4E, by its device ID. Only reads.
*/
func openBoard(id sanshoku.Identity, invert bool) func(context.Context) (sanshoku.Device, error) {
	return func(context.Context) (sanshoku.Device, error) {
		e, err := openLPC()
		if err != nil {
			return nil, fmt.Errorf("%s: %w: %w", id, sanshoku.ErrUnsupported, err)
		}
		for _, port := range superIOPorts {
			s, err := pawnio.NewSuperIO(e, port)
			if err != nil {
				continue
			}
			var devID int
			err = s.In(func(c pawnio.Chip) error {
				hi, err := c.Inb(sioDeviceID)
				if err != nil {
					return err //nolint:wrapcheck // pawnio names it
				}
				lo, err := c.Inb(sioDeviceID + 1)
				devID = int(hi)<<8 | int(lo)
				return err //nolint:wrapcheck // pawnio names it
			})
			if err == nil && (devID&sioIDMask == sioNCT6795 || devID&sioIDMask == sioNCT6797) {
				id.Phys = fmt.Sprintf("Super I/O %#04x at %#x", devID, port)
				return &boardDevice{id: id, sio: s, invert: invert}, nil
			}
		}
		_ = e.Close()
		return nil, fmt.Errorf("%s: %w", id, errNoChip)
	}
}

// errNoChip is a board whose Super I/O chip is not one this driver knows.
var errNoChip = errors.New("no NCT6795D or NCT6797D answered")

// boardDevice is the board's RGB header, open.
type boardDevice struct {
	id     sanshoku.Identity
	invert bool

	mu      sync.Mutex
	sio     *pawnio.SuperIO
	held    colour
	sent    bool
	claimed bool
}

func (d *boardDevice) Identity() sanshoku.Identity { return d.id }

// Close unloads the module and sends nothing: the header keeps its colour.
func (d *boardDevice) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.sio == nil {
		return nil
	}
	err := d.sio.Close()
	d.sio = nil
	return err //nolint:wrapcheck // pawnio names it
}

var _ lighting.Canvas = (*boardDevice)(nil)

// Keys is the header's one light, as OpenRGB names it.
func (d *boardDevice) Keys() []lighting.Key { return []lighting.Key{{ID: 0, Name: "MSI LED"}} }

// Floor is the shortest interval at which frames are sent.
func (d *boardDevice) Floor() time.Duration { return boardFrameFloor }

/*
Frame sets the header's colour. Before the first on this handle, the chip is
readied as OpenRGB readies it: the setup bit on logical device 9, the RGB
logical device enabled, no pulse or blink, the header on with the board's
colour order, and the rainbow mode off. The colour is four bits a channel,
repeated in each channel's four registers. Every sequence holds the ISA bus.
*/
func (d *boardDevice) Frame(_ context.Context, px []lighting.Pixel) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.sio == nil {
		return fmt.Errorf("lighting %s: %w", d.id, os.ErrClosed)
	}
	for _, p := range px {
		if p.ID != 0 {
			return fmt.Errorf("a frame to %s: key %d is not a light on it", d.id, p.ID)
		}
		if next := (colour{p.R, p.G, p.B}); next != d.held {
			d.held, d.sent = next, false
		}
	}
	if d.sent && d.claimed {
		return nil
	}
	err := d.sio.In(func(c pawnio.Chip) error {
		if !d.claimed {
			if err := d.claim(c); err != nil {
				return err
			}
		}
		return d.write(c)
	})
	if err != nil {
		return fmt.Errorf("a frame to %s: %w", d.id, err)
	}
	d.claimed, d.sent = true, true
	return nil
}

func (d *boardDevice) claim(c pawnio.Chip) error {
	if err := c.Outb(sioLogicalDevice, ldnGPIO); err != nil {
		return err //nolint:wrapcheck // pawnio names it
	}
	v, err := c.Inb(regGPIOSetup)
	if err != nil {
		return err //nolint:wrapcheck // pawnio names it
	}
	if err := c.Outb(regGPIOSetup, v&^0x08|0x10); err != nil {
		return err //nolint:wrapcheck // pawnio names it
	}
	if err := c.Outb(sioLogicalDevice, ldnRGB); err != nil {
		return err //nolint:wrapcheck // pawnio names it
	}
	en, err := c.Inb(regRGBEnable)
	if err != nil {
		return err //nolint:wrapcheck // pawnio names it
	}
	if en&rgbEnableMask != rgbEnableMask {
		if err := c.Outb(regRGBEnable, rgbEnableMask); err != nil {
			return err //nolint:wrapcheck // pawnio names it
		}
	}
	cfg := byte(rgbConfig3)
	if d.invert {
		cfg |= rgbConfig3Swap
	}
	for _, w := range [][2]byte{{regRGBConfig1, 0x00}, {regRGBConfig3, cfg}, {regRGBRainbow, 0x00}} {
		if err := c.Outb(w[0], w[1]); err != nil {
			return err //nolint:wrapcheck // pawnio names it
		}
	}
	return nil
}

// write sets the colour registers, the RGB logical device selected first.
func (d *boardDevice) write(c pawnio.Chip) error {
	if err := c.Outb(sioLogicalDevice, ldnRGB); err != nil {
		return err //nolint:wrapcheck // pawnio names it
	}
	for ch, v := range [3]byte{d.held.r >> 4, d.held.g >> 4, d.held.b >> 4} {
		for i := range 4 {
			if err := c.Outb(regRGBRedFirst+byte(ch*4+i), v|v<<4); err != nil { //nolint:gosec // 12 registers
				return err //nolint:wrapcheck // pawnio names it
			}
		}
	}
	return nil
}

// Release forgets the colour and sends nothing: no command is known that
// returns the header to a stored effect.
func (d *boardDevice) Release(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.sio == nil {
		return fmt.Errorf("releasing %s: %w", d.id, os.ErrClosed)
	}
	d.held, d.sent = colour{}, false
	return nil
}
