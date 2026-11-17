# 035 — A frame ends the Kraken's effect, and an effect can restart

**Issue**: #97

## Status: COMPLETE

## Context

Found while building hotaru's spec 077 (ushineko/hotaru#218):

- The Kraken's driver kept its record of the running firmware effect after a
  frame was drawn over it. A later `SetEffect` of the same effect was skipped
  as already running, and the ring stayed on the frame. The MSI driver forgets
  its effect on a frame, and the Keychron driver reads its effect from the
  keyboard each time.
- A consumer could not start an effect from its beginning when nothing about
  it changed, which starting several devices' effects in step needs (hotaru
  spec 077 R3.5).

## Requirements

- R1 A frame the Kraken sends forgets the running effect (its colour is kept
  for the next): `Effect` is `ErrUnknownEffect` after it, and the next
  `SetEffect` of the effect is sent.
- R2 `lighting.Settings.Restart` sends the effect even where nothing differs,
  so it starts from its beginning: on the Kraken, the MSI card and the K4 HE.

## Acceptance Criteria

- [x] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries` pass.
- [x] Tests: a frame then the same effect is sent, and `Effect` is unknown in
      between (nzxt); `Restart` sends again with nothing changed (nzxt, msi,
      keychron).

## Risks & Assumptions

- **A restart is a write.** On the Kraken it is one `2A 04` per channel, on
  the card an effect's register writes, on the keyboard one value; a consumer
  restarts when it means to.
- **Rollback:** revert.

## Verification

`make test`, `make lint` (0 issues), `make check-api`, `make check-support` and
`make check-no-binaries` pass, with the tests named above.
