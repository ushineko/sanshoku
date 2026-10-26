package aula

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/hidraw"
	"github.com/ushineko/sanshoku/lighting"
)

/*
The F75's own effect and its colour, through the receiver (spec 020).

The keyboard keeps two blocks in its flash: the config block, which names the
running effect and each effect's colour slot, and the palette, a row of seven
colours for each effect. deepan-alve/womier-l65-linux documents both for the
Womier L65, the same keyboard platform, and its l65ctl.py writes them through
the same receiver (docs/credits.md).

A block is read with one packet of operation 0, and the keyboard answers with
the block's packets, numbered. A block is written one packet at a time with
operation 1, and the keyboard acknowledges each packet with the command and the
packet's index; the command's high bit set in the acknowledgement is a
checksum error. Both blocks are flash, so they are written only by SetColour,
only where the colour differs, and nothing else here sends their commands.
*/
const (
	commandReadConfig   = 0x44
	commandWriteConfig  = 0x04
	commandReadPalette  = 0x49
	commandWritePalette = 0x09

	// operationRead and operationWrite are the high nibble of a packet's
	// length byte.
	operationRead  = 0
	operationWrite = 1

	// ackError is set on a write's acknowledgement when the keyboard found
	// the packet's checksum wrong.
	ackError = 0x80

	configLen  = 128
	paletteLen = 490

	// configEffect is the running effect; configSelfDefine is 1 while the
	// per-key colours are shown instead.
	configEffect     = 0x0A
	configSelfDefine = 0x09
	// configTable is where effect n's brightness sits, at configTable + 2n,
	// with its speed and colour slot in the byte after.
	configTable = 0x38

	// slotsPerRow is how many colours a palette row holds; slotColourful is
	// the slot that shows the effect's own several colours instead of one.
	slotsPerRow   = 7
	slotColourful = 7
	rowLen        = slotsPerRow * 3

	// firstColoured and lastColoured are the effects that take a colour. Off
	// (0) and Self-define (21) do not.
	firstColoured = 1
	lastColoured  = 18
	selfDefine    = 21
)

// configEnd is the config block's last two bytes.
var configEnd = []byte{0x5A, 0xA5}

/*
blockWait bounds one block read, every packet of it. Spec 019 measured the
palette, the larger, at 144 ms through the receiver.
*/
const blockWait = 1500 * time.Millisecond

/*
ackWait bounds the wait for one written packet's acknowledgement. Not
measured here; l65ctl.py waits for each and spec 020 E2 times them.
*/
const ackWait = 200 * time.Millisecond

// effectNames are the firmware's effects by number, as the vendor's program
// names them.
var effectNames = map[byte]string{
	0: "Off", 1: "Fixed on", 2: "Respire", 3: "Rainbow", 4: "Flash away", 5: "Raindrops",
	6: "Rainbow wheel", 7: "Ripples shining", 8: "Stars twinkle", 9: "Shadow disappear",
	10: "Retro snake", 11: "Neon stream", 12: "Reaction", 13: "Sine wave", 14: "Retinue scanning",
	15: "Rotating windmill", 16: "Colourful waterfall", 17: "Blossoming", 18: "Rotating storm",
	selfDefine: "Self-define",
}

// effectName is the effect's name, or its number where the firmware has no
// name for it.
func effectName(n byte) string {
	if name, ok := effectNames[n]; ok {
		return name
	}
	return fmt.Sprintf("effect %d", n)
}

// errBadBlock is a block that is not the length or shape it should be.
var errBadBlock = errors.New("not a whole block")

// checkBlocks refuses a config block or palette that is not whole.
func checkBlocks(cfg, pal []byte) error {
	if len(cfg) != configLen || !bytes.Equal(cfg[configLen-2:], configEnd) {
		return fmt.Errorf("the config block: %w (%d bytes)", errBadBlock, len(cfg))
	}
	if len(pal) != paletteLen {
		return fmt.Errorf("the palette: %w (%d bytes)", errBadBlock, len(pal))
	}
	return nil
}

// running is the effect the config block names, Self-define where its flag
// is set whatever the effect byte says.
func running(cfg []byte) byte {
	if cfg[configSelfDefine] == 1 {
		return selfDefine
	}
	return cfg[configEffect]
}

// coloured is whether an effect takes a colour slot.
func coloured(n byte) bool { return n >= firstColoured && n <= lastColoured }

// slotAt is where effect n's speed and colour slot are in the config block.
func slotAt(n byte) int { return configTable + 2*int(n) + 1 }

// paletteColour is slot s of effect n's palette row.
func paletteColour(pal []byte, n, s byte) lighting.Colour {
	at := int(n)*rowLen + int(s)*3
	return lighting.Colour{R: pal[at], G: pal[at+1], B: pal[at+2]}
}

// describe is the effect the two blocks say is running.
func describe(cfg, pal []byte) lighting.Effect {
	n := running(cfg)
	e := lighting.Effect{Name: effectName(n), Coloured: coloured(n)}
	if s := cfg[slotAt(n)] & 0x0F; e.Coloured && s < slotsPerRow {
		c := paletteColour(pal, n, s)
		e.Colour = &c
	}
	return e
}

/*
recolour is the two blocks with the running effect showing c, and which of
them changed. The least is changed (spec 020 R2.3):

  - the effect's slot already holds c: nothing;
  - another slot of the effect's row holds c: the config block, pointing there;
  - the effect is on a slot from 0 to 6: the palette, that slot set to c;
  - the effect is on colourful: the palette's slot 0 and the config block.

Every other byte is as it was. Pure, so the choice is tested without a device.
*/
func recolour(cfg, pal []byte, c lighting.Colour) (newCfg, newPal []byte, cfgChanged, palChanged bool, err error) {
	if err := checkBlocks(cfg, pal); err != nil {
		return nil, nil, false, false, err
	}
	n := running(cfg)
	if !coloured(n) {
		return nil, nil, false, false, fmt.Errorf("%s: %w", effectName(n), lighting.ErrNoColour)
	}
	newCfg, newPal = bytes.Clone(cfg), bytes.Clone(pal)
	at := slotAt(n)
	slot := cfg[at] & 0x0F
	if slot < slotsPerRow && paletteColour(pal, n, slot) == c {
		return newCfg, newPal, false, false, nil
	}
	for s := byte(0); s < slotsPerRow; s++ {
		if paletteColour(pal, n, s) == c {
			newCfg[at] = cfg[at]&0xF0 | s
			return newCfg, newPal, true, false, nil
		}
	}
	if slot >= slotsPerRow {
		slot = 0
		newCfg[at] = cfg[at] & 0xF0
		cfgChanged = true
	}
	p := int(n)*rowLen + int(slot)*3
	newPal[p], newPal[p+1], newPal[p+2] = c.R, c.G, c.B
	return newCfg, newPal, cfgChanged, true, nil
}

/*
Effect reads the effect the keyboard is running and its colour.

Both blocks are read: the config block names the effect and its slot, and the
palette holds the colour. A keyboard that is not an F75 is
sanshoku.ErrUnsupported and nothing is read; one that does not answer is
hidraw.ErrSilent.
*/
func (d *device) Effect(ctx context.Context) (lighting.Effect, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	cfg, pal, err := d.blocks(ctx)
	if err != nil {
		return lighting.Effect{}, fmt.Errorf("reading %s's effect: %w", d.id, err)
	}
	if err := checkBlocks(cfg, pal); err != nil {
		return lighting.Effect{}, fmt.Errorf("reading %s's effect: %w", d.id, err)
	}
	return describe(cfg, pal), nil
}

/*
SetColour makes the running effect show c and keeps the effect.

It reads both blocks, works out the least that shows c (recolour), writes it
(store) and reads both back. Nothing is written where the colour is already
shown, for an effect with no colour, or for a block that is not whole. A
write that stops part way says which block and packet.

A colour stored through the palette does not survive the keyboard being
switched off: at the next power-on it loads its factory settings, effect
included (spec 020 E4). A colour already in the effect's row, stored through
the config block alone, does. A consumer that wants the colour kept reads
Effect when the keyboard comes back and asserts it again, as hotaru does for
the Apex.
*/
func (d *device) SetColour(ctx context.Context, c lighting.Colour) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	wrote, err := d.setColour(ctx, c)
	if err != nil {
		return wrote, fmt.Errorf("setting %s's colour: %w", d.id, err)
	}
	return wrote, nil
}

func (d *device) setColour(ctx context.Context, c lighting.Colour) (bool, error) {
	cfg, pal, err := d.blocks(ctx)
	if err != nil {
		return false, err
	}
	newCfg, newPal, cfgChanged, palChanged, err := recolour(cfg, pal, c)
	if err != nil || (!cfgChanged && !palChanged) {
		return false, err
	}
	return true, d.store(ctx, newCfg, newPal, cfgChanged, palChanged)
}

// SaveLen is the length of a saved copy of the two blocks: the config block,
// then the palette.
const SaveLen = configLen + paletteLen

/*
Saver is a copy of the F75's two blocks and the way back to one, for the
bench: it saves before every write it makes, and restores from a save (spec
020 R3). Not a capability; a consumer sets a colour through lighting.Onboard.
*/
type Saver interface {
	Save(ctx context.Context) ([]byte, error)
	Restore(ctx context.Context, saved []byte) error
}

// Save reads both blocks, whole, as one copy of SaveLen bytes.
func (d *device) Save(ctx context.Context) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	cfg, pal, err := d.blocks(ctx)
	if err == nil {
		err = checkBlocks(cfg, pal)
	}
	if err != nil {
		return nil, fmt.Errorf("saving %s: %w", d.id, err)
	}
	return append(cfg, pal...), nil
}

/*
Restore writes a saved copy back: each block only where it differs from what
the keyboard holds, the palette first, and both read back after. A copy that
is not whole is refused before anything is read.
*/
func (d *device) Restore(ctx context.Context, saved []byte) error {
	if len(saved) != SaveLen {
		return fmt.Errorf("restoring %s: %w (%d bytes)", d.id, errBadBlock, len(saved))
	}
	wantCfg, wantPal := saved[:configLen], saved[configLen:]
	if err := checkBlocks(wantCfg, wantPal); err != nil {
		return fmt.Errorf("restoring %s: %w", d.id, err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	cfg, pal, err := d.blocks(ctx)
	if err == nil {
		err = d.store(ctx, wantCfg, wantPal, !bytes.Equal(cfg, wantCfg), !bytes.Equal(pal, wantPal))
	}
	if err != nil {
		return fmt.Errorf("restoring %s: %w", d.id, err)
	}
	return nil
}

/*
store writes the blocks that changed and reads both back.

A palette is always followed by the config block, changed or not. The
firmware applies settings when the config block is written, and a palette
written alone was shown at once and then gone at the next power-on, with the
keyboard on its factory settings (spec 020 E4); a config block written alone
persisted. l65ctl.py writes the config block after every palette.
*/
func (d *device) store(ctx context.Context, cfg, pal []byte, cfgChanged, palChanged bool) error {
	if !cfgChanged && !palChanged {
		return nil
	}
	cfgChanged = cfgChanged || palChanged
	if palChanged {
		if err := d.writeBlock(ctx, commandWritePalette, pal); err != nil {
			return fmt.Errorf("the palette: %w", err)
		}
	}
	if cfgChanged {
		if err := d.writeBlock(ctx, commandWriteConfig, cfg); err != nil {
			return fmt.Errorf("the config block: %w", err)
		}
	}
	gotCfg, gotPal, err := d.blocks(ctx)
	if err != nil {
		return fmt.Errorf("reading back: %w", err)
	}
	if !bytes.Equal(gotPal, pal) {
		return errors.New("the palette read back is not what was written")
	}
	if !bytes.Equal(gotCfg, cfg) {
		return errors.New("the config block read back is not what was written")
	}
	return nil
}

// blocks checks the model and reads the config block and the palette.
func (d *device) blocks(ctx context.Context) (cfg, pal []byte, err error) {
	if d.rd == nil {
		return nil, nil, os.ErrClosed
	}
	if err := d.knowModel(ctx); err != nil {
		return nil, nil, err
	}
	if d.model.id != modelF75 {
		return nil, nil, fmt.Errorf("model %#02x is not an F75: %w", d.model.id, sanshoku.ErrUnsupported)
	}
	if cfg, err = d.readBlock(ctx, commandReadConfig); err != nil {
		return nil, nil, fmt.Errorf("the config block: %w", err)
	}
	if pal, err = d.readBlock(ctx, commandReadPalette); err != nil {
		return nil, nil, fmt.Errorf("the palette: %w", err)
	}
	return cfg, pal, nil
}

/*
readBlock asks for a block and reassembles its packets by index.

The receiver pushes frames of its own on the same report, so anything that is
not a reply to command is skipped. Running out of time before every packet has
arrived is hidraw.ErrSilent.
*/
func (d *device) readBlock(ctx context.Context, command byte) ([]byte, error) {
	rctx, cancel := context.WithTimeout(ctx, blockWait)
	defer cancel()
	d.rd.Drain(hidraw.QueueDepth)
	if err := d.rd.Write(packet(command, 1, 0, operationRead, nil)); err != nil {
		return nil, err
	}
	var (
		chunks = map[byte][]byte{}
		total  byte
		buf    = make([]byte, reportLen)
		isOurs = answersTo(command)
	)
	for total == 0 || len(chunks) < int(total) {
		n, err := d.rd.Read(rctx, buf)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, fmt.Errorf("%w: %d of %d packets: %w", hidraw.ErrSilent, len(chunks), total, err)
			}
			return nil, err
		}
		r := buf[:n]
		if !isOurs(r) || r[2] == 0 {
			continue
		}
		length := min(int(r[4]&0x0F), bodyLen)
		total, chunks[r[3]] = r[2], bytes.Clone(r[5:5+length])
	}
	var out []byte
	for i := range total {
		chunk, ok := chunks[i]
		if !ok {
			return nil, fmt.Errorf("packet %d of %d is missing", i, total)
		}
		out = append(out, chunk...)
	}
	return out, nil
}

// writeBlock writes a block one packet at a time and waits for each packet's
// acknowledgement. Anything wrong with one stops the write there.
func (d *device) writeBlock(ctx context.Context, command byte, block []byte) error {
	total := (len(block) + bodyLen - 1) / bodyLen
	d.rd.Drain(hidraw.QueueDepth)
	for i := range total {
		chunk := block[i*bodyLen : min((i+1)*bodyLen, len(block))]
		p := packet(command, byte(total), byte(i), operationWrite, chunk) //nolint:gosec // at most 35 packets
		actx, cancel := context.WithTimeout(ctx, ackWait)
		ack, err := hidraw.Exchange(actx, d.rd, p, answersTo(command), reportLen)
		cancel()
		switch {
		case err != nil:
			return fmt.Errorf("packet %d of %d: %w", i, total, err)
		case ack[1]&ackError != 0:
			return fmt.Errorf("packet %d of %d: the keyboard found its checksum wrong", i, total)
		case ack[3] != byte(i): //nolint:gosec // at most 35 packets
			return fmt.Errorf("packet %d of %d: acknowledged as packet %d", i, total, ack[3])
		}
	}
	return nil
}
