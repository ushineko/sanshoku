# 041 — EVGA Ampere graphics cards over I2C

**Issue**: #109

## Status: COMPLETE

## Executive Summary

A new `evga` driver lights EVGA's Ampere graphics cards over I2C: each zone
one colour, as a canvas, with release putting back what the card showed. The
RTX 3080 FTW3 Ultra LHR is tested on the Linux test machine; 59 other cards
are expected. `i2c.Bus` gains reads and the SMBus block write, a breaking
change. Review `evga/canvas.go` (the claim and the release) and the `i2c`
additions first.

## Context

hotaru 2.0 dropped OpenRGB, and the Linux test machine's EVGA GeForce RTX 3080
FTW3 Ultra LHR (PCI `10de:2216`, subsystem `3842:4897`) went dark (hotaru
#231). OpenRGB lit four LEDs on it. The maintainer asked for the driver to be
built from OpenRGB's protocol and checked on the machine in one run, and for
the other products OpenRGB lists to come in as Expected (2026-11-20).

The protocol is in `docs/evga-gpu.md`. Package `i2c` could only write; this
controller is read at open.

## Requirements

### R1. The driver

- R1.1 An `evga` package and `evga.Driver`, named "evga", in `all.Drivers`.
  It finds NVIDIA adapter 1 on a card whose four PCI IDs are on its
  allow-list: the 60 cards OpenRGB 1.1 lists for this controller, at address
  `0x2D`. Nothing is opened to find.
- R1.2 Open reads the firmware (`B1`, 6 bytes), the modes register (`C0`, 10
  bytes) and each lit zone's static register. A card that does not answer
  in full is not opened. Open writes nothing.
- R1.3 Names: the card as OpenRGB names it (`EVGA GeForce RTX 3080 FTW3 Ultra
  LHR`), the name hotaru 0.1.29 showed.
- R1.4 The save register (`90`) is never written; the write path refuses it.

### R2. The canvas

- R2.1 One key per zone with LEDs, named as OpenRGB names it.
- R2.2 Before the first frame on a handle: the unlock (`B2`), then every zone
  static, LED counts kept, zones in step (`C0`), as OpenRGB's Direct mode.
- R2.3 A frame writes the static register of each zone that changed: `04`,
  full brightness, red, green, blue.
- R2.4 `Release` writes back the static registers and the modes register read
  at open, after the unlock, so a card running its own effect runs it again.
  A handle that drew nothing sends nothing. `Close` sends nothing.
- R2.5 `Floor` is 50 ms (the bench run).

### R3. Package i2c

- R3.1 `i2c.Bus` gains `ReadByteData`, `ReadBlockData` (I2C block read) and
  `WriteSMBusBlock` (SMBus block write, which spec 042 needs). A breaking
  change to the interface, named in the changelog.

### R4. Documentation

- R4.1 `docs/evga-gpu.md`, credits, README row and changelog, `devices.md`
  through `Support()`.

## Acceptance Criteria

- [x] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries`, `CGO_ENABLED=0 make build`, and `go vet` for
      windows, linux and darwin pass.
- [x] Tests on a fake controller: find by the four IDs on adapter 1 only;
      open reads and writes nothing; a card that does not answer is not
      opened; zones without LEDs are not keys; the first frame claims the card
      and writes each zone, the next only what changed; release puts back
      what was read; the save register is never written.
- [x] Live on the test machine: `sanshoku-bench light --yes --driver evga`
      shows steady, breathe and wave on the card's zones; the floor is
      measured; release puts back what the card showed.
- [x] Docs per R4.

## Out of scope

- The card's own effects (breathing, rainbow and the rest) as
  `lighting.Onboard`: hotaru draws the card. A follow-up if drawing at the
  card's floor proves too slow.
- Resizing the addressable header.
- Windows (#110).

## Risks & Assumptions

- **The zones in step.** OpenRGB writes the sync byte as 1 in Direct mode;
  the bench showed every zone taking its frame with it.
- **Writes to a graphics card.** Only to the address OpenRGB uses, on a card
  identified by four PCI IDs, and never the save register.
- **Rollback:** revert; nothing else uses the package. The `i2c.Bus` change
  reverts with it.

## Verification

### Bench run (2026-11-20, the Linux test machine, hotaru and OpenRGB stopped, the maintainer watching)

- `sanshoku-bench light --keys --driver evga`: `EVGA GeForce RTX 3080 FTW3
  Ultra LHR (3842:4897)` on `/dev/i2c-10`, four keys: Front Logo, End plate
  Logo, Back Logo, Addressable Header. Opening read the firmware, the modes
  register and four static registers, and wrote nothing.
- `sanshoku-bench light --yes --driver evga --hold 5s`: steady, breathe and
  wave at 50 ms, 100 frames each, none timed out; a frame wrote in 11.2 to
  14.0 ms at the median, 21.1 ms at most. The maintainer: all three showed,
  "then back" to the card's earlier look on release (15.1 ms). The zones in
  step (sync 1) still each took their colour.
- hotaru and OpenRGB were restarted after; hotaru 0.1.29 found the card
  again.
