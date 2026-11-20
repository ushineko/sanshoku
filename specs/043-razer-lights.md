# 043 — Razer lighting by custom frames

**Issue**: #106

## Status: INCOMPLETE

## Executive Summary

(Written last.)

## Context

The `razer` package reads batteries only. hotaru 2.0 dropped OpenRGB, and the
Linux test machine's Razer Mouse Dock Pro (`1532:00a4`, 9 LEDs) and the
Basilisk V3 Pro behind it (13 LEDs) went dark (hotaru #231); the Windows test
machine's Basilisk Ultimate and its dock with them. hotaru 0.1.29 named them
`Razer Mouse Dock Pro` and `Razer Basilisk V3 Pro (Wireless) (Razer Mouse
Dock Pro)`. The maintainer asked for the driver to be built from OpenRGB's
protocol and checked on the machine in one run, with the other products
OpenRGB lists as Expected (2026-11-20). The protocol is in
`docs/razer-lights.md`.

## Requirements

### R1. The driver

- R1.1 `razer.Lights`, named "razer-lights", in `all.Drivers`, beside the
  battery driver, which does not change. It finds the same control nodes
  (vendor `1532`, page `FF00` or `FF01`), and is a candidate for a node whose
  product is in its table.
- R1.2 Each such node is asked, once per process, which devices are paired to
  it (`00:BF`, a getter); each paired device in the table is a candidate of
  its own on the same node, named `<device> (<receiver>)` as OpenRGB names
  it, on its own transaction ID.
- R1.3 Names, zones and LED names follow OpenRGB's.
- R1.4 Only six reports are built: `00:BF`, and the custom frame and custom
  mode for each matrix type. Nothing that stores a setting.
- R1.5 A report through a receiver polls for its answer, as OpenRGB does.
- R1.6 The table: every device OpenRGB 1.1 lights through the standard,
  extended or linear matrix reports, as facts. Left out, and listed: laptops
  (some need a keep-alive), devices needing another report ID or software
  mode first, devices on OpenRGB's custom and ARGB paths, entries whose zones
  do not fill their matrix, and a row too long for a report. Keyboards' keys
  are named by position, as OpenRGB names LEDs without a layout.

### R2. The canvas

- R2.1 A frame that changed is sent whole: each row, then the custom mode.
  An unchanged frame sends nothing.
- R2.2 `Floor` and the hold are measured on the bench.
- R2.3 `Release` forgets the frame and sends nothing; `Close` sends nothing.

### R3. Documentation

- R3.1 `docs/razer-lights.md`, credits, README row and changelog,
  `devices.md` through `LightsSupport()`.

## Acceptance Criteria

- [ ] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries`, `CGO_ENABLED=0 make build`, and `go vet` for
      windows, linux and darwin pass.
- [ ] Tests on a fake node: the dock and the mouse it lists are candidates,
      named as OpenRGB names them, the dock asked once; only products in the
      table; the mouse's keys and zones; an extended frame and its mode on
      the dock's and the mouse's transaction IDs; a standard frame; a linear
      frame declared `32`; only the six reports built; release and close;
      every device in the table fits the reports.
- [ ] Live on the test machine: `sanshoku-bench light --yes --driver
      razer-lights` finds the dock and the mouse behind it and shows steady,
      breathe and wave on both; the floor and the hold are noted.
- [ ] Docs per R3.

## Out of scope

- Effects, brightness and storing to the device.
- The Kraken headsets, the ARGB controller, laptops and the devices R1.6
  leaves out.
- Keyboard legends for key names.

## Risks & Assumptions

- **Contention.** hayami reads the mouse's battery through the dock's node
  on the same transaction ID; the dock answers busy to one of them while the
  other is relayed. The lighting driver polls for its own answer and moves
  on; a battery poll that meets busy is silence, as it is today.
- **A sleeping mouse** is still listed as paired (O); frames to it go
  unanswered until it wakes.
- **Rollback:** revert; the battery driver is untouched.

## Verification

(Results pasted here.)
