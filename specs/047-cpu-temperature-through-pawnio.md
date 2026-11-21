# 047 — The processor's temperature through PawnIO

**Issue**: #116

## Status: COMPLETE

## Executive Summary

A new `cpu` package reads the processor's temperature on Windows directly
through PawnIO: an AMD Zen's Tctl (with k10temp's offsets for Tdie) and an
Intel package temperature, labelled as LibreHardwareMonitor labels them, so
hayami can drop LibreHardwareMonitor. Tested on the Windows test machine's
Ryzen 5 2600X with LHM not running. Review `cpu/cpu.go` (the decoding) and
`cpu/reader.go` first.

## Context

hayami reads the processor's temperature on Windows from LibreHardwareMonitor's
web server, its only use of LHM. LHM also holds the SMBus and ISA bus mutexes
for seconds, which stalls the lighting drivers (specs 044, 045). The
maintainer: reading the sensors directly so hayami no longer needs LHM
(2026-11-20). PawnIO is reachable since spec 044; LHM itself reads the
processor through the same PawnIO modules. The facts are in `docs/pawnio.md`.

## Requirements

- R1 A `cpu` package: `Open` loads AMDFamily17 for an AMD processor and
  IntelMSR for an Intel one, as Windows names the maker; a `Reader` keeps the
  module loaded and `Temperature` reads it; `Close` unloads it. Off Windows,
  `Open` is `pawnio.ErrNoPawnIO`: hwmon reads the processor there.
- R2 AMD: SMN `0x59800` under `Global\Access_PCI`, decoded as k10temp and LHM
  decode it; Tdie labelled `Core (Tdie)` where the model has an offset, else
  the one value labelled `Core (Tctl/Tdie)`.
- R3 Intel: TjMax (`1A2`) less the package readout (`1B1`), labelled `CPU
  Package`; a readout not marked valid is no reading.
- R4 The bench's `host` prints the direct reading beside LHM's.
- R5 No module is shipped; they come with PawnIO (a later installer step).

## Acceptance Criteria

- [x] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries`, `CGO_ENABLED=0 make build`, `go vet` and
      golangci-lint for windows pass.
- [x] Tests: the Zen register's steps and range flag; the offsets and labels
      (2700X with, 2600X without, a Threadripper); the Intel package and its
      valid bit; an AMD read under the PCI mutex; an Intel read; a refused
      read; no PawnIO off Windows.
- [x] Live on the Windows test machine: the processor's temperature read with
      LibreHardwareMonitor not running.

## Out of scope

- hayami's switch from LHM to this package (a hayami spec).
- Per-CCD and per-core temperatures, fan speeds and voltages from the Super
  I/O chip: not what hayami reads.
- Installing the modules.

## Risks & Assumptions

- **Two programs reading the same register** (LHM and this) is harmless;
  both take the PCI mutex.
- **A processor newer than the offset table** reads its Tctl as Tdie, as
  k10temp and LHM do.
- **Rollback:** revert; nothing uses the package yet.

## Verification

### Bench run (2026-11-20, the Windows test machine, elevated SSH)

PawnIO.Modules 0.2.12's signed release (`release_0_2_12.zip`, SHA-256
`09698897…f9d`), its `AMDFamily17.bin` and `IntelMSR.bin` beside the bench.
LibreHardwareMonitor not running.

`sanshoku-bench host --repeat 5 --interval 2s`: `cpu 44.8 °C Core
(Tctl/Tdie), through PawnIO (AMD Ryzen 5 2600X Six-Core Processor)`, then
41.9, 50.8, 47.6 and 44.6 °C, each read in well under a millisecond; LHM's
line reported its web server not answering, as it was not running. The 2600X
has no k10temp offset, so the label is the one LHM gives it. A side-by-side
with LHM was offered and skipped by the maintainer.
