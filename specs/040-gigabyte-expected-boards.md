# 040 — The Fusion 2 boards OpenRGB lists, as expected

**Issue**: #111

## Status: COMPLETE

## Executive Summary

The `gigabyte` driver now also opens the 317 boards OpenRGB 1.1 lists on
product `5702`, each with the layout OpenRGB's table gives it, as Expected.
Nine boards OpenRGB lists twice with different layouts are left out. Review
`gigabyte/expected.go` against OpenRGB's table, and the invariants test in
`gigabyte/expected_test.go`.

## Context

Spec 039 made the driver open a Fusion 2 USB controller only when its board's
layout is in a table, because the controller does not report which lights it
has, and measured one board. The maintainer asked for the boards OpenRGB
lists to be added as Expected (2026-11-19), as spec 033 did for the other
lighting drivers.

OpenRGB 1.1's `GigabyteFusion2USB_Devices.cpp` lists 546 board entries:
346 on product `5702` (327 boards), the rest on `5711`, `8297` and `8950`,
which the driver refuses.

## Requirements

- R1 The driver's table holds every `5702` board OpenRGB lists, other than
  the measured Z790 AORUS MASTER X, with its controller numbers and each
  light's name and index, transcribed as facts. A measured board's layout
  wins over an expected one.
- R2 Left out: a board OpenRGB lists twice for one controller with different
  layouts (a revision the name does not tell apart), and a layout in which
  two zones share a name, two single-colour lights share a register, or two
  headers share a slot. A light OpenRGB lists twice identically is listed
  once.
- R3 A support entry, Expected, for the other boards, naming how many and
  which were left out. `docs/gigabyte-fusion2.md` lists the boards.
- R4 Credits name the table.

## Acceptance Criteria

- [x] A test checks every layout in both tables: names unique, at most one
      header per slot, single-colour lights on distinct registers below 8,
      no board both measured and expected, no left-out board expected.
- [x] An expected board opens with its layout and is named after the board
      and its chip; a left-out board is refused after its report.
- [x] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries` pass.
- [x] Docs per R3 and R4, and the README changelog.

## Out of scope

- Products `5711`, `8297` and `8950`: each needs measuring first (spec 039
  R1.1). `5711` has more headers and a second calibration report.
- OpenRGB's generic fallback layouts for boards it does not list.

## Risks & Assumptions

- **OpenRGB's table may be wrong for a board.** An Expected entry says no one
  has benched it. A wrong layout writes to a register with nothing on it, or
  misses a light; nothing is written that the board keeps (spec 039 R1.5).
- **A header's limit** stays at 256 lights, the IT57xx limit, though many
  layouts in OpenRGB's table say 1024.
- **Rollback:** revert; the measured board is unaffected.

## Verification

The transcription (2026-11-19), by a script outside the module, from OpenRGB
1.1's table at tag `release_1.1`: 76 zones, 84 layouts, 546 board entries; on
`5702`, 346 entries for 327 boards. Every `5702` layout fits the driver: a
header on index 5 (header 1) or on 3 or 6 (header 2), single-colour lights on 0 to
7. Nine boards have two different layouts and are left out. Once identical
duplicate lights are merged, no layout clashes. The result: 317 boards, 326
controllers, 42 distinct layouts.
