# 027 — The ASUS Aura mainboard as a canvas with zones

**Issue**: #76

## Status: COMPLETE

## Context

The ROG Maximus Z790 Hero's lights (the board's own, and four addressable
headers driving the case fans and strips) are lit through OpenRGB on the desk
this module is written on. Retiring OpenRGB (hotaru #212) needs a native
driver for the Aura USB controller (`0b05:18f3`).

The protocol is in `docs/asus-aura.md`, from OpenRGB 1.1 (credited, not
translated) and the bench. Two things are new to this module:

- **Zones.** The board has its own light and four headers. A consumer
  addresses them as zones, as it did through OpenRGB.
- **Lengths the device does not know.** A header has no way to say how many
  LEDs its strip has. OpenRGB kept the length in its profile; here the
  consumer sets it.

Names follow OpenRGB's ("ASUS ROG MAXIMUS Z790 HERO", "Addressable RGB Header
2", "Addressable RGB Header 2, LED 7"), so what was written against OpenRGB's
listing finds the same lights without a `formerly` rule.

## Requirements

### R1. Zones in `lighting`

- R1.1 `lighting.Zone` (name, first light, count, min and max for a zone the
  consumer sizes) and `lighting.Zoned` (`Zones`, `Resize`). A resizable zone
  starts at its min. `Resize` refuses a zone that is not resizable or not
  there (`lighting.ErrNoZone`) or a length out of range, and changes nothing.

### R2. The driver

- R2.1 An `asus` package and `asus.Driver`, named "asus", in `all.Drivers`. It
  finds hidraw nodes of vendor `0b05` on usage page `0xFF72`, and opens only
  `18f3`; another product is a candidate refused without being opened.
- R2.2 The name is "ASUS " and the DMI board name where Linux has one, else the
  node's.
- R2.3 Open reads the version (`EC 82`) and the configuration table (`EC B0`),
  each by its reply's first byte within 500 ms, lays out the channels from the
  table, and sends `EC 52 53 00 01` as OpenRGB does. A board that does not
  answer is not opened.
- R2.4 Channels: the board's own lights (direct channel `04`), then one per
  addressable header (direct channel *i* − 1), at 0 lights until resized, 0 to
  500. Keys and zones are named as OpenRGB names them. A key is a byte, so all
  channels together are at most 256 lights.

### R3. The canvas

- R3.1 `Frame` merges `px` and, for each channel with lights that changed,
  sends `EC 35 <effect> 00 00 FF` once per handle, then its lights 20 a
  packet, applying on the last. A key not in `Keys` is an error and nothing is
  sent.
- R3.2 `Floor` is 16 ms (E5).
- R3.3 `Release` forgets the frame and sends nothing; `Close` sends nothing.
  No command is known that returns the board to its stored effect.
- R3.4 The board holds a frame with nothing sent (E4). Not `Fading`.

### R4. The bench

- R4.1 `sanshoku-bench light --zone NAME=N` resizes before streaming, and
  `--keys` lists zones.

### R5. Documentation

- R5.1 `docs/asus-aura.md`; credits; udev for `0b05` hidraw; README;
  `devices.md`.

## Experiments, on the desk with someone watching (OpenRGB stopped)

- E1 **Header map.** The board's light magenta, headers 1 to 4 red, green,
  blue, white at 16 lights: which lights are which header?
- E2 **Lengths.** Each header with its 16th light white and the rest dim: does
  the white land on each strip's last LED, and do any LEDs stay unlit?
- E3 **Gen1.** Is anything different without `EC 52 53 00 01`? (Only if the
  board can be brought back to its own state between tries.)
- E4 **Hold.** Nothing sent for some minutes, and the handle closed: does the
  frame stay?
- E5 **Rate.** A breathe on everything at 33 and 16 ms: smooth, or stepped?
- E6 **12 V headers.** Anything plugged into them, and does a longer board
  channel reach them?

## Acceptance Criteria

- [x] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries`, `CGO_ENABLED=0 make build`, and `go vet` for
      windows, linux and darwin pass.
- [x] Tests on a fake controller: open lays out the Hero from its table and
      sends the three opening reports; no answer is not opened; a product off
      the list is never opened; DMI naming; RGB-header naming; resize and its
      refusals; a frame's claim once per channel and its packets; packets
      split at 20 and past 255; release and close send nothing.
- [x] E1, E2, E4 and E5 run, recorded in Verification, and R3.2 and R3.4
      settled. E3 could not be run: see Verification.
- [x] Live: `sanshoku-bench light --yes --driver asus` with the desk's header
      lengths shows steady, breathe and wave across the board and its fans.
- [x] Docs per R5.

## Out of scope

- The firmware effects (`EC 35` modes, `EC 36` colours) and saving to the
  board (`EC 3F 55`): hotaru draws the board (#212).
- Other Aura USB devices: addressable-only controllers, keyboards, mice.

## Risks & Assumptions

- **The Gen1 packet** is sent because OpenRGB sends it; what it does is not
  known.
- **Lengths are the consumer's.** A wrong length lights fewer LEDs, or writes
  to LEDs that are not there, which the board ignores (to be checked, E2).
- **Rollback:** revert; nothing else uses the package. `lighting.Zoned` is
  additive.

## Verification

### Before the spec (2026-11-17)

- `EC 82`: `AULA3-6K75-0309`, in 10.8 ms. `EC B0` in 12.8 ms; the table from
  offset 4: `10 1f 04 01 83 08 0b 3c 00 01 00 00 78 3c 00 01 00 00 78 3c 00 01
  00 00 78 3c 00 01 0d 03 01 f4` and zeros. Headers 4, board lights 1, RGB
  headers 3.
- `sanshoku-bench light --keys --driver asus --zone "Addressable RGB Header
  1=16" --zone "Addressable RGB Header 4=2"`, with OpenRGB running: "ASUS ROG
  MAXIMUS Z790 HERO", 19 keys, the five zones as above, keys named as OpenRGB
  names them. Nothing was drawn.

### The experiments (2026-11-17, OpenRGB stopped, the maintainer watching)

A probe writing raw reports to the controller's node; every channel put in
direct mode first.

- **E1, the header map.** The board's light magenta, headers 1 to 4 red,
  green, blue, white: header 1 is the mirror accent over the VRM, header 2 the
  two 120 mm side fans, header 3 the rear fan. Nothing showed white or
  magenta: header 4 has nothing plugged in, and the board's own light is not
  visible on this case.
- **E2, the lengths.** Header 2: LED 16 at the end of the second fan, nothing
  past it: 16 (two fans of 8). Header 3: LEDs 17 to 20 lit in red, green,
  blue and yellow, nothing from 21: 20. Header 1, the other headers dark
  because the mirror reflects them: 17 to 40 showed light, 21 to 40 and 19 to
  20 did not, 18 alone did: 18. OpenRGB had each header at 16, so the last two
  LEDs of the accent and the last four of the rear fan were never lit.
- **E3, Gen1.** Not run. OpenRGB has sent `52 53 00 01` at every start, and
  nothing short of a power cycle brings the board back to a state where its
  absence could be seen. The driver sends it, as OpenRGB does.
- **E4, the hold.** Red, green and blue on the three headers, then nothing
  for three minutes with the handle closed: unchanged.
- **E5, the rate.** A white breathe on the board's light and the three headers
  at their lengths: 30.3 fps at 33 ms and 62.5 fps at 16 ms, writing a frame
  in 8.3 to 8.4 ms (four packets); smooth at both.
- **E6, the 12 V headers.** Not run: the table says three, OpenRGB never
  lights them on this board, and none is known to have anything on it.

### Live through the driver

`sanshoku-bench light --yes --driver asus --hold 5s` with `--zone
"Addressable RGB Header 1=18"`, `2=16` and `3=20`: 55 keys, floor 20 ms at the
time. Steady red, a red breathe and a wave across the accent and all three
fans; 750 frames, about 8 ms each. Release sent nothing, and the last wave
frame stayed.

