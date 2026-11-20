# 045 — MSI mainboards' Super I/O RGB header

**Issue**: #107

## Status: COMPLETE

## Executive Summary

A new `msi.Board` driver lights the one RGB header of MSI boards driven by
their Super I/O chip, through PawnIO, holding the ISA bus mutex across each
sequence. The B450 Tomahawk Max is tested on the Windows test machine; 49
other boards are expected. Review `msi/board.go` first.

## Context

#107 assumed the Windows test machine's MSI B450 Tomahawk Max used Mystic
Light over USB. It has no MSI USB device: OpenRGB 1.1 drives its one RGB
header through the Super I/O chip, a Nuvoton NCT6795D or NCT6797D, which on
Windows means PawnIO's LpcIO module (spec 044). hotaru 0.1.29 named it
`MSI B450 TOMAHAWK MAX (MS-7C02)`, one LED, Direct only. The protocol is in
`docs/msi-board.md`.

## Requirements

- R1 `msi.Board`, named "msi-board", in `all.Drivers`. Find reads the DMI
  maker and name (the registry on Windows, sysfs on Linux) and is a candidate
  for an MSI board whose name carries one of OpenRGB's 50 MS- numbers, named
  `MSI ` and the DMI name. Nothing is opened.
- R2 Open loads LpcIO and finds an NCT6795D or NCT6797D at `0x2E` or `0x4E`
  by its device ID, reads only. Without PawnIO, `ErrUnsupported`.
- R3 One light, `MSI LED`. The first frame on a handle readies the chip as
  OpenRGB does; each frame writes the colour, four bits a channel, into the
  twelve colour registers. Every sequence holds the ISA bus mutex.
- R4 `Release` and `Close` send nothing. `Floor` 50 ms (the bench run).

## Acceptance Criteria

- [x] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries`, `CGO_ENABLED=0 make build`, and `go vet` for
      windows, linux and darwin pass.
- [x] Tests on a fake chip: found by DMI maker and number; another maker and
      an unlisted board absent; the chip found by its ID, writing nothing;
      another chip and no PawnIO refused; the readying writes and the colour
      registers; an inverted and a plain board's `FF`; release and close.
- [x] Live on the Windows test machine: the bench finds the board and shows
      steady, breathe and wave on its header.

## Out of scope

- Mystic Light over USB (MSI's later boards).
- Linux: the chip is reached through PawnIO only.

## Risks & Assumptions

- **Writes to the Super I/O chip**, which also holds the hardware monitor.
  Only the registers OpenRGB writes, on logical devices `09` and `12`, and
  every sequence under the ISA bus mutex, which OpenRGB does not take.
  LibreHardwareMonitor runs on the test machine (hotaru #197).
- **Rollback:** revert.

## Verification

### Bench run (2026-11-20, the Windows test machine, elevated SSH, hotaru and OpenRGB stopped, the maintainer watching)

- `sanshoku-bench light --keys --driver msi-board`: `MSI B450 TOMAHAWK MAX
  (MS-7C02)`, one key `MSI LED`; the chip found at a port, reads only.
- With LibreHardwareMonitor running: steady red, 100 frames; then breathe and
  wave timed out waiting for the ISA bus mutex ("went red, then off").
- With LHM closed: steady, breathe and wave at 50 ms, 100 frames each, none
  timed out, under 1 ms a frame. The maintainer: all three showed, and the
  header held its last colour with nothing sent.
- hotaru and OpenRGB were started again after.

### LibreHardwareMonitor holds the buses

With LibreHardwareMonitor running, the global mutexes were held for more than
two seconds at a time: the first SMBus run waited for ever (the wait was
then unbounded), and once bounded the board took a few frames and then timed
out on every one. With LHM closed, both ran at full rate. hotaru on Windows
reads the processor's temperature through LHM, so the two have to share; that
is a hotaru question (#197 is likely the same cause under OpenRGB, which takes
no ISA mutex at all).

