# 044 — PawnIO on Windows, and the SMBus through it

**Issue**: #110

## Status: INCOMPLETE

## Executive Summary

(Written last.)

## Context

Package `i2c` was Linux only, so on Windows no memory or Super I/O device
could be lit. OpenRGB reaches them through PawnIO, a signed kernel driver
installed apart, which runs signed modules and serves Administrator processes
only; the Windows test machine has PawnIO 2.2.0, and OpenRGB's copies of the
modules. #110's comment sets out the options for hotaru's service account;
this spec is the module's half. The maintainer: an installer step for PawnIO
comes later (2026-11-20). The protocol is in `docs/pawnio.md`.

## Requirements

- R1 A `pawnio` package: `Open(module)` loads a module blob from
  `ModuleDir` (default: the program's directory) into a PawnIO executor,
  through `PawnIOLib.dll` from PawnIO's installation, loaded at run time; an
  `Executor` runs a module function by name. Missing PawnIO, a missing
  module, and a refusal for want of Administrator rights are each their own
  error. Off Windows, `Open` is `ErrNoPawnIO`.
- R2 `Lock(name)` takes a named global mutex, as LibreHardwareMonitor, HWiNFO
  and OpenRGB do.
- R3 `SuperIO` over the LpcIO module: the slot for port `0x2E` or `0x4E`, then
  configuration reads and writes, `In` holding
  `Global\Access_ISABUS.HTP.Method` across a whole sequence.
- R4 Package `i2c` on Windows: `Adapters` lists each SMBus the Intel and AMD
  modules reach (the AMD one per port), with the controller's PCI IDs from
  the module; `Open` addresses one. Transfers take Linux's SMBus layout and
  hold `Global\Access_SMBUS.HTP.Method`. The ports and sleep mode are set as
  OpenRGB sets them.
- R5 Nothing here installs PawnIO or ships a module. The bench reads the
  module directory from `SANSHOKU_PAWNIO_MODULES`.

## Acceptance Criteria

- [ ] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries`, `CGO_ENABLED=0 make build`, and `go vet` for
      windows, linux and darwin pass.
- [ ] Tests: the SMBus transfer's packing, its byte and block reads and
      writes, the mutex held; the Super I/O slot, the ISA mutex held across a
      sequence; no PawnIO off Windows.
- [ ] Live on the Windows test machine, as Administrator over SSH, with
      OpenRGB's modules: the AMD SMBus listed with its PCI IDs, and spec 046's
      modules found on it.

## Out of scope

- Installing PawnIO or its modules (a hotaru installer step, later).
- hotaru's service account and PawnIO's Administrator requirement (#110's
  options).
- The Intel Skylake IMC and NCT6793 SMBus modules.

## Risks & Assumptions

- **Writes on the memory bus**, now on Windows, where the kernel does not
  refuse an address its drivers own. The drivers' reads-only identification
  and register allow-lists (specs 042, 046) are what stand between a probe
  and the SPD.
- **Another program on the same bus.** The two mutexes are the convention;
  a program that ignores them can still collide.
- **Rollback:** revert; Windows goes back to no I2C.

## Verification

(Results pasted here.)
