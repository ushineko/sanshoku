# 046 — The original Corsair Vengeance RGB

**Issue**: #108

## Status: COMPLETE

## Executive Summary

`corsairdram` learns the original Corsair Vengeance RGB: found by reads only,
one light, five register writes a colour, at a 200 ms floor. Two modules are
tested on the Windows test machine through PawnIO. Review the family probe
in `corsairdram/driver.go` first.

## Context

#108's Windows half: two Corsair Vengeance RGB DDR4 modules at `0x5A` and
`0x5B` on the Windows test machine's AMD B450, which OpenRGB 1.1 drives with
its original Vengeance controller, not the one spec 042 follows. hotaru 0.1.29
named them `Corsair Vengeance RGB (0x5A)` and `(0x5B)`, one light each. The
SMBus is reached through PawnIO (spec 044). The protocol is in
`docs/corsair-dram.md`.

## Requirements

- R1 `corsairdram` probes for a second family: a module answering `BA` for
  every register `A0` to `AF`, at the same addresses, reads only, after the
  newer family's two registers say it is not one of those.
- R2 Named `Corsair Vengeance RGB (0x5A)`; one light, `Corsair LED`, in
  `Corsair Zone`.
- R3 A frame: `A4` = 0, `B0`–`B2`, `A6` = 0. The family writes only those
  five registers; the newer family's allow-list is its own.
- R4 Release and close send nothing. `Floor` 200 ms (the bench run).

## Acceptance Criteria

- [x] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries`, `CGO_ENABLED=0 make build`, and `go vet` for
      windows, linux and darwin pass.
- [x] Tests: found by reads only, named and laid out as OpenRGB; a frame's
      five writes; an unchanged frame nothing; each family's allow-list.
- [x] Live on the Windows test machine: both modules found and shown steady,
      breathe and wave.

## Risks & Assumptions

- **A third probe.** On a machine with other things at those addresses, Find
  reads sixteen more registers there, once per process.
- **Rollback:** revert.

## Verification

### Bench run (2026-11-20, the Windows test machine, elevated SSH, hotaru and OpenRGB stopped, LibreHardwareMonitor closed, the maintainer watching)

- `sanshoku-bench light --yes --driver corsairdram --hold 5s` found both
  modules, `0x5A` and `0x5B`, on `pawnio:SmbusPIIX4.bin:0`, one key each.
- Steady, breathe and wave on each, none timed out; a colour took about
  157 ms (five byte writes), so a frame every 50 ms became one every 157.
  `Floor` for this family is 200 ms. The maintainer: both showed all three,
  and both held their last colour with nothing sent.
- With LHM running, no module was found: the SMBus mutex was held.

