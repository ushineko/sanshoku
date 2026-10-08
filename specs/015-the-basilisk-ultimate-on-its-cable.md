# 015 — the Basilisk Ultimate on its cable

**Issue**: #44

## Status: COMPLETE

## Context

The Basilisk Ultimate is read through its dongle (1532:0088, spec 003). Put on
its cable to charge, the mouse enumerates as itself, 1532:0086, and the driver
finds it, since its control interface declares the vendor page, and reads it.
But the product is in neither the kinds table nor the support table, so the
reading is `KindOther` and the bench marks it "not in the support table". A
consumer that lays its cells out by kind gave it a generic cell beside an empty
mouse slot (hayami#162).

## Requirements

- R1 1532:0086 is `KindMouse` in the Razer driver's kinds.
- R2 A support-table entry: Expected on Linux (OpenRazer lists it as the
  Basilisk Ultimate, wired; not benched there), Tested on Windows from a bench
  reading.
- R3 docs/devices.md regenerated.

## Acceptance Criteria

- [x] The bench reads 1532:0086 as a mouse on Windows: "Basilisk Ultimate 82%
  charging mouse".
- [x] A test holds the Basilisk Ultimate to `KindMouse` on either link; it
  fails with the new kinds entry removed.
- [x] `go test ./...`, vet for linux, darwin and windows, golangci-lint for
  windows and linux, and the generated docs are clean.

## Risks & Assumptions

- Additive: one kinds entry and one support row. Rollback: revert.
- The reading keeps its name, "Basilisk Ultimate", the same name the dongle's
  node gives with its own suffix ("Basilisk Ultimate Dongle"). Giving both
  links one name, or one identity, was considered and left out: a consumer
  shows one mouse by its own rule (hayami spec 050).

## Verification

2026-10-08, Windows 11, the mouse on its cable: `sanshoku-bench read --driver
razer` → "Razer Basilisk Ultimate (1532:0086) … battery: Basilisk Ultimate
82% charging mouse"; before this, the same reading was "other" and "not in the
support table".
