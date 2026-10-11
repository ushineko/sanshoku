# 018 — the machine itself, on Windows

**Issue**: #50

## Status: COMPLETE

## Context

hayami runs on Windows and reads the graphics card and the processor's
temperature there with code of its own: D3DKMT and the "GPU Engine" counters
for the card (hayami spec 034), and LibreHardwareMonitor's web server for the
processor (hayami specs 036 and 042). It lives in `hayami/internal/core`, at
`6a2754f`: `graphics_windows.go`, `engines.go`, `lhm.go` and
`lhm_windows.go`.

hotaru is being ported to Windows and its dashboard wants the same three
figures. It cannot import hayami's `internal` package, so the choice is a
second copy or one shared one. This module exists so that there is one.

`docs/design.md` kept host readings out: "`/proc/stat`, `/proc/meminfo`,
`/proc/net/dev` and DRM busy counters (not devices, and the consumer reads
them in a few lines)". That reasoning holds on Linux and does not carry to
Windows. The card is 300 lines of struct layouts taken from a kernel header,
with a counter set whose instances have to be reduced the way Task Manager
reduces them. The processor is a JSON tree whose values are locale-formatted
text, matched by label because its identifiers renumber. Neither is a few
lines, and each constant in them was a measurement.

What was measured, on the desk hayami's specs were written at (Ryzen 5 2600X,
RTX 3060 Ti, Windows 11) and recorded there:

- `D3DKMTQueryAdapterInfo` with `KMTQAITYPE_ADAPTERPERFDATA` reads the
  temperature in tenths of a degree, 43.2 °C where `nvidia-smi` said 43. Two
  adapters are listed: the card and the Basic Render Driver.
- Two collections of the engine counter a few milliseconds apart read a busy
  card as 0 %. 200 ms apart, the figure agreed with Task Manager's.
- LibreHardwareMonitor 0.9.6 has no WMI namespace; `data.json` is the route.
  Its `SensorId` is not unique within a tree.
- Its web server listens on every interface whatever address it is given, and
  accepts requests that set a fan's speed.

## Requirements

- R1 A package `gpu`. `Reader` (zero value ready, safe for concurrent use)
  with `Read(ctx) (Card, error)` and `Close() error`. `Card` carries the name,
  the temperature and the load, each figure with whether it was read.
  `ErrNoCard` wraps `sanshoku.ErrAbsent`. Off Windows `Read` returns an error
  wrapping `errors.ErrUnsupported`, with the same exported identifiers.
- R2 The first `Read` takes two collections `Warmup` (200 ms) apart. No two
  collections are closer than `Warmup`; a `Read` that comes sooner waits out
  the rest under its context. A context that is already over returns its
  error without waiting.
- R3 `Close` releases the PDH query, which hayami's reader never did. A closed
  `Reader` reads again.
- R4 A package `lhm`, building everywhere: `Node`, `Text`, `ParseValue`,
  `CPULabels`, `CPUTemperature`, `Fetch`, `Read`, `URL`, `Timeout`, and the
  sentinels `ErrUnreachable`, `ErrAuth`, `ErrNoSensor`, `ErrTooLarge`.
- R5 `lhm.Fetch` makes one kind of request, a GET of the address it is given.
  It sends no credentials. An answer over 8 MiB is refused, on its declared
  length where it declares one.
- R6 `lhm.Host` and `lhm.ProbeHost()`: whether PawnIO is installed,
  LibreHardwareMonitor is running, and its startup task is registered. Asked
  read-only and without administrator rights. Off Windows, all false.
- R7 What stays in the consumer: the words a person reads. hayami's `Absence`
  codes and its advice ("Options > Remote Web Server > Run") are an
  application's; this module returns the sentinel and the facts.
- R8 No subprocess. `nvidia-smi` stays the consumer's fallback.
- R9 `sanshoku-bench host` reads both sources, with `--lhm`, `--repeat` and
  `--interval`. A source that is not there is said and exits 0.
- R10 Tests carried from hayami, against this module's names: the tree and
  label order, either decimal mark, a number where text was expected, the
  HTTP failures told apart, the size cap by header and by body, the engine
  reduction. New: a cancelled read returns at once; a closed reader reads
  again; `ErrNoCard` is absent.
- R11 `docs/design.md` (what belongs here, transports), `.claude/CLAUDE.md`
  (the platform rule, what the module opens, the network rule), README (two
  rows, the testbench, the platform paragraph, changelog), regenerated
  `docs/api.md`.

## Acceptance Criteria

- [x] `go test ./...` passes on Windows 11.
- [x] `go vet ./...` passes for `GOOS=windows`, `linux` and `darwin`.
- [x] The pinned golangci-lint (v2.12.2, Go 1.26.0) reports 0 issues on
  Windows and with `GOOS=linux`.
- [x] `docs/api.md` is regenerated from the code.
- [x] `sanshoku-bench host` reads the RTX 3060 Ti and the Ryzen's temperature
  on Windows, and both temperatures agree with another reader of the same
  source.
- [x] With LibreHardwareMonitor's address wrong, the bench says the server is
  not answering and what the machine has, and exits 0.
- [x] `make test`, `make lint`, `make check-api` and `make check-support` pass
  on Linux (CI).

## Out of scope

- **Adopting it.** hayami deleting its copy, and hotaru calling this, are
  specs in those repositories, as every migration has been.
- **Processor load and memory.** `GetSystemTimes` and `GlobalMemoryStatusEx`
  are one call each. They are the Windows counterpart of `/proc/stat` and
  `/proc/meminfo`, and stay with the consumer for the reason those do.
- **A driver and a support entry.** Neither source is a device a `Driver`
  finds, so neither is in `all.Drivers()`, `scan` or `docs/devices.md`. The
  bench's `host` verb is their oracle.
- **A card chosen by the caller.** `Read` takes the first hardware adapter
  that reports a temperature, as hayami did. A machine with two cards that
  matter is a later spec.

## Risks & Assumptions

- **The network rule is narrowed, not dropped.** "No network access in any
  library package" becomes: none except `lhm`, which makes a GET to an address
  its caller gives and defaults to loopback. The address is not restricted to
  loopback, because hayami's setting is not (its spec 036: "a setting that
  points elsewhere is the user's"). A consumer that wants loopback only passes
  `lhm.URL`. The alternative, parsing here and fetching in each consumer,
  keeps the rule whole and leaves two copies of the size cap, the status
  handling and the timeout.
- **The struct layouts are from `d3dkmthk.h`** and are checked only by reading
  a real card. `D3DKMT_ADAPTER_PERFDATA` is WDDM 2.4 and later; an older
  driver answers no temperature and the card is still named.
- **`data.json` is undocumented.** Its format changed between releases, which
  is why values are read as text or number and matched by label.
- **Additive.** No existing package changes. Rollback: revert; consumers pin a
  module version.

## Verification

2026-10-10, Windows 11 Pro 26200, Go 1.27.0 (lint under Go 1.26.0), Ryzen 5
2600X, RTX 3060 Ti, LibreHardwareMonitor running with PawnIO:

- `go test ./...`: every package ok.
- `go vet ./...` for windows, linux, darwin: clean.
- golangci-lint v2.12.2: 0 issues on Windows and with `GOOS=linux`.
- `sanshoku-bench host --repeat 3 --interval 2s`:

  ```
  gpu  NVIDIA GeForce RTX 3060 Ti  43.4 °C  5 %  (462.1 ms)
  cpu  50.4 °C  Core (Tctl/Tdie), LibreHardwareMonitor  (10.8 ms)
  gpu  NVIDIA GeForce RTX 3060 Ti  43.8 °C  1 %  (1.0 ms)
  cpu  47.8 °C  Core (Tctl/Tdie), LibreHardwareMonitor  (6.0 ms)
  gpu  NVIDIA GeForce RTX 3060 Ti  43.5 °C  2 %  (1.0 ms)
  cpu  45.9 °C  Core (Tctl/Tdie), LibreHardwareMonitor  (7.2 ms)
  ```

  The first card reading carries the warm-up. After it a reading costs about
  1 ms for the card and 6 to 11 ms for the processor.
- Cross-checked in one run: the bench read the card at 43.4 °C where
  `nvidia-smi` said 43, and the processor at 49.0 °C where the tree's own
  `Value` said "49.0 °C". The load was not compared like for like: the bench
  read 3 % and `nvidia-smi` 24 % a moment later, over different windows and by
  different definitions (the busiest engine against NVIDIA's own figure).
- `sanshoku-bench host --lhm http://127.0.0.1:9/data.json`: the card as
  above, then "cpu  none: LibreHardwareMonitor's web server is not answering",
  "PawnIO installed: true, LibreHardwareMonitor running: true, startup task:
  true", exit 0.
- CI on PR #51, Linux: `go test -race ./...`, `make lint`, `make build`,
  `make check-support` and `make check-api` pass. Its Windows job passes
  `go test ./...` too, on a runner with no card.
