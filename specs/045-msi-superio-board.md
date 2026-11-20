# 045 — MSI mainboards' Super I/O RGB header

**Issue**: #107

## Status: INCOMPLETE

## Executive Summary

(Written last.)

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
- R4 `Release` and `Close` send nothing. `Floor` 50 ms, to be noted on the
  bench.

## Acceptance Criteria

- [ ] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries`, `CGO_ENABLED=0 make build`, and `go vet` for
      windows, linux and darwin pass.
- [ ] Tests on a fake chip: found by DMI maker and number; another maker and
      an unlisted board absent; the chip found by its ID, writing nothing;
      another chip and no PawnIO refused; the readying writes and the colour
      registers; an inverted and a plain board's `FF`; release and close.
- [ ] Live on the Windows test machine: the bench finds the board and shows
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

(Results pasted here.)
