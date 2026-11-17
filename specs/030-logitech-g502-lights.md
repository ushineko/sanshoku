# 030 — The G502 X PLUS's lights over HID++

**Issue**: #79

## Status: COMPLETE

## Context

The Logitech G502 X PLUS, through its Lightspeed receiver (`046d:c547`), is
lit through OpenRGB on the desk this module is written on: the last device
there that OpenRGB drives (hotaru #212). This module's `logitech` driver
already reads its battery over HID++ 2.0 and has, until now, written nothing.

The protocol is in `docs/logitech-lights.md`, from OpenRGB 1.1's HID++ 2.0
driver (credited, not translated; much of it OpenRGB took from captures of the
vendor's program) and a read-only probe of the mouse on 2026-11-17:

- The mouse is device index 1 behind the receiver's `0xFF00` node. Its
  features: `0x8071` (RGB effects) at 9, `0x8081` (per-key) at 10, `0x8100`
  (onboard profiles) at 11; no `0x8101`.
- `0x8081`'s zone bitmap names 8 zones (bits 1 to 8).
- `0x8071` has one cluster of 4 effects, and its effect card's template is
  `20 64`, as OpenRGB's capture says.

## Requirements

### R1. Transport

- R1.1 The HID++ request gains a long form: report `11`, 16 parameter bytes.
  A reply is matched as before, by device index, feature index, function and
  software ID, in either report length.
- R1.2 HID++ 2.0 error `08` (busy) is told apart from other errors.
- R1.3 A HID++ 2.0 error is read from its code byte (the sixth) and matched to
  its request by feature index and function byte. It had been read from the
  function byte, and any device's error taken for the request's.

### R2. The canvas

- R2.1 The node's device is a `lighting.Canvas` and `lighting.Zoned` for the
  first device behind it, by index, that has `0x8081`. Found on first use and
  kept for the handle. A node with none has no keys.
- R2.2 Keys: one per zone of `0x8081`'s bitmap (pages 0 to 2, bit 0 of page 0
  skipped), in zone order, named "LED A" on, as OpenRGB names the G502 X's.
  One zone, "Mouse LEDs".
- R2.3 **Claim**, before the first frame on a handle: `0x8071` SetSWControl
  `01 03 06`, SetRgbPowerMode `01 01`, power configuration `01 00 00 00 3C 01
  2C` (60 s idle, 300 s sleep), and host mode (`0x8100` function 1 `02`, or
  `0x8101` function 6 `05`).
- R2.4 **Prep**, once after the claim, where the device has an effect card:
  SetEffectByIndex (long) cluster `FF` effect 2 with the card's template in
  parameter bytes 8 and 9, persist 1; then cluster `FF` effect *n*, persist 1,
  where *n* is cluster 0's effect count. OpenRGB's capture says per-key does
  not engage without it; what it means is not known.
- R2.5 **Frame**: where the held frame changed, `0x8081` SetIndividual (long),
  four `zone R G B` a report, each acknowledged; then FrameEnd (long, zeros).
  A FrameEnd answered busy is sent again 10 ms later, up to 250 ms. After the
  first FrameEnd, SetSWControl `01 03 05`.
- R2.6 A frame that fails (the mouse asleep or gone) leaves the canvas
  unclaimed, so the next frame claims again.
- R2.7 `Release`: SetSWControl `01 00 00`, then onboard mode (`0x8100`
  function 1 `01`, or `0x8101` function 6 `03`); the frame is forgotten.
  `Close` sends nothing.
- R2.8 `Floor` is 50 ms (E2). Not `lighting.Fading` (E3). A lighting request waits up to 1 s: handing the engine back was answered in 370 to 510 ms.

### R3. Documentation

- R3.1 `docs/logitech-lights.md`; credits; README; `devices.md`; the support
  entry gains `lighting`.

## Experiments, on the desk (OpenRGB stopped, someone watching)

- E1 **Claim and a frame.** The eight LEDs in eight colours: which LED is
  where?
- E2 **Rate.** A breathe at 200, 100 and 50 ms: smooth, stepped, or stalled?
  The busy replies counted.
- E3 **Hold.** One frame, then nothing: does it stay, and what does the mouse
  do when it sleeps and wakes?
- E4 **Release.** Back to the onboard effect?
- E5 **Without the prep.** Does a frame show without it? (Only if E1 works.)

## Acceptance Criteria

- [x] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries`, `CGO_ENABLED=0 make build`, and `go vet` for
      windows, linux and darwin pass.
- [x] Tests on a fake receiver: a long request's bytes; the lit device found
      by its feature; keys from a zone bitmap; the claim and prep in order with
      the template; a frame's packets and FrameEnd; FrameEnd busy then
      answered; a failed frame claims again; release; no `0x8081`, no keys;
      the battery path unchanged.
- [x] E2 to E4 run, recorded in Verification, and R2.8 settled. E1's map and
      E5 were not run: see Verification.
- [x] Live: `sanshoku-bench light --yes --driver logitech` shows steady,
      breathe and wave on the mouse; release returns its onboard effect.
- [x] Docs per R3.

## Out of scope

- The mouse's firmware effects, onboard profiles and DPI.
- A second lit device behind the same receiver.
- The canvas's name: it is the receiver node's ("Logitech USB Receiver"). A
  consumer that knew the mouse as OpenRGB's "G502 X PLUS" maps the name
  (hotaru's `formerly`).

## Risks & Assumptions

- **The prep is not understood.** It is sent because the vendor's program
  sends it, as captured by OpenRGB's authors; it writes with persist set.
- **Battery.** A wireless mouse spends battery on a stream; the bench notes
  the rate, and a consumer decides how much to send.
- **Rollback:** revert. The battery path is untouched.

## Verification

### The probe before the spec (2026-11-17, OpenRGB running, nothing written)

Requests from software ID `0C` on the receiver's `0xFF00` node, the mouse
woken first: index 1 answered (17.3 ms); indices 2 to 6 answered HID++ 1.0
error `08`. Feature indices: `8071` 9, `8081` 10, `8100` 11, `8101` 0, `4522`
0, `1004` 6. `8081` GetInfo page 0: `00 00 ff 01 00…`; pages 1 and 2 empty.
`8071` device info: 1 cluster; cluster 0: 4 effects; effect card (`FF 00 01 00
01`): bytes 10 and 11 `20 64`.

### Tests

`logitech/lighting_test.go` on the fake receiver, extended with the G502 X
PLUS's lighting as the probe read it: a long request's 20 bytes; keys and the
zone from the bitmap, the card's template and the effect count; no lit device,
no keys; the first frame's claim, prep (template in place, kept), two reports
and FrameEnd, then `01 03 05`; an unchanged frame nothing, a changed one only
the reports and FrameEnd; FrameEnd busy twice then answered; a silent mouse
fails the frame and the next claims again; release; an unknown key; the
battery still read; a 2.0 error read from its code and another client's
skipped. The package's existing battery tests pass unchanged.

### Bench and live (2026-11-17, OpenRGB stopped, the maintainer watching)

- `light --keys --driver logitech`: 8 keys, "LED A" to "LED H", zone "Mouse
  LEDs".
- **E1.** A steady red frame showed on the mouse; the first frame, claim and
  prep included, 90 ms. The eight LEDs' positions were not mapped.
- **E2.** A red breathe at 200, 100 and 50 ms, and at 50 ms again for 9 s:
  every frame acknowledged, about 24 ms a frame, nothing busy; smooth at 50 ms.
- **E3.** One blue frame, then nothing for 75 s untouched: blue the whole time,
  past the 60 s idle timer. A green frame sent when the mouse had been left
  alone after a release: all 10 frames unanswered; moved, it showed a rainbow;
  the next frame then took it and showed green.
- **E4.** Release: the mouse's onboard lighting, red, each time. Handing the
  engine back took 371 ms on the probe, and the first release through the
  driver took 1.1 s because its 300 ms wait ran out; with a 1 s wait, 13 to
  508 ms.
- **E5.** Not run: OpenRGB has sent the prep, with keep set, on every claim of
  this mouse, so its absence cannot be seen here.
- Live: `light --yes --driver logitech --hold 5s`: steady red, a red breathe
  and a rainbow wave on the mouse, 300 frames all acknowledged (about 24 ms
  each), then the onboard red after release.

