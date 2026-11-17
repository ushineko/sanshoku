# 026 — The Corsair MM700 as a canvas

**Issue**: #75

## Status: COMPLETE

## Context

The Corsair MM700 RGB mousepad (`1b1c:1b9b`) is lit through OpenRGB on the
desk this module is written on. Retiring OpenRGB (hotaru #212) needs a native
driver for it. It is the simplest of the five: three lights, wired, one
protocol family (Corsair's "V2" peripherals).

The protocol is in `docs/corsair-v2.md`, from OpenRGB 1.1's Corsair V2 driver
(credited, not translated) and a bench session on 2026-11-17 recorded in
Verification. What the bench settled, before this spec:

- Endpoint `0x22` is refused (status `09`); endpoint `0x01` opens.
- The lights are Left, Right and Logo, in that order.
- **A frame holds.** In software mode the pad kept a frame for 28 minutes with
  nothing sent and the handle closed. OpenRGB's 30-second keepalive is for
  keyboards that revert; the MM700 does not, so it is not `lighting.Fading`.
- **A frame sent in hardware mode is accepted and not shown.** Every status
  reads `00`, and the pad keeps its stored effect.
- A frame is three exchanges, about 17 ms. Smooth at 30 and 58 frames a
  second.

## Requirements

### R1. The driver

- R1.1 A `corsair` package and `corsair.Driver`, named "corsair", in
  `all.Drivers`. It finds hidraw nodes of vendor `1b1c` on usage page
  `0xFF42`, and opens only products on its allow-list: the MM700 (`1b9b`).
  A product not on it is a candidate refused with `sanshoku.ErrUnsupported`
  without being opened.
- R1.2 Open asks the pad its product (property `0x12`) and checks it is the
  allow-listed one, asking up to 8 times 120 ms apart (a freshly enumerated
  device does not answer at once). No answer is `sanshoku.ErrAbsent`.
- R1.3 Reports are 64 bytes with no report ID (`00` prepended on write). A
  reply carries the command at byte 1 and a status at byte 2; a status other
  than `00` is an error naming it.

### R2. The canvas

- R2.1 `Keys`: `Left`, `Right`, `Logo`, ids 0 to 2.
- R2.2 `Frame` merges `px` into the held frame and, where the frame changed,
  sends it: open endpoint `0x01`, one block write of 9 bytes (all red, then
  all green, then all blue), close. Each step waits for its reply. A key not
  in `Keys` is an error and nothing is sent.
- R2.3 **The mode is read before a frame is sent** (property `0x03`), and the
  pad is put in software mode (`0x03` = `02`) when it reads hardware. A frame
  sent in hardware mode would be accepted and not shown.
- R2.4 `Floor` is 20 ms.
- R2.5 `Release` puts the pad in hardware mode (`0x03` = `01`), where it shows
  its stored effect, and forgets the held frame. `Close` sends nothing: the
  pad keeps the last frame (and software mode) until it is replugged.
- R2.6 Not `lighting.Fading`.

### R3. Support and documentation

- R3.1 A support entry (Tested, Linux; Windows Expected) and `Describe`.
- R3.2 `docs/corsair-v2.md`, the protocol, with sources. `docs/credits.md`:
  OpenRGB's Corsair V2 driver. `docs/devices.md` regenerated. udev:
  `packaging/60-sanshoku.rules` grants `1b1c` hidraw. README changelog.

## Acceptance Criteria

- [x] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries`, `CGO_ENABLED=0 make build`, and `go vet` for
      windows, linux and darwin pass.
- [x] Tests on a fake pad: the product check with retries; a frame's three
      exchanges byte for byte (planar, 9 bytes); an unchanged frame sends
      nothing; an unknown key sends nothing; the mode read and the claim when
      it reads hardware, and no claim when it reads software; a status other
      than `00` is an error; release sets hardware mode; close sends nothing.
- [x] Live, with someone watching: `sanshoku-bench light --yes --driver
      corsair` shows steady, breathe and wave on the pad, starting from
      hardware mode; release returns it to its stored effect.
- [x] Docs per R3.

## Out of scope

- The MM700's firmware effects (hotaru draws it; maintainer's decision on
  #212), its brightness setting, and saving to it.
- Other Corsair V2 devices and the wireless receivers (write ID `09`).

## Risks & Assumptions

- **One pad.** Measured on one MM700, firmware property `0x13` = `00050001`.
- **OpenRGB also drives it** while its server runs: two writers. hotaru's
  adoption switches OpenRGB off for it, as for the Kraken.
- **Rollback:** revert; nothing else uses the package.

## Verification

### The bench before the spec (2026-11-17, OpenRGB stopped)

A probe writing raw reports to `/dev/hidraw10` (interface 1, page `0xFF42`):

- Properties: `03` (mode) = 1, hardware, as OpenRGB's exit left it; `11` =
  `1b1c`; `12` = `1b9b`; `13` = `00050001`; `5F` (asked by OpenRGB at claim)
  answered status `05`, unsupported. Each reply in 0.7 to 2.0 ms.
- Endpoints: open `22` answered status `09`; open `01` answered `00`.
- Software mode, then one frame red, green, blue: Left red, Right green, Logo
  blue. 16.1 ms for the frame.
- Nothing sent for 28 minutes, the handle closed: still red, green, blue.
- A breathe of all three in white: 30.3 fps at 33 ms (mean 16.9 ms a frame)
  and 58.4 fps at 16 ms (mean 17.0 ms), both smooth to the maintainer.
- Hardware mode (read back as `01`): the pad showed a steady blue, its stored
  effect. A red frame then, every status `00`: still blue.

### Tests

`corsair/driver_test.go` on a fake pad that answers as the bench showed: the
product asked until answered (three silent, then answered), never answered is
`ErrAbsent` and the node closed, another product is `ErrUnsupported`, and a
product off the allow-list is never opened; a frame from hardware mode reads
the mode, claims software mode, opens endpoint `01`, writes one 9-byte planar
block and closes; from software mode there is no claim; an unchanged frame
sends nothing; an unknown key sends nothing; a refused step is an error and
the frame is sent again; release sets hardware mode; close sends nothing.

### Live (2026-11-17, the maintainer watching)

`sanshoku-bench` from this branch, OpenRGB stopped. Its stop had ended in a
kill, so it never handed the pad back: the pad read software mode, showing
hotaru's colours. The probe set hardware mode, and the pad showed its steady
blue.

- `light --keys --driver corsair`: Left, Right, Logo; floor 20 ms.
- `light --yes --driver corsair --hold 5s`: steady red from the blue (the
  driver claimed software mode), then a red breathe, then a wave across all
  three. 749 frames, all acknowledged; a frame in about 14 to 15 ms (max
  39.8 ms). Release in 3.0 ms: the steady blue again.

