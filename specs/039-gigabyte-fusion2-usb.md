# 039 — Gigabyte RGB Fusion 2 USB mainboards as a canvas with zones

**Issue**: #105

## Status: INCOMPLETE

## Executive Summary

(Written last.)

## Context

hotaru 2.0 dropped OpenRGB (hotaru #212) and lights only what sanshoku
drives. A second test machine (Linux) has a Gigabyte Z790 AORUS MASTER X
whose lights OpenRGB drove and sanshoku cannot (hotaru #231).

What that machine shows (2026-11-19, read only):

- Two USB controllers, both `048d:5702` (ITE, "ITE Device"), on separate USB
  paths. Each is one HID collection on vendor page `0xFF89` with two feature
  reports: `5A` (16 bytes) and `CC` (63 bytes). There is no input or output
  report.
- DMI board name `Z790 AORUS MASTER X`, vendor `Gigabyte Technology Co., Ltd.`
- Through OpenRGB 1.1 with hotaru 0.1.29: `Z790 AORUS MASTER X (0x57010100)`
  with 61 LEDs and `(0x57020100)` with 1, both in Direct.

OpenRGB 1.1 (`Controllers/GigabyteRGBFusion2USBController`, GPL, read and
credited, not translated) gives the protocol:

- Everything is a 64-byte feature report with ID `CC`. A question is a
  command write followed by a feature read.
- `CC 60` then a read returns the controller's report: a controller number
  (0 or 1 on a board with two), firmware version, flags, the strip lengths it
  was last given, a product string, a colour order per addressable header,
  and a chip ID.
- **There is no layout on the controller.** OpenRGB finds the board's layout
  in a table by DMI name and controller number. For this board: controller 0
  has two addressable headers (`ARGB_V2_1/ARGB_V2_3`, `ARGB_V2_2`, 0 to 256
  LEDs each) and one single-colour light (`LED_C`); controller 1 has one
  single-colour light (`I/O Cover`).
- Addressable headers take frames: report `CC`, a header byte (`58`, `59`),
  a byte offset and count, and up to 19 lights per packet, in the colour
  order the controller reports. Single-colour lights take a "static effect"
  with one colour, applied by a commit report.
- OpenRGB can also write a colour-order calibration (`CC 33`), save the
  lighting to the board's flash (`CC 5E`, `CC 47`) and scan for Gen2 strips
  (`CC 38` to `CC 3D`). All are opt-in settings in OpenRGB, off by default.

## Requirements

### R1. The driver

- R1.1 A `gigabyte` package and `gigabyte.Driver`, named "gigabyte", in
  `all.Drivers`. It finds hidraw nodes of vendor `048d` on usage page
  `0xFF89`, and opens only `5702`. Other Fusion 2 products (`8297`, `5711`,
  `8950`, and `0414:a100`) are candidates refused without being opened,
  until one is measured.
- R1.2 Open sends `CC 60`, reads the `CC` feature report within 500 ms, and
  decodes it. A controller that does not answer is not opened.
- R1.3 **Known boards only.** The layout comes from a table in the package,
  keyed by DMI board name and controller number. It holds the Z790 AORUS
  MASTER X's two controllers. A controller on a board not in the table is a
  candidate refused with the reason, and is not written to beyond `CC 60`.
- R1.4 Open then sends what OpenRGB sends at start, and nothing else: lamp
  array off (`CC 48 00`) when the report's flag `02` is set, the reset
  (`CC 20` to `CC 27` with zeros, then `CC 28 FF 00`), and beat off
  (`CC 31 00`). E6 records what the reset shows.
- R1.5 **Never sent:** `CC 33` (calibration), `CC 5E` and `CC 47` (flash and
  persistence), `CC 38` to `CC 3D` (Gen2 scan). A test asserts it.
- R1.6 Name: the DMI board name and the chip ID as OpenRGB formats it, `Z790
  AORUS MASTER X (0x57010100)`. hotaru 0.1.29 showed the two controllers by
  exactly these names (E0), and hotaru keys a canvas by its name, so two
  controllers named alike would collide. The chip ID is the chip's model and
  revision, not a serial.

### R2. Zones and lights

- R2.1 The controller is a `lighting.Zoned` canvas. An addressable header is
  a zone starting at 0 lights, resizable 0 to 256. A single-colour light is a
  fixed zone of one light.
- R2.2 Zone and key names follow OpenRGB's (`ARGB_V2_2`, `ARGB_V2_2, LED 4`,
  `LED_C`, `I/O Cover`), so what was written against OpenRGB's names finds
  the same lights.
- R2.3 `Resize` sends the length classes (`CC 34`, 32, 64 or 256 per header)
  and turns the headers' own effect off (`CC 32` with their bits), as OpenRGB
  does.

### R3. The canvas

- R3.1 `Frame` merges `px` and sends only the zones whose lights changed. A
  header: its lights, 19 a packet, in the colour order the controller
  reported (E3). A single-colour light: a static effect in its colour. One
  commit (`CC 28` with the lights' mask) after the single-colour lights. A
  key not in `Keys` is an error and nothing is sent.
- R3.2 `Floor` is measured (E5).
- R3.3 Hold: whether the controller keeps a frame with nothing sent (E4)
  decides whether it is `Fading`.
- R3.4 `Release`: E7 decides. If turning the headers' own effect back on
  returns the board to its stored lighting, `Release` does that; otherwise it
  forgets the frame and sends nothing, as the Aura driver does. `Close` sends
  nothing.

### R4. The bench

- R4.1 `sanshoku-bench light --driver gigabyte` works as for the Aura board,
  `--zone NAME=N` included. E0's report was read by a script outside the
  module, as the Aura experiments' raw reports were.

### R5. Documentation

- R5.1 `docs/gigabyte-fusion2.md` (the protocol, sources marked as in
  `docs/asus-aura.md`), credits, udev for `048d` hidraw, README row and
  changelog, `devices.md` through `Support()`.

## Experiments

On the Linux test machine. E0 runs with OpenRGB still running; the rest with
hotaru and OpenRGB stopped there, and the maintainer watching the board.

- E0 **The report.** `CC 60` and the read, on both controllers: controller
  number, firmware, flags, strip lengths, product string, colour orders,
  chip ID. Do the chip IDs match the names hotaru 0.1.29 shows?
- E1 **The map.** `ARGB_V2_1/ARGB_V2_3` red, `ARGB_V2_2` green, `LED_C`
  blue, `I/O Cover` magenta: which lights are which?
- E2 **Lengths.** Each header with its last light white: how many LEDs does
  each strip have? OpenRGB's 61 on controller 0 is 60 across the headers
  and `LED_C`.
- E3 **Colour order.** Red, green and blue on each header, in the reported
  order: do they show as sent?
- E4 **Hold.** Nothing sent for some minutes, the handle closed: does the
  frame stay?
- E5 **Rate.** A breathe on everything at 33 and 16 ms: smooth, and how long
  does a frame take to write?
- E6 **Opening.** Does the reset at open show (a blink, a blackout)?
- E7 **Release.** The headers' own effect turned back on: does the board
  return to its stored lighting?

## Acceptance Criteria

- [ ] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries`, `CGO_ENABLED=0 make build`, and `go vet` for
      windows, linux and darwin pass.
- [ ] Tests on a fake controller: open decodes a `CC 60` report and lays
      out each of the board's controllers; no answer is not opened; an
      unknown board and an unmeasured product are refused and never written
      beyond what R1.3 allows; the opening reports; the never-sent commands
      (R1.5) across open, resize, frames, release and close; resize and its
      refusals; header packets split at 19 in the reported colour order;
      single-colour lights and the commit; only changed zones sent.
- [ ] E0 to E7 run, recorded in Verification, and R1.6, R3.2, R3.3 and R3.4
      settled.
- [ ] Live: `sanshoku-bench light --yes --driver gigabyte` with the measured
      lengths shows steady, breathe and wave across both controllers.
- [ ] Docs per R5.

## Out of scope

- Firmware effects (Static, Breathing, Wave and the rest): hotaru draws the
  board.
- Other boards and products. A follow-up issue lists what OpenRGB's table
  covers, as #90 did for the other drivers.
- Calibration, flash and Gen2 strips (R1.5).
- Adopting the driver in hotaru, which is hotaru #231's.

## Risks & Assumptions

- **Writes to a board.** The driver writes only to a controller it has
  identified by vendor, product, usage page, board name and controller
  number, and never to its flash (R1.5).
- **The reset at open** is sent because OpenRGB sends it at every start; E6
  records what it does.
- **Lengths are the consumer's**, as on the Aura board. A wrong length lights
  fewer LEDs, or writes past the strip.
- **One board.** The table grows by measurement, not by copying OpenRGB's
  table wholesale.
- **The machine is not the desk.** Experiments need the maintainer at the
  test machine, and hotaru and OpenRGB stopped there and restarted after.
- **Rollback:** revert; nothing else uses the package.

## Alternatives Considered

- Considered a generic layout for unknown boards from the controller's own
  report (its strip lengths and flags); rejected for now, because the report
  does not say which single-colour lights exist, and a guess writes to
  lights that may not be there.
- Considered porting OpenRGB's whole board table; rejected: it is GPL, and
  none of its other boards has been measured here.

## Verification

### Before the spec (2026-11-19, read only)

The board's two controllers on the test machine, through sysfs and
`hotaru light list` (0.1.29, OpenRGB 1.1):

- `048d:5702` twice, `hidraw3` and `hidraw10`, each on page `0xFF89` with
  feature reports `5A` (16 bytes) and `CC` (63 bytes).
- DMI board name `Z790 AORUS MASTER X`.
- OpenRGB's listing: `Z790 AORUS MASTER X (0x57010100)` with 61 LEDs in
  Direct, `(0x57020100)` with 1.

### E0, the report (2026-11-19, OpenRGB running)

`CC 60` and a read of `CC` on each controller, by a script:

| | `hidraw10` | `hidraw3` |
|---|---|---|
| Controller | 0 | 1 |
| Product string | `IT5701-GIGABYTE V7.0.18.1` | `IT5702-Z490XE V1.11.1.0` |
| Firmware | 7.0.18.1 | 1.11.1.0 |
| Flags | `00` | `00` |
| Strip detect | `01` | `07` |
| Length classes | 0, 0, 0 | 0, 0, 0 |
| Colour order, header 1, header 2, board | GRB, GRB, BGR | GRB, none, none |
| Chip ID | `0x57010100` | `0x57020100` |

The chip IDs are the suffixes hotaru 0.1.29 shows, which settles R1.6. No
flag is set, so neither controller has a lamp array or Gen2 strips, and
start sends no `CC 48`. Nothing visible changed.
