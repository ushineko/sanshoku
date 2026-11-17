# 028 — The Keychron K4 HE as a canvas

**Issue**: #77

## Status: COMPLETE

## Context

The Keychron K4 HE (`3434:0e40`) is lit through OpenRGB on the desk this
module is written on, where its direct writes looked "hue-only". Retiring
OpenRGB (hotaru #212) needs a native driver. The protocol is in
`docs/keychron.md`, from OpenRGB 1.1 (credited, not translated) and a bench
session on 2026-11-17 (Verification). What the bench settled first:

- **The per-key effect is 23**, as OpenRGB has it.
- **The firmware takes each key's hue and saturation, and ignores its V.**
  Keys sent at V 255, at V 80 and black all showed the same brightness, and
  the black keys white (saturation 0). That is OpenRGB's "hue-only".
- **Brightness is one value for the board** (VIA `07 03 01`). A sweep of it
  breathed smoothly at 30 steps a second and went fully dark.
- A frame of 100 keys is 12 request and reply packets, about 62 ms. A hue
  wave flat out ran 16 fps, very smooth.

The maintainer chose (2026-11-17): each key gets its hue and saturation, and
the board's brightness follows the frame's brightest key.

## Requirements

### R1. The driver

- R1.1 A `keychron` package and `keychron.Driver`, named "keychron", in
  `all.Drivers`. It finds hidraw nodes of vendor `3434` on usage page
  `0xFF60`, and opens only the K4 HE (`0e40`), named "Keychron K4 HE" as
  OpenRGB names it; another product is a candidate refused without being
  opened.
- R1.2 Reports are 32 bytes after a report ID of 0. A reply echoes the command
  (and the subcommand, for `A8`), within 1 s.
- R1.3 Open asks the VIA protocol (`01`), the Keychron features (`A2`), which
  must have Keychron RGB (bit 7), and the LED count (`A8 05`). A keyboard that
  does not answer, or lacks Keychron RGB, is not opened.

### R2. The canvas

- R2.1 Keys: one per LED, ids 0 to count − 1, in the firmware's LED order, as
  OpenRGB orders them. Names are not read (OpenRGB names them from the
  keymap); they are empty.
- R2.2 `lighting.Zoned`: one zone, "Keyboard", of every key, not resizable, as
  OpenRGB names it.
- R2.3 `Frame` merges `px`. Before sending, it reads the running effect
  (`08 03 02`); if it is not 23, the driver records it and the brightness the
  first time, and sets effect 23 (`07 03 02 17`) and per-key solid
  (`A8 08 00`). The keyboard's own keys can change the effect at any time.
- R2.4 Each key goes as its hue (0 to 255 for 0° to 360°) and saturation. Only
  the 9-key packets (`A8 0A <start> <count>` + H S V) whose hue or saturation
  changed are sent; the V byte carries the key's value, which the firmware
  ignores. The board's brightness (`07 03 01`) is set to the brightest key's
  value when it changes.
- R2.5 A key not in `Keys` is an error and nothing is sent.
- R2.6 `Floor` is 70 ms. Not `lighting.Fading`.
- R2.7 `Release` puts back the effect and brightness recorded at the claim,
  and forgets the frame. `Close` sends nothing.

### R3. Documentation

- R3.1 `docs/keychron.md`; credits; udev for `3434` hidraw; README;
  `devices.md`.

## Acceptance Criteria

- [x] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries`, `CGO_ENABLED=0 make build`, and `go vet` for
      windows, linux and darwin pass.
- [x] Tests on a fake keyboard: open and its refusals; the claim and what it
      records; no claim when effect 23 runs; only changed packets sent;
      brightness from the brightest key, sent only when it changes; the HSV
      bytes of known colours; release restores the effect and brightness;
      close sends nothing; an unknown key sends nothing.
- [x] Live, with someone watching: `sanshoku-bench light --yes --driver
      keychron` shows steady, breathe (through the board's brightness) and
      wave, from the keyboard's own effect; release puts the effect back.
- [x] Docs per R3.

## Out of scope

- The 22 VIA effects as firmware modes (hotaru draws the keyboard; #212), and
  saving (`A8 02`).
- Key names from the keymap.
- Other Keychron boards: the per-key effect's number depends on how many
  effects a firmware has.

## Risks & Assumptions

- **A dark key cannot be drawn among lit ones.** A key the frame makes black
  shows white at the board's brightness. A consumer's scene that leaves keys
  off will show them white.
- **Effect 23** is right for this firmware (v1.1.1, 2025-06-17). Another
  firmware may number it differently; the claim reads it back.
- **Rollback:** revert; nothing else uses the package.

## Verification

### The bench before the spec (2026-11-17)

Read-only, OpenRGB running: VIA protocol `0x000C`, Keychron protocol 2,
firmware `v1.1.1 2025-06-17-10:38:42`, features `0x8F`, RGB protocol 1, 100
LEDs, effect 20, brightness 200, speed 127, colour H `3B` S `86`; LED indices
by row for six rows of 21 columns (posted on #77).

With OpenRGB stopped, the maintainer watching:

- Effect 23 and per-key solid, then keys 0–8 red, 9–17 green, the rest blue:
  exactly that. 100 keys in 57 ms.
- Keys 0–8 red at V 255, 9–17 red at V 80, the rest black: all one brightness,
  the black keys white.
- Brightness 40: dark. 100: dim. A sweep 0 to 255 and back at 33 ms: smooth,
  and dark at the bottom.
- A hue wave across the keys, each frame as soon as the last: 161 frames in
  10 s (16.1 fps), 62.2 ms a frame, very smooth.
- Put back: effect 20, brightness 200 (reads 199), speed 127, colour H `3B`
  S `86`.

### Tests

`keychron/driver_test.go` on a fake keyboard that answers as the bench
showed: open asks VIA, the features and the LED count, and refuses a keyboard
without Keychron RGB or that does not answer; a product off the list is never
opened; the first frame records effect 20 and brightness 200, claims effect
23 and solid, sends 12 packets and the brightness of the brightest key; then
only the packet whose hue or saturation changed, and a dimmer frame of the
same hues only the brightness; an effect changed at the keyboard is taken
back, keeping what was found first; release puts back effect 20 and 200;
close sends nothing; an unknown key sends nothing; HSV of red, green, blue,
orange, grey and black.

### Live (2026-11-17, OpenRGB stopped, the maintainer watching)

`sanshoku-bench light --yes --driver keychron --hold 6s`, from the keyboard's
own Solid Reactive Multinexus: steady red on every key, the board breathing
red down to dark, a rainbow wave across the keys, and after release the
reactive effect again. 258 frames, all answered: steady 5.6 ms a frame (only
the look at the effect), breathe 10.0 ms (the brightness), wave 65.8 ms
(every key); release 10.7 ms. A run before it, which the maintainer missed,
took the same times.

