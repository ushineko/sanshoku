# 020 — AULA F75: the colour of its own effect

**Issue**: #56

## Status: INCOMPLETE

## Context

The F75's firmware runs a lighting effect of its own. On the desk it runs
Reaction: a key lights when it is typed. The user wants a program asked for
purple to keep Reaction and make it purple.

Spec 019 gave the keyboard a `lighting.Canvas`. A canvas replaces the effect
with frames, and on this keyboard a frame fades within a second or two unless
it is sent again. That draws over the effect; it cannot recolour it.

The effect's colour is stored in the keyboard, in two blocks of its flash.
Spec 019 read both through the receiver and wrote neither. Its R1.10 forbids
the writes, and its Alternatives record why: they change what the keyboard
does when nothing drives it. The user decided on 2026-10-26 that this is what
they want, with two limits: a write happens only when someone asks for a
colour, and only when the stored colour differs. F75 only.

### What is known

From [womier-l65-linux](https://github.com/deepan-alve/womier-l65-linux)
(MIT): `docs/L65-protocol.md`, and `linux/l65ctl.py`, whose `colors` command
does this through the same receiver. The Womier L65 is the same keyboard
platform (SinoWealth SH68F90, Compx BK3632 receiver `3554:fa09`).

- **Framing.** The receiver's 20-byte report `0x13`, as spec 013 describes:
  `13 CMD NPKG IDX (OPER<<4 | LEN) <14 data bytes> CSUM`, where the checksum
  is the sum of bytes 0 to 18.
- **Reads.** One packet with operation 0. The keyboard answers with `NPKG`
  packets, reassembled by index. A read command is the write command plus
  `0x40`.
- **Writes.** One packet per 14 bytes with operation 1. The keyboard
  acknowledges each packet: the command echoed, the index echoed, and the
  command's high bit set on a checksum error. A command whose own high bit is
  set, such as the frame `0x88`, is not acknowledged.
- **Config block**, 128 bytes, read `0x44`, write `0x04`, ending `5A A5`:
  - `0x0A` the current effect;
  - `0x09` 1 while Self-define (per-key colours) is active;
  - `0x38 + 2n` effect n's brightness, 0 to 4;
  - `0x39 + 2n` effect n's speed (high nibble, 0 to 4) and colour slot (low
    nibble, 0 to 7).
- **Palette**, 490 bytes through the receiver, read `0x49`, write `0x09`: 23
  rows of seven colours, three bytes each. Row n is effect n's. Slot 0 to 6
  picks a colour from the row; slot 7 is "colourful", the effect's own
  several colours. The cable's palette is 512 bytes (commands `0x8A` and
  `0x0A`).
- **Effects** by number: 0 off, 1 Fixed on, 2 Respire, 3 Rainbow, 4 Flash
  away, 5 Raindrops, 6 Rainbow wheel, 7 Ripples shining, 8 Stars twinkle, 9
  Shadow disappear, 10 Retro snake, 11 Neon stream (the factory default), 12
  Reaction, 13 Sine wave, 14 Retinue scanning, 15 Rotating windmill, 16
  Colourful waterfall, 17 Blossoming, 18 Rotating storm, 21 Self-define. Off
  and Self-define take no colour slot.
- **Measured here** (spec 019): the config read takes 47 ms in 10 packets and
  the palette read 144 ms in 35 packets, through the receiver on this
  keyboard. Both read back unchanged after a frame session.
- **Persistence.** Settings survive unplugging. A reboot through the chip's
  bootloader restores a factory template for the config block and palette.

Nothing in that list about writes has been measured on this keyboard.

## Requirements

### R1. A capability for an effect's colour

- R1.1 A new optional interface in `lighting`:

  ```go
  // Onboard is a device whose firmware runs a lighting effect of its own,
  // with a colour the device stores.
  type Onboard interface {
      // Effect reads the effect the firmware is running now.
      Effect(ctx context.Context) (Effect, error)

      // SetColour makes the running effect show c and keeps the effect. It
      // writes the device only where the stored colour differs, and says
      // whether it wrote.
      SetColour(ctx context.Context, c Colour) (wrote bool, err error)
  }

  // Effect is a firmware effect as the device reports it.
  type Effect struct {
      Name string
      // Coloured is whether the effect takes a colour. Off does not.
      Coloured bool
      // Colour is the effect's colour, or nil where it shows several.
      Colour *Colour
  }

  // Colour is one colour, absolute.
  type Colour struct{ R, G, B uint8 }
  ```

- R1.2 `ErrNoColour` is an effect that takes no colour. `SetColour` on one
  returns it wrapped with the effect's name and writes nothing.
- R1.3 `sanshoku.Capabilities` reports `"onboard"` for a device that
  implements it, and the support entry gains the capability.
- R1.4 `Canvas` and `Fading` do not change.

### R2. The F75 through its receiver

- R2.1 The receiver's device implements `Onboard` on the same open handle as
  its battery and canvas. The model question (spec 019 R1.2) is asked before
  the first read of either block, and anything but an F75 is refused with
  nothing read or written.
- R2.2 `Effect` reads the config block. A block that is not 128 bytes, or
  does not end `5A A5`, is an error. While Self-define is active (`0x09` is
  1), the effect is Self-define whatever `0x0A` says.
- R2.3 `SetColour` reads the config block and the palette, then writes the
  least that makes the running effect show the colour:
  - the effect's slot already holds the colour: nothing;
  - another slot in the effect's row holds it: the config block only, with
    the effect's slot pointed at that slot;
  - otherwise, with the effect on a slot from 0 to 6: the palette only, with
    that slot set to the colour;
  - otherwise, with the effect on colourful (slot 7): the palette's slot 0
    for that effect, then the config block pointing at slot 0.
- R2.4 Every other byte of both blocks is written back as it was read. The
  effect number, brightness and speed are never changed, and no other row of
  the palette is touched.
- R2.5 The palette is written before the config block, because the firmware
  applies settings when the config block is written.
- R2.6 Every write packet waits for its acknowledgement, up to 200 ms. No
  acknowledgement, a checksum error, or an index that does not match is an
  error, and no later packet is sent. A write that stops part way is reported
  as that, with the block it was in.
- R2.7 After writing, both blocks are read back and compared with what was
  meant. A difference is an error that names the block.
- R2.8 A keyboard that does not answer a read (asleep or out of range) is
  `hidraw.ErrSilent`, and nothing is written.
- R2.9 Spec 019's R1.10 is replaced: the frame path sends `0x4A`, `0x05` and
  `0x88` only, as before; `0x44`, `0x49`, `0x04` and `0x09` are sent only by
  `Effect` and `SetColour`, and by `aula.Saver`'s `Save` and `Restore`, which
  the bench uses (R3).
- R2.10 The cable's device does not implement `Onboard` in this spec. Its
  blocks use other commands and another palette length, and it has not been
  measured.

### R3. The bench

- R3.1 `sanshoku-bench onboard` prints the running effect, its colour, and
  the effect's palette row.
- R3.2 `sanshoku-bench onboard --save FILE` writes both blocks to a file as
  read. Every write through the bench saves first, to a file it names.
- R3.3 `sanshoku-bench onboard --colour RRGGBB --yes` calls `SetColour` and
  prints whether it wrote and what it wrote.
- R3.4 `sanshoku-bench onboard --restore FILE --yes` writes both blocks back
  from a saved file, after checking the length and the end marker.

### R4. Tests

- R4.1 Packets as pure functions: a write of each block is the packets
  `l65ctl.py` would send, byte for byte, with indexes, counts, lengths and
  checksums.
- R4.2 The choice in R2.3, as a pure function of the two blocks: each of the
  four cases, and that every byte outside the change is the same.
- R4.3 On the fake receiver, which acknowledges writes as R2.6 describes and
  can withhold one: the colour already shown writes nothing; a refused effect
  writes nothing; a bad end marker writes nothing; a missing acknowledgement
  stops the write and is an error; a read-back that differs is an error.
- R4.4 The guard from spec 019 R4.4 changes with R2.9: only tests that call
  `Effect` or `SetColour` may see the four block commands.
- R4.5 `make test` still opens no device.

### R5. Experiments, on the desk, with the user watching

- E1 **Save.** `onboard --save` through the receiver. Expect Reaction (12),
  and keep the file for every step that follows.
- E2 **Purple.** `onboard --colour 8000ff --yes`. Typed keys light purple and
  the effect is still Reaction. The read-back matches. Record which case of
  R2.3 ran and the time each write took.
- E3 **Again.** The same command. Nothing is written.
- E4 **Off and on.** The keyboard switched off and on: Reaction, still purple.
- E5 **Asleep.** The command while the keyboard sleeps: `ErrSilent`, nothing
  written.
- E6 **Restore.** `onboard --restore` with E1's file: the colour from before
  E2 is back, and both blocks read back as E1 saved them.
- E7 **The canvas after.** A frame session from spec 019 over the recoloured
  effect, then stop: the effect comes back purple, and both blocks are
  unchanged.

### R6. Documentation

- R6.1 `docs/devices.md` and the F75's support entry describe the capability,
  and say that it writes the keyboard's flash.
- R6.2 `docs/credits.md`: womier-l65-linux's row gains the config block, the
  palette and the write acknowledgement.
- R6.3 `docs/api.md` regenerated, and the changelog entry.

## Acceptance Criteria

- [ ] `lighting.Onboard`, `Effect`, `Colour` and `ErrNoColour` exist as R1
      says, and `Capabilities` reports `"onboard"` for the F75's receiver.
- [ ] The R2.3 choice is tested in each case, and no byte outside the change
      is altered.
- [ ] The packet encoding matches `l65ctl.py`'s byte for byte.
- [ ] On the fake: no write when nothing changes, for a refused effect, or
      for a bad block; a missing acknowledgement stops the write.
- [ ] The guard test passes with the new command lists.
- [ ] `make test` and `make lint` pass, on Linux and Windows.
- [ ] E1 to E7 run on the desk and their results are recorded in
      Verification.
- [ ] Docs per R6.

## Out of scope

- Choosing the effect, its brightness or its speed.
- The side strip, the key matrix and per-key colours (Self-define).
- The cable (R2.10).
- hotaru. Using `Onboard` from `light set` is a hotaru spec.

## Risks & Assumptions

- **Flash wear.** Each change writes one or two sectors of flash. The chip's
  endurance is not known here. R2.3 writes the least it can and nothing when
  nothing changes. The consumer decides how often to ask; the hotaru spec
  states its own limit.
- **A bad write.** A wrong config block can leave the keyboard with odd
  settings. R2.4 keeps every other byte, R2.7 reads back, and the bench saves
  before every write. A save restores with R3.4. The chip's bootloader resets
  to the factory template as a last resort.
- **The L65's layout on the F75.** The offsets come from another keyboard on
  the same chip. Spec 019 found the F75's config block the same through both
  routes. E1 and E2 confirm the offsets before the code is final.
- **A write cut short.** The keyboard falling asleep or going out of range
  part way through. It is not known whether the firmware stores a half-written
  block. R2.6 reports it and the save restores it.
- **The canvas and the effect on one keyboard.** Frames draw over the effect
  while they run. A consumer that does both sees frames until they stop, then
  the effect. E7 checks that frames leave the stored colour alone.
- **Rollback**: revert the PR. A keyboard that was written keeps its colour
  until it is set again or restored from a save.

## Alternatives Considered

- **The canvas, a still frame resent every 500 ms.** Already possible.
  Rejected for this: it replaces the effect, and it is radio traffic on a
  battery for as long as the colour is wanted.
- **The nearest of the factory slots, config block only.** One small write
  and no palette write. Rejected as the only path: seven colours, and purple
  is not one of them. R2.3 still takes this path when the row already holds
  the colour.
- **Always slot 0, as `l65ctl.py colors` does.** Simpler. Rejected: it
  rewrites the config block on every change of an effect that is on another
  slot, where writing that slot needs the palette only.

## Verification

`go vet ./...` and `go test -skip Live ./...` pass on Windows.
`TestLiveAulaReadingsArePlausible` fails on this desk for a reason outside
this spec: with the keyboard charging on its cable and switched to 2.4 GHz,
the receiver reports power state `0x11`, which `Decode` does not know.

The bench, 2026-10-26, Windows 11, the F75 on its receiver and charging on
its cable, the user watching:

- **E1.** `onboard --save`: both blocks whole, 618 bytes in 203 ms. The
  effect read as 7, Ripples shining, on slot 2, `#0000ff`; the user saw blue
  ripples. The per-effect table sits at `0x38 + 2n` as the L65 document says,
  but brightness reads 9 for most effects where the document says 0 to 4.
  Every palette row held the factory colours.
- **E2.** `onboard --colour 8000ff --yes`: a palette-only write, 35 packets,
  each acknowledged; 789 ms with the reads, and the read-back matched. The
  user saw purple ripples, the effect unchanged.
- **E3.** The same again: wrote false, 199 ms, nothing written.
- **E4, failed.** After the keyboard was switched off and on, it ran Neon
  stream, the factory default. Both blocks read as the factory template: 11
  config bytes back to defaults, the user's own colour slots on other effects
  included, and the purple gone from the palette.
- **Config only.** `onboard --restore` with E1's save wrote the config block
  alone (the palette already matched), 10 packets, 573 ms. The user saw blue
  ripples. After the keyboard was switched off and on, they were still blue,
  and both blocks matched E1's save byte for byte.

- **The config block after the palette.** The user's suggestion: re-assert
  the effect after the colour, as `l65ctl.py colors` does. `onboard --colour
  8000ff --yes` wrote the palette (35 packets) and then the config block (10
  packets), 914 ms, and the read-back matched. The user saw purple ripples.
  After off and on, the keyboard ran Neon stream again, and both blocks read
  the same factory template as the first time, byte for byte. A config-only
  restore from E1's save put the user's settings back.

So a config block written through the receiver persists, and a palette
written through the receiver makes the keyboard load its factory template at
the next power-on, whether or not the config block follows it. The keyboard
itself can show purple, and the vendor's program stores custom colours, so
the fault is in this write. One explanation fits the documents: on the cable
the colour table is 512 bytes with `5A A5` at `0x1FA`, and the receiver
carries 490 of them. Not tested.

Decided with the user, 2026-10-26: a power cycle is rare, and the consumer
puts the colour back afterwards, as hotaru already does for the Apex, which
also loses what it was showing. So `SetColour` writes the palette when the
colour needs it, followed by the config block as `l65ctl.py` does, and the
documentation says the colour does not survive a power cycle. A colour
already in the effect's row, stored through the config block alone, does.
The reset also puts the effect back to Neon stream and every other effect's
settings to the factory's, so putting the colour back means putting the
effect back too. That is the consumer's spec.
