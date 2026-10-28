# 021 — room on the screen without the readout

**Issue**: #60

## Status: COMPLETE

## Executive Summary

`place` no longer counts the Kraken's vacant image slots as images at offset
0, so a moving picture alternates between the start of memory and the space
after the image on screen instead of climbing until memory runs out. The
readout clear stays as the last resort. Reviewers should look first at the
`vacant` check in `place` and at the fake's slot memory that the new tests use.

## Context

The Kraken 2024 Elite stores pictures in sixteen slots that share about 24 MB
of memory (`capacity`, 24,320 packets). `send` asks `place` where a new image
may go. When `place` finds no room, `send` calls `clear()`, which switches the
panel to the firmware's liquid readout (`38 01 02`) and empties every slot.

With a moving picture (hotaru's dashboard, a new image every three seconds)
that happens about every 8.5 minutes. The switch is visible, and on the Kraken
it also turns ring LED 22 green until the next lighting frame (hotaru #24,
measured 3 of 3 on 2026-10-12).

### Why the memory fills

`Image` deletes the previous slot once the new one is on screen, so in steady
state one slot holds data: the one on screen. The other fifteen are vacant and
report offset 0, size 0.

`place` counts those vacant slots as images. A vacant slot at offset 0 that is
not the target counts as an overlap, so `place` puts every new image after the
highest one. The images climb through memory until `highest + packets` passes
`capacity`. The "room at the start" rule cannot fire either, because the vacant
slots make `lowest` 0. Then `place` fails and `send` clears through the
readout.

### What was measured

A probe build of hotaru, 2026-10-20 to 2026-10-27, with a patched `send` that
reserved the image at offset 0 where `place` failed, instead of clearing:

- 1,278 wraps to offset 0, every one accepted by the cooler.
- The image on screen sat at offsets 24,206–24,210 each time. Images were
  89–91 packets.
- No readout switch apart from one at shutdown. The ring stayed clean.

## Requirements

**R1. A vacant slot is not an image.** `place` leaves vacant slots out of its
overlap, highest and lowest arithmetic. The target slot's own entry is still
read for its offset and size.

**R2. The readout is the last resort.** `send` still calls `clear()` when
`place` fails with vacant slots ignored. That is memory too full or too
fragmented to fit the image beside what is on screen.

**R3. The slot on screen is never written.** Unchanged, and kept by R1: the
slot on screen is not vacant, so its range still counts as an overlap.

**R4. Cost.** No new commands. `place` reads the slot replies `send` already
asks for.

## Acceptance Criteria

- [x] AC1 (R1): `place`, with only the slot on screen occupied at the top of
  memory and the rest vacant, returns offset 0 (unit test).
- [x] AC2 (R1, R3): `place`, with the slot on screen at offset 0, returns the
  address after it, not 0 (unit test).
- [x] AC3 (R1): the existing `TestPlacement` cases still pass, apart from
  cases that relied on vacant slots counting, which are rewritten with the
  reason stated.
- [x] AC4 (R2): `place` still fails when the image cannot fit before or after
  the slot on screen, and `send` then clears through the readout (unit test).
- [x] AC5: over 2,000 images sent through a fake that keeps slot offsets and
  sizes, `send` never writes `38 01 02` and never writes into the slot on
  screen (unit test).
- [x] AC6 (live): hotaru built against this branch, with the dashboard on the
  Kraken for at least two hours. The screen never leaves the dashboard and
  ring LED 22 stays its scene colour, with somebody watching. (Amended: the
  first wording also asked for the log to show no readout switch, but
  `clear()` logs nothing, so a quiet log is not evidence either way.)
- [x] AC7: `make test` and `make lint` pass; changelog entry under
  `### Unreleased`.

## Risks & Assumptions

- **The device refuses an address it did not choose** (`04`). Offset 0 was
  accepted 1,278 times in the probe run. The address after the top image is
  what `place` returns today. AC6 checks both on the hardware.
- **liquidctl counts vacant slots.** This driver was ported from it. liquidctl
  then clears memory the same way; this spec departs from it on purpose, with
  the measurement above as the reason.
- **Rollback.** Revert the commit; the readout clear comes back.
- **hotaru picks this up through a sanshoku release.** hotaru pins sanshoku
  v0.1.5, 43 commits behind; its bump is part of the hotaru side of #24.

## Alternatives Considered

- Considered the probe's change (offset 0 when `place` fails); rejected
  because it does not check the slot on screen, and an image on screen at
  offset 0 would be written over.
- Considered emptying the non-displayed slots and placing again; rejected
  because they are already vacant in steady state, and the fault is in how
  `place` reads them.

## Verification

Unit tests and lint, 2026-10-28, on c468aaf:

- `make test` passes, including the three new `TestPlacement` cases (AC1,
  AC2, AC4), `TestAMovingPictureNeverLeavesTheScreenForTheReadout` (AC5) and
  `TestNoRoomBesideTheImageOnScreenStillClears` (AC4). No existing
  `TestPlacement` case changed (AC3).
- On the code before the fix, the two new `send` tests fail: the moving
  picture test reports "image 3 climbed to 15".
- `make lint`: 0 issues. `govulncheck ./...`: no vulnerabilities found.
- hotaru (v0.1.28, pinned to sanshoku v0.1.5) builds and passes `make test`
  with a `replace` pointing at this branch.

Live run (AC6), 2026-10-27 23:13:50 to 2026-10-28 01:13:56, Kraken 2024
Elite:

- hotaru `0.1.28-sanshoku021` (v0.1.28 with a `replace` to this branch) ran
  as the user service with the dashboard on the cooler screen, a new image
  every three seconds.
- With the code before the fix, memory filled about every 8.5 minutes, so the
  run would have switched to the readout about 14 times.
- The person watching reported that the screen stayed on the dashboard and
  ring LED 22 kept its scene colour for the whole run.
- The service did not restart (`NRestarts=0`) and logged nothing after
  startup.
