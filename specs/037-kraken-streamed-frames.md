# 037 — The Kraken's panel takes streamed frames

**Issue**: #27

## Status: INCOMPLETE

## Executive Summary

The Kraken Elite's panel gains a second way to show a picture: streamed raw
frames, as NZXT CAM sends them, through a new optional `screen.Streamer`. A
frame shows at once in full colour with no bucket and no floor; the firmware
takes the panel back 30 s after the last one, so the caller resends within
`StreamHold` (25 s). Reviewers: `nzxt/frame.go` and the hold's reasoning
first.

## Context

The Kraken's panel shows a picture today one way: as a GIF uploaded to one of
sixteen buckets, then selected (spec 005). That costs a settling period that
grows with the file (`ImageFloor`: 1 to 3 s), holds the picture to 256
colours, and needs the bucket bookkeeping in `nzxt/screen.go`. hotaru draws
its dashboards that way.

NZXT CAM 4.76.9 draws without buckets (capture of 2026-11-16, on #27). Per
frame, on the HID interface and bulk endpoint 2:

    36 01 00 01 <format>        answered 37 01
    bulk: the 12-byte header, <format> 00 00 00, <payload length LE32>
    bulk: the payload
    36 02                       answered 37 02

A bench probe on 2026-11-18 (Kraken Elite V2, firmware 1.2.12, a person
watching; results on #27) sent format `09`, raw RGB888 at 640 × 640:

- A frame shows at once, in full colour. About 100 ms per frame in all;
  `37 02` after 3 to 20 ms (median 17 ms); about 10 frames a second back to
  back.
- **The firmware returns the panel to its readout 30 s after the last
  frame**, unasked. A frame sent again every 20 s held for 2 minutes with no
  visible change.
- CAM's session start (`36 04`, `36 03`, readout, empty buckets) made no
  difference, so it is not sent.
- The ring and the status readings were unaffected.

The old note in `nzxt/screen.go`, that the RGB565 path flickered and static
images are dropped after five to ten seconds, is this 30 s return seen without
knowing its length.

Format `08` (q565, compressed RGB565, about 20 fps in CAM) is not part of this
spec. Its encoding is not yet written down from a decoded frame, and nothing
needs more than 10 fps today.

## Requirements

- **R1 A frame capability.** `screen` gains an optional interface, so a
  panel without the path does not have to pretend:

  ```go
  type Streamer interface {
      Stream(ctx context.Context, img image.Image) error
      StreamHold() time.Duration
  }
  ```

  - R1.1 `Stream` draws one picture and returns when the device answers
    `36 02`. The device's answer paces a caller; there is no separate floor.
  - R1.2 An image of a different size from `Size()` is scaled to fit (the
    rule `fit` already applies to GIFs), and a non-RGBA image is converted.
    A caller can hand in whatever it drew.
  - R1.3 `StreamHold` is how long a frame stays up with nothing sent after
    it: the measured 30 s, less a margin, stated as the time by which the
    caller must send again (25 s). The driver starts no goroutine and keeps
    no timer (the module's rule); resending is the caller's job.
  - R1.4 `screen.Panel` does not change. `Capabilities` does not gain a
    name: a framer is a kind of screen.
- **R2 The nzxt driver.**
  - R2.1 The Kraken Elite (`3012`) panel is a `Streamer`, sending format `09`, blue first.
  - R2.2 The Kraken 2024 Plus (`3014`) is not, until a bench run on one says
    the format and the hold carry over. Its 240 × 240 size is liquidctl's
    figure.
  - R2.3 After a frame the driver records that no bucket is showing. The
    next `Image` then treats every bucket as free to use, and `Readout` hands
    the panel back as it does now.
  - R2.4 `Close` hands the panel back after frames as after an image, and
    commits the held lighting frame first, as `liquid` does today.
  - R2.5 Every constant carries what it was measured from: format `09`, the
    20-byte header, the 30 s hold.
- **R3 The testbench.**
  - R3.1 `screen --yes` gains `--frame`: it draws the gradient test card
    through `Stream`, holds for `--hold` with a resend every `StreamHold`, and
    hands back. Without `--frame` the verb is unchanged.
  - R3.2 `--animate D` sends moving frames for D and prints the rate and
    each frame's time in all (start to `37 02`).
  - R3.3 The probe's own verb (`stream`) and `StreamOpen` are removed.
- **R4 Docs.**
  - R4.1 `nzxt/screen.go`'s package comment: the two paths, the 30 s return,
    and that format `08` is known and left out.
  - R4.2 `docs/api.md` (via `make check-api`), README "What is in it" if the
    screen row names the operations, changelog under `### Unreleased`.
  - R4.3 `docs/devices.md` via `make generate`: the Elite's screen notes say
    it takes frames.

## Acceptance Criteria

- [x] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries` pass. (`make test` with `-skip TestLive`: the
      live liquidctl comparison fails while hotaru holds the desk's cooler,
      as it did before this branch.)
- [x] Tests against the nzxt fake, extended to answer `36 01` and `36 02`
      and record bulk writes:
  - a frame sends `36 01 00 01 09`, the 20-byte header with the payload's
    length, 1228800 bytes of RGB, then `36 02`, in that order;
  - a 320 × 320 or paletted image arrives as 640 × 640 RGB;
  - a refused `36 01` or `36 02` (result byte not `01`) is the frame's error;
  - after a frame, `Image` may reuse any bucket, and `Readout` and `Close`
    send `38 01 02`;
  - the Kraken 2024 Plus panel is not a `Streamer`.
- [x] Live, a person watching (bench):
  - `screen --yes --frame --hold 2m`: the test card in full colour for the
    whole 2 minutes, no blink at the resends, then the readout;
  - `screen --yes --frame --hold 40s` with the resend turned off by a test
    flag: the readout returns about 30 s after the frame (confirms R1.3 still
    holds);
  - `screen --yes --frame --animate 10s`: smooth motion, rate and frame
    times recorded;
  - `screen --yes` (GIF) straight after a frame run: the test card shows
    (R2.3).

  The ring check first listed here (hotaru drawing on the ring during a
  framed run) moved to the hotaru spec that adopts frames, at the
  maintainer's decision: only one program holds the cooler, so the bench and
  hotaru cannot run together.
- [x] Docs per R4.
- [x] Found on hotaru's spec 080 live run, after the bench run below:
  - [x] format `09` is blue, green, red: sent red first, a light blue
        dashboard showed as yellow on brown. A test pins the byte order.
  - [x] a `Frame` method on the screen hid `lighting.Canvas.Frame`, and the
        Kraken Elite stopped being a canvas, so hotaru could not draw its
        ring. The interface is `Streamer` with `Stream` and `StreamHold`, and
        a test checks both models are still canvases.
- [ ] Live: a light blue picture streamed to the panel shows light blue. (`docs/devices.md` prints only the Windows notes for
      this entry, so the Linux note is in `support` output, not the page.)

## Risks & Assumptions

- **The 30 s return is one firmware's.** Measured on 1.2.12. A later firmware
  could shorten it; a caller that resends at 25 s would see the readout
  flash. The bench's resend-off run is how to check after an update.
- **Cost moves to the caller.** A still picture now costs one 1.2 MB transfer
  every 25 s (about 100 ms of bus time) for as long as it is meant to stay
  up, where a GIF in a bucket cost nothing after its upload. hotaru decides
  where each is worth it, in its own spec, under its cost rule.
- **The readout and the ring.** The firmware re-applies each lighting
  channel's mode 150 to 300 ms after switching to the readout (spec 024).
  `liquid` commits the held frame first for that reason. A return the device
  makes by itself at 30 s gets no commit, so a ring drawn with live frames
  could change. A caller that resends never reaches that point. hotaru's
  spec carries the live ring check. The ring did not change in any bench run
  here, with nothing drawing on it.
- **Two programs.** A frame from another program (CAM under Wine, liquidctl)
  between this driver's frames is not detected, as with buckets today.
- **Windows.** The screen needs usbfs, which Windows lacks; frames do too.
  Nothing changes there.
- **Rollback.** The interface is additive. Reverting the commit removes it;
  a consumer that asserts `screen.Streamer` falls back to `Image`.

## Alternatives Considered

- Considered the driver resending by itself; rejected because driver
  packages start no goroutines, and the caller already owns a ticker.
- Considered a `Frame` method on `screen.Panel`; rejected because it breaks
  every implementer for a path only one model is known to have.
- Considered format `08` first; rejected because its encoding is still
  unknown, and 10 fps of raw frames covers everything a consumer draws today.

## Verification

**After the first PR push (2026-11-18), on hotaru's spec 080 live run.** The
dashboard `ice1` (light text on dark blue) showed yellow text on brown: red and
blue swapped. hotaru also never attached the cooler's ring. A diagnostic on the
desk failed at once: `*nzxt.framePanel is not lighting.Canvas: missing method
Frame`. Both are fixed in the second push.

Bench, 2026-11-18, Kraken Elite V2 on firmware 1.2.12, hotaru stopped, the
maintainer watching the panel.

**Probe runs** (before the code, results on #27): a frame shows at once in
full colour; the readout returns 30 s after the last frame (timed by the
maintainer); a resend every 20 s held 2 minutes with no blink; CAM's session
start made no difference.

**1. `screen --yes --frame --hold 2m`**: the gradient for the whole 2
minutes, no blink at the four resends, then the readout.

```
nzxt  NZXT Kraken Elite V2 (1e71:3012)  panel 640x640, frames held 25s
  frame: ok (90.7 ms); holding 2m0s, resending true
  +25s  sent again (97.9 ms)
  +50s  sent again (86.8 ms)
  +1m15s  sent again (92.7 ms)
  +1m40s  sent again (87.5 ms)
  readout: ok (3.5 ms)
```

**2. `screen --yes --frame --no-resend --hold 40s`**: the readout came back by
itself at about 30 s.

```
nzxt  NZXT Kraken Elite V2 (1e71:3012)  panel 640x640, frames held 25s
  frame: ok (126.4 ms); holding 40s, resending false
  readout: ok (3.6 ms)
```

**3. `screen --yes --frame --animate 10s --hold 3s`**: the bar moved
smoothly.

```
nzxt  NZXT Kraken Elite V2 (1e71:3012)  panel 640x640, frames held 25s
  animated: 100 frames in 10.049s, 10.0 fps; a frame took 82.1 ms at least, 94.7 median, 121.3 at most
  frame: ok (92.7 ms); holding 3s, resending true
  readout: ok (3.6 ms)
```

**4. `screen --yes --frame --hold 3s`, then `screen --yes --hold 5s`**: the
gradient, a moment of readout, the GIF checkerboard for 5 s, the readout.

```
nzxt  NZXT Kraken Elite V2 (1e71:3012)  panel 640x640, frames held 25s
  frame: ok (91.7 ms); holding 3s, resending true
  readout: ok (4.3 ms)
nzxt  NZXT Kraken Elite V2 (1e71:3012)  panel 640x640, test card 6766 bytes, floor 2s
  image: ok (92.9 ms)
  waited 5s
  readout: ok (4.1 ms)
```

Tests: `go test -race -skip TestLive ./...` passes; `make lint` 0 issues;
`make check-api`, `check-support`, `check-no-binaries` pass; `GOOS=windows go
vet` on the changed packages passes; `govulncheck ./...` finds nothing.
