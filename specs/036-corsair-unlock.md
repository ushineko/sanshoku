# 036 — The MM700's lighting endpoint is never left locked

**Issue**: #99

## Status: COMPLETE

## Context

Found on hotaru's spec 077 live run (ushineko/hotaru#218), 2026-11-17. The
MM700 takes a frame as three commands: open the lighting endpoint
(`08 0D 00 01`), the colours, close it (`08 05 01 00`). hotaru sends one
about 18 times a second.

Twice the pad refused every open with status `03` until the endpoint was
closed by hand, and stayed on its last frame across hotaru restarts:

- at 15:46, with a game under Wine holding the pad's nodes;
- at 18:25, after a hotaru restart, with nothing else holding them.

Both followed a restart. A frame whose context is cancelled after the open,
as a stopping service's is, returns before the close, and the endpoint stays
open. Closing it by hand (`08 05 01 00`, answered `00`) and opening again
(`00`) drew at once.

## Requirements

- R1 Once an open has been answered, the frame closes the endpoint whatever
  happens after it, with a short time of its own if the frame's context has
  ended.
- R2 An open refused with status `03` (in use) is followed by a close and
  one more open. A second refusal is the frame's error.
- R3 `Close` closes the endpoint before the node, best effort, and says
  nothing if it was not open.
- R4 The corsair page says that another program holding the pad's lighting,
  or a stop between an open and a close, leaves it refusing, and that the
  driver closes and opens again.

## Acceptance Criteria

- [x] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries` pass.
- [x] Tests, against the fake extended to keep the endpoint's state as the
      pad did (an open while open is status `03`, a close ends it):
  - a frame cancelled after the open, or while its colours are on their
    way, leaves the endpoint closed;
  - an endpoint left open: the next frame closes, opens and draws;
  - an endpoint that refuses twice: the frame fails, and the next succeeds
    once it is free;
  - `Close` closes an open endpoint.
- [x] Live: with hotaru drawing on the MM700, the service restarted ten
      times, and the pad drawing after each.
- [x] Docs per R4.

## Risks & Assumptions

- **Status `03` is "in use".** Seen only when the endpoint was left open, and
  cleared by a close each time. Another cause of `03` gets one close and one
  open more, which cost two exchanges of about a millisecond.
- **A close from a program not holding the endpoint.** It closed what another
  program left open, on the desk. A program actively drawing on the pad
  alongside hotaru would have its endpoint closed under it; two programs
  drawing one pad already fight, and this does not make that worse.
- **Rollback:** revert.

## Verification

### Tests

`make test`, `make lint`, `make check-api`, `make check-support` and
`make check-no-binaries` pass. The fake keeps the endpoint's state: an open
while it is open is `03`, a close ends it, and a read on an ended context
fails, as a real one does. Each part of the change was taken out in turn, and
a test failed each time:

- without the close and second open, `TestAnEndpointLeftOpenIsClosedAndOpenedAgain`
  and `TestAnEndpointThatStaysRefusedIsAnErrorUntilItIsFree`;
- without the close after a failed block, `TestAFrameCutOffInItsColoursStillCloses`;
- without the close in `Close`, `TestCloseShutsAnEndpointAFrameCouldNot`.

Writing the cut-off test found a case the first change missed: a frame cut
off after its open was written and before the answer was read. The pad may
have opened, so an open that fails other than by a refusal is closed too.

### The desk (2026-11-17)

hotaru built against this branch, drawing Rainbow Wave on the MM700, and the
service restarted ten times: after each the pad was drawing, its frames
counting at about 18 a second, and no frame was refused. Before the change the
lock followed two restarts of about a dozen that day, so ten clean restarts
are good evidence, not proof.
