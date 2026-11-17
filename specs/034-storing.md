# 034 — Which devices keep their effect in storage that wears

**Issue**: #95

## Status: COMPLETE

## Context

hotaru's spec 077 (ushineko/hotaru#218) puts back a firmware effect it
recorded, after a restart or a power cycle. On the Kraken, the MSI card and
the K4 HE the effect is held in memory and is lost with power, so putting it
back is a write in memory. On the F75 an effect or colour write goes to flash
(spec 023), and a consumer writes it only when a person asks.

`lighting.Onboard` said every onboard device stores its colour, which is true
only of the F75, and `aula.Saver` is the bench's, not a capability. A consumer
could not tell the two apart.

## Requirements

- R1 `lighting.Storing` is an `Onboard` device whose firmware keeps its effect
  and colour in storage that outlives a power cycle and wears with every
  write. It has one method, `StoresEffects()`, which marks the device and does
  nothing.
- R2 The F75 through its receiver is `Storing`. The Kraken, the MSI card and
  the K4 HE are not.
- R3 `Onboard`'s documentation says a write goes to storage only on a
  `Storing` device.

## Acceptance Criteria

- [x] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries` pass.
- [x] The F75's receiver handle is `Storing` (a compile-time assertion), and a
      test in nzxt, msi and keychron finds their devices are not.
- [x] Docs: `docs/design.md` (Lighting), README, the API index.

## Risks & Assumptions

- **A marker, not a behaviour.** Nothing a driver does changes.
- **Rollback:** revert.

## Verification

`make test`, `make lint` (0 issues), `make check-api`, `make check-support` and
`make check-no-binaries` pass. `aula/onboard.go` asserts
`var _ lighting.Storing = (*device)(nil)`; `nzxt`, `msi` and `keychron` each
test that their device is not `Storing`.
