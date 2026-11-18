# 038 — The Kraken Elite's panel takes Q565 frames

**Issue**: #103

## Status: COMPLETE

## Executive Summary

The Kraken Elite's panel takes Q565 frames through a new `screen.Mover`,
beside `Streamer`: format `08`, seritools' public QOI-for-RGB565 codec, at the
device's 20 frames a second and a fraction of a raw frame's bytes. Raw 24-bit
frames stay for still pictures. The encoder is written from the published
description and leaves out the optional `DIFF_INDEXED` op, ten times faster on
noise for 3.7% more bytes. Reviewers: `internal/q565/q565.go`, then `Move` in
`nzxt/frame.go`.

## Context

Spec 037 gave the Kraken Elite's panel streamed frames through
`screen.Streamer`: format `09`, raw pixels blue first, 1228800 bytes a frame,
about 10 frames a second. NZXT CAM also streams in format `08`, at about 20 a
second, which 037 left out because its encoding was unknown.

Format `08` is **Q565**: QOI adapted to 16-bit RGB565, designed by seritools,
with a reference implementation at
[seritools/q565](https://github.com/seritools/q565) (MIT or Apache-2.0). The
evidence is on #103: the header and opcodes match; CAM's 6616-byte frame is an
all-black Q565 image to the byte; every captured opening decodes.

A probe on this branch (commit "a Q565 probe") encodes Q565 in Go from the
published description. Its output matches, pixel for pixel, a decoder
transcribed from the reference's `decode.rs` and `ops.rs`. On the desk
(2026-11-18, Kraken Elite V2, firmware 1.2.12, the maintainer watching):

| | result |
|---|---|
| colour card (red, green, blue bands) | shown in that order: standard RGB565, red in the high bits |
| card | 6621 bytes, 65 to 75 ms |
| moving gradient, 10 s | 198 frames, 19.7 a second, smooth |
| per frame | about 180 KB; 44 ms to send (the device's pace), 1.3 ms to encode |
| nothing sent after the card | the readout returns at about 30 s, as with `09` |

Encoding, measured on the probe (`BenchmarkEncode`, i9-14900K): 1.1 ms for a
flat field or a smooth gradient, 1.3 ms for repeated colours, 48 ms for pure
noise. Noise is the worst case, and the time goes on the colour-array search
for `DIFF_INDEXED`, which the reference marks as optional.

**Colour depth is the trade.** Q565 carries 65536 colours; format `09` carries
24-bit colour at half the rate and about ten times the bytes. A still picture
where every colour counts is better as `09`; anything that moves is better as
Q565. So the two stay side by side, and the caller chooses.

## Requirements

- **R1 A Q565 capability.** `screen` gains an optional interface beside
  `Streamer`:

  ```go
  type Mover interface {
      Move(ctx context.Context, img image.Image) error
  }
  ```

  - R1.1 `Move` draws one picture as a Q565 frame, scaled to the panel as
    `Stream` scales, and returns when the device answers `36 02`. The answer
    paces a caller drawing frame after frame.
  - R1.2 A Mover is also a Streamer. `StreamHold` applies to both: a frame of
    either format stays up until the hold has passed with nothing newer.
  - R1.3 No method name of `Mover` is a method name of `lighting.Canvas` or
    its optional interfaces (`Frame`, `Floor`, `Keys`, `Release`, ...), and a
    test checks that the Kraken Elite is still a canvas. Spec 037's `Frame`
    hid the canvas's.
  - R1.4 `screen.Panel` and `screen.Streamer` do not change.
- **R2 The encoder.** `internal/q565`, from the probe.
  - R2.1 Encodes any `image.Image`; an `*image.RGBA` is read from its bytes.
  - R2.2 Round-trips exactly for flat, gradient, repeated-colour and noise
    pictures, against a decoder in the tests.
  - R2.3 An all-black 640x640 frame is 6616 bytes, as CAM's.
  - R2.4 640x640 encodes in at most 25 ms for the worst case in the
    benchmark (noise), so a frame never takes longer to encode than the
    device takes to show one. If the colour-array search is what exceeds it,
    the encoder leaves that op out or bounds the search, and the benchmark
    records the size cost.
  - R2.5 Channel order is standard RGB565, red in the high bits, as measured.
- **R3 The nzxt driver.**
  - R3.1 The Kraken Elite's panel is a Mover, sending format `08`.
  - R3.2 Frames of `08` and `09` may follow each other in any order; each is
    one complete frame.
  - R3.3 After a Q565 frame no bucket is showing, as after a raw one.
  - R3.4 The probe's `StreamEncoded` goes; `send` stays unexported.
- **R4 The testbench.** `screen --yes --frame --q565` draws the card and the
  animation as Q565; `--frame` alone stays raw. The probe's `q565` verb goes,
  and the colour card (bands) moves into `screen`.
- **R5 Docs.**
  - R5.1 `docs/credits.md`: seritools/q565, MIT or Apache-2.0, the format and
    its description; the encoder is written from the description.
  - R5.2 `nzxt/frame.go`'s comments: format `08`, the colour-depth trade.
  - R5.3 `docs/api.md` via `make generate`, the README's screen row and
    changelog, the support note for the Kraken Elite.

## Acceptance Criteria

- [x] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries` pass (`-skip TestLive` while hotaru holds the
      desk's cooler).
- [x] Encoder tests: round trips (R2.2); the black frame's 6616 bytes (R2.3);
      the benchmark run and its numbers recorded in Verification (R2.4).
- [x] Driver tests against the nzxt fake: a Move sends `36 01 00 01 08`, the
      20-byte header with format `08` and the payload's length, a payload
      starting `q565` and decoding to the picture, then `36 02`; a refusal is
      an error; `08` and `09` frames alternate; the Kraken Elite is a Mover,
      a Streamer and a `lighting.Canvas`; the Kraken 2024 Plus is none of the
      first two.
- [x] Live, the maintainer watching:
  - [x] `screen --yes --frame --q565`: the card's bands read red, green, blue;
  - [x] `--animate 10s`: smooth, at least 18 frames a second, the rate and
        sizes recorded;
  - [x] with `--no-resend`, the readout returns at about 30 s;
  - [x] a raw frame after Q565 frames and the other way round both show.
- [x] Docs per R5.

## Risks & Assumptions

- **65536 colours.** Gradients band more in Q565 than in `09`. Callers choose
  per picture; dithering before encoding is a caller's option, not this
  spec's.
- **One firmware.** Measured on 1.2.12, as spec 037.
- **Encode time scales with content.** Video sits between a gradient and
  noise. R2.4 bounds the worst case; the device's 44 ms per frame bounds the
  rate in any case.
- **The format's owner.** Q565 is seritools' public format, which NZXT CAM
  appears to use unchanged. A change on NZXT's side would show as a garbled
  panel, and the bench's card shows it.
- **Windows.** The panel needs usbfs, as in spec 037; nothing changes there.
- **Rollback.** Additive: reverting removes `Mover`, and a caller falls back
  to `Stream`.

## Alternatives Considered

- Considered replacing format `09` with Q565 behind `Stream`; rejected
  because a still picture would lose 24-bit colour for a rate it does not
  need.
- Considered building the reference's Rust decoder to check the encoder;
  the build was refused by the session's permission check, so the encoder was
  checked against a decoder transcribed by hand from the reference's
  `decode.rs` and `ops.rs`, run outside the repository (Verification). The
  tests use a Go decoder written from the description.
- Considered keeping `DIFF_INDEXED` in the encoder; left out because the
  search over 64 entries made noise take 55 ms, longer than the device takes
  to show a frame, for 3.7% fewer bytes. The reference marks it optional.
- Considered a public `q565` package; kept internal because no caller needs
  to encode Q565 itself. It can move out when one does.

## Verification

Desk, 2026-11-18: Kraken Elite V2 on firmware 1.2.12, hotaru stopped, the
maintainer watching.

**Encoder.** `go test ./internal/q565 -run - -bench Encode -benchmem`,
i9-14900K, 640x640:

```
BenchmarkEncode/black-30       821043 ns/op   106496 B/op    1 allocs/op
BenchmarkEncode/gradient-30   1038037 ns/op   106496 B/op    1 allocs/op
BenchmarkEncode/noise-30      5946444 ns/op  6365217 B/op   12 allocs/op
BenchmarkEncode/stripes-30    1224138 ns/op   909312 B/op    5 allocs/op
```

Sizes: black 6616 (CAM's), gradient 40971, stripes 273170, noise 1187242
bytes. With `DIFF_INDEXED` searched: noise 54.6 ms and 1144874 bytes; the
other sizes the same or one byte smaller.

**Against the reference's reading.** The four pictures, encoded, decoded by a
Python transcription of `decode.rs` and `ops.rs` (including `sum_n`'s wrap)
and compared with the expected RGB565 pixels:

```
black.q565: 640x640, 409600 pixels, consumed 6616/6616 bytes, MATCH
gradient.q565: 640x640, 409600 pixels, consumed 40971/40971 bytes, MATCH
noise.q565: 640x640, 409600 pixels, consumed 1187242/1187242 bytes, MATCH
stripes.q565: 640x640, 409600 pixels, consumed 273170/273170 bytes, MATCH
```

**Checks.** `go test -race -skip TestLive ./...`, `make lint` (0 issues),
`make check-api`, `make check-support`, `make check-no-binaries`,
`GOOS=windows go vet` on the changed packages, and `govulncheck ./...` (no
vulnerabilities) pass.

**Live.**

1. `screen --yes --frame --q565 --hold 10s`: the bands read red, green, blue.

   ```
   nzxt  NZXT Kraken Elite V2 (1e71:3012)  panel 640x640, frames held 25s, motion frames true
     frame: ok (41.5 ms); holding 10s, resending true
     readout: ok (4.1 ms)
   ```

2. `--animate 10s --no-resend --hold 40s`: smooth; the readout came back by
   itself at about 30 s.

   ```
   animated: 197 frames in 10.022s, 19.7 fps; a frame took 41.1 ms at least, 45.5 median, 73.7 at most
   frame: ok (44.0 ms); holding 40s, resending false
   readout: ok (4.0 ms)
   ```

3. Raw purple, then Q565 bands, then raw yellow, 5 s each, on one handle (a
   short program outside the repository, since each bench run hands the
   panel back): all three shown in order, then the readout.

**The probe before the spec** (on #103): the card in RGB565 order, 6621
bytes in 65 to 75 ms; a moving gradient at 19.7 frames a second, about
180 KB a frame, 44 ms to send and 1.3 ms to encode.
