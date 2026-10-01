# 010 — Apex lighting by direct frames

**Issue**: #29

## Status: COMPLETE

## Context

The Apex Pro TKL Wireless Gen 3 has no lighting-effect command. That is a
measured fact, not an absence of documentation. On 2026-09-30, with OpenRGB
and hotaru stopped and the board showing the firmware's own rainbow sweep,
every bare command from 0x00 to 0x7F was sent on the cable (1038:1646) and
from 0x00 to 0xFF through the receiver (1038:1644) while the user watched;
none changed the effect. The only writes that did anything were 0x01 and
0x41, which reboot the keyboard, and the OLED commands, which blanked the
display. The 0x20/0xA0 record found by the getter sweep carries the 60-second
idle timer and two colours and is RAM only; it is not the effect.

Then the keyboard was passed to a Windows VM and SteelSeries GG was captured
from the host with usbmon while the user cycled GG's effects and presets and
turned the brightness down. GG sent one thing: 2065 feature reports in 116
seconds, `61 <n> (key r g b)×n`, 85 keys each, 17.8 a second, every one
acknowledged by a `61` input report on the vendor endpoint in 15 ms (median;
25 ms worst). Nothing else went to the keyboard after GG's startup burst. The
frame content follows the user's steps: one colour, then one colour breathing,
then sixty colours a frame, then the same at a fifth of the level when the
brightness slider moved. GG renders every effect on the host and streams it.
The brightness is a multiply.

So on this board, a lighting mode is a renderer and a frame stream. OpenRGB's
Direct mode is the stream with no renderer; its Onboard mode is the firmware's
one fallback effect, which GG does not customise from its Illumination tab;
and OpenRGB's exit sends 0x41, which reboots the keyboard and moves its hidraw
node. hotaru wants the Apex to breathe or ripple like its Keychron does, and
the Keychron does it in firmware; here hotaru has to draw it. This spec gives
this module the frame path, through the handle the `steelseries` driver already
holds for the battery, and a bench command that proves it on the hardware. The
effects themselves are hotaru's (a follow-up spec there); this module ships the
canvas and three demonstration patterns in the bench, which is the test policy's
oracle.

Sources, in order of weight: the usbmon capture (`apex-gg.pcap`, kept with the
session that made it; its command list is in the issue), OpenRGB's
`SteelSeriesApexController.cpp` (direct id 0x61 for 1644 and 1646, 0x4B init,
0x41 "onboard", key table), SignalRGB's `Steelseries_Keyboard_Controller.js`
(0x21 for 1646 on the cable, 0x61 for 1644; no init), Aurora PR #298 (key 0x00
as a whole-board broadcast, on the full-size 1640). GG sent no 0x4B before its
first frame.

## Requirements

### R1. The capability

- R1.1 A new package `lighting`, importing nothing from this module, holding
  the `Canvas` capability as `screen` holds `Panel`:

  ```go
  // Key is one addressable light, numbered as the device numbers it.
  type Key struct {
      ID   byte   // the device's id for the light; a HID usage on the Apex
      Name string // the key's name where the driver knows it, else ""
  }
  // Pixel is one light's colour for one frame. Colours are absolute.
  type Pixel struct {
      ID      byte
      R, G, B uint8
  }
  type Canvas interface {
      // Keys lists the lights a frame may address, in the device's order.
      Keys() []Key
      // Frame shows one frame. A light not in px is left as it was.
      Frame(ctx context.Context, px []Pixel) error
      // Release hands the lighting back to the firmware.
      Release(ctx context.Context) error
      // Floor is the shortest interval at which frames reliably show.
      Floor() time.Duration
  }
  var ErrNoCanvas = errors.New("no canvas on this device")
  ```

  No effect, colour-space or timing code lives here. A consumer owns the
  renderer and the ticker; the capability owns the device's facts.
- R1.2 `sanshoku.Capabilities` reports `"lighting"` after `"screen"`;
  `docs/api.md` and the API canary cover the package.

### R2. The Apex in the `steelseries` driver

- R2.1 Products 0x1644 and 0x1646 satisfy `lighting.Canvas` on the same open
  device that satisfies `battery.Source`. No other product does; the Arctis
  base station and the mice are untouched.
- R2.2 A frame is one feature report of 641 bytes, the width the 0xFFC0
  collection declares for usage 0xF2: `[id, n, (key, r, g, b)×n, 0…]`, sent
  with `hidraw.SetFeature` behind a leading zero report number. `n` is the
  number of pixels given, at most 159 (what fits). The id is 0x61 on both
  products, as OpenRGB sends and GG sent through the receiver. **Amended
  after E1:** 0x61 shows on both; 0x21 shows on the cable only and is not
  acknowledged through the receiver, so one id serves both products.
- R2.3 `Keys` is the 85 ids GG addressed on this board, in GG's order, named
  by HID usage: 0x04–0x27, 0x28–0x2E, 0x2F–0x31, 0x33–0x39, 0x3A–0x45 (F1–F12),
  0x49–0x52 (navigation and arrows), 0xE0–0xE7 (modifiers), 0xF0 and 0xFB
  (the SteelSeries and media keys). 0x46–0x48 and 0x64 were not in GG's frame
  and are not listed; E3 checks whether they light.
- R2.4 After each frame the driver reads the acknowledgement, matching
  `reply[0] == id`, through `hidraw.Exchange` with the driver's `Timeout`, and
  returns its error if it does not come. The battery reader on the same handle
  already drains before it asks, so acks a consumer never waits for cannot
  starve it (issue #24).
- R2.5 `Floor` is what E2 measures: the shortest frame interval at which every
  frame is acknowledged within the timeout over a 30-second run. The number
  goes in the code with the date. GG's 56 ms median interval and 15 ms ack
  latency bound it from both sides. **Amended after E2:** 16 ms, the shortest
  interval tried, with no timeout and no tearing; the stream runs at the
  acknowledgement's pace below that (about 17 ms through the receiver, 4-6 ms
  on the cable).
- R2.6 `Release`'s behaviour is E4's result. If the firmware resumes its own
  lighting on its own once frames stop, `Release` stops nothing (the consumer
  stopped) and sends nothing, and the doc comment says how long the firmware
  takes. If it does not, `Release` sends 0x41, documents that the keyboard
  re-enumerates and the device is `sanshoku.ErrGone` afterwards, and the
  consumer is told to `Close` and rescan. Either way `Release` never sends 0x01.
  **Amended after E4:** the board held its last frame for five minutes, so
  `Release` sends 0x41 and the device is `ErrGone` afterwards.
- R2.7 No initialisation command is sent before the first frame unless E1 shows
  one is needed on this firmware. GG sent none; OpenRGB's 0x4B is a 65-byte
  feature report and is kept in the spec as the thing to try if frames are
  acknowledged and not shown.
- R2.8 The driver sends no other command to the keyboard than the frame, the
  acknowledgement read, the battery getter and (per R2.6) 0x41. The OLED
  commands 0x0C/0x1F/0x4C/0x5F blank the display and stay out.

### R3. The bench

- R3.1 `sanshoku-bench light`, refused without `--yes` like `screen`, finds
  every `lighting.Canvas`, prints its identity, key count and floor, and runs
  three patterns for `--hold` each (default 5 s): steady (one colour on every
  key), breathe (that colour scaled by a sine), and wave (hue by key index,
  moving). Then it calls `Release` and reports what the board did. The
  patterns are the oracle for "frames show"; they are not an API.
- R3.2 `--rate` sets the frame interval (default the floor) and the command
  reports frames sent, acks received, ack latency p50/p95/max, and frames
  whose ack timed out. This is how E2 is run.
- R3.3 `--keys` lists `Keys()` and exits, for a consumer mapping a layout.

### R4. Documentation

- R4.1 `docs/devices.md` (via `make generate`) shows the Apex with
  `battery, lighting` and the bench date; `support.go`'s entry says the frame
  id and floor measured.
- R4.2 `docs/credits.md` adds OpenRGB's Apex controller, SignalRGB's keyboard
  controller, Aurora PR #298 and apex-tux PR #74 for the frame format, the
  per-product id and the OLED commands to avoid.
- R4.3 `docs/design.md`'s capability list names `lighting.Canvas`, and a short
  section says what this module does and does not do about lighting: the
  stream, not the effects, and why (the capture).
- R4.4 README: the `lighting` package in the table, a line in Devices, and a
  changelog entry under `### Unreleased` marked as new API.

### R5. Experiments, run on the desk before the code is final

Each is one bench run with the user watching, recorded in Verification.

- E1 **Frame id by route.** On the cable (1646): a steady red frame with id
  0x61, then with 0x21; which shows, and which is acknowledged. Then through
  the receiver (1644), the same. If 0x61 is not shown on the cable, R2.2
  changes to a per-product id and the support entry says so.
- E2 **Floor.** Frames at 16 ms, 33 ms, 56 ms and 100 ms, 30 s each, with
  the ack statistics of R3.2. The floor is the shortest interval with no
  timed-out ack and no visible tearing.
- E3 **Addressing.** A frame naming only key 0x29 (Esc) red: does Esc alone
  change and the rest hold their last colour (R1.1's "left as it was"), or
  does the rest go dark? A frame naming key 0x00: whole-board broadcast as on
  Aurora's 1640, or ignored? Keys 0x46–0x48 and 0x64: lit or ignored?
- E4 **What stopping does.** Stream steady blue for 10 s and stop. Time how
  long until the firmware's own effect returns, if it does, up to five
  minutes. This fixes R2.6.
- E5 **Sharing.** The same as E2 at the floor with OpenRGB's server running
  and its Apex in Onboard mode: do acks still arrive, and does OpenRGB's exit
  (0x41) take the board away mid-stream as expected. Written into
  `docs/contention.md`.

## Acceptance Criteria

- [x] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries` and `CGO_ENABLED=0 make build` pass.
- [x] `lighting` has unit tests for frame encoding as a pure function (id, n,
      159-pixel cap, 641-byte width, a pixel list longer than the cap is an
      error) and the steelseries fake acknowledges a frame and refuses one on
      a product that has no canvas.
- [x] E1–E5 run on cachyos with the user watching, their outcome written into
      Verification, and R2.2, R2.5 and R2.6 amended to what was measured.
- [x] `sanshoku-bench light --yes` on cachyos shows steady, breathe and wave on
      the Apex, releases it, and the ack statistics are pasted below. This is
      the integration-boundary criterion: a real keyboard, not a fake.
- [x] `sanshoku-bench read --driver steelseries` still reads the Apex battery
      on the same handle while nothing streams, and immediately after a
      `light` run.
- [x] The Arctis Nova Pro Wireless and the Razer, Logitech, Apple and NZXT
      benches are unchanged (`sanshoku-bench verify` where a reference exists).
- [x] Docs per R4, including the changelog entry.

## Risks & Assumptions

- **A second streamer.** OpenRGB's server holds the same node, and in Direct
  mode it streams too; two writers alternate frames and the board flickers
  between them. This module cannot arbitrate that; the consumer that starts a
  stream owns the decision, as hotaru already does when it chooses between
  OpenRGB and a direct driver. E5 records what sharing looks like so the
  consumer's doctor can say it.
- **OpenRGB's exit reboots the keyboard.** 0x41 re-enumerates it, the hidraw
  number moves, and an open handle is `ErrGone`. Not this module's doing, but
  a consumer streaming through it sees the device vanish when OpenRGB stops;
  the battery reader already handles `ErrGone` by rescanning.
- **Wireless battery.** Frames through the receiver at 18 a second keep the
  radio busy; GG does it, so the firmware is built for it, but a consumer
  should expect the battery to fall faster while an effect runs. Noted in the
  capability's doc comment, not measured here.
- **Nothing is written to flash.** The frame path is the one GG uses all day;
  the 0x20 record proved RAM only; no command in R2.8 persists anything. The
  keyboard's onboard profile is as the user left it.
- **The key list is one board's.** 85 keys of a US TKL. A full-size Gen 3 or
  another layout has more; `Keys` is per product and the support entry says
  which one was measured. Unknown ids are sent and ignored (E3 says whether
  that holds), so a wrong list lights fewer keys rather than failing.
- **Rollback**: revert. The capability is additive; nothing consumes it until
  hotaru's spec does.

## Alternatives Considered

Considered rendering effects in hotaru through OpenRGB's Direct mode instead
of this module; rejected because OpenRGB's exit reboots the keyboard, its
Direct frames carry no acknowledgement back to the caller, and the battery
driver here already holds the handle, so one process would otherwise hold the
node twice through two libraries.

Considered shipping effects in this module; rejected because effects are a
product decision (hotaru's scenes, the design system's motion rules) and this
module is device access. The bench patterns exist to prove the stream and are
deliberately not exported.

Considered waiting for the firmware effect command to turn up in a later GG
capture; rejected because the capture shows GG itself never uses one for its
effects or presets, so there is nothing further to wait for on this board.

## Verification

All runs on `cachyos` on 2026-10-01, with the user watching the board,
OpenRGB and hotaru stopped except in E5. Board firmware 3.24.1.

- **E1, frame id by route.** Receiver (1644): 0x61 red acknowledged in
  14.6 ms (`61 00 55 …`) and the board went red; 0x21 green was not
  acknowledged in 500 ms and the board stayed red. Cable (1646): 0x61 blue
  acknowledged in 3.7 ms and shown, then 0x21 red acknowledged in 3.6 ms
  (`21 00 55 …`) and shown. R2.2 keeps 0x61 for both.
- **E2, floor.** Receiver, wave, 30 s each, no tearing or freezing seen at any
  rate:

  ```
  wave every 16ms:  1723 frames, 1723 acknowledged, 0 timed out; ack p50 17.0 ms, p95 20.0 ms, max 30.0 ms
  wave every 33ms:   910 frames,  910 acknowledged, 0 timed out; ack p50 16.9 ms, p95 22.8 ms, max 28.8 ms
  wave every 56ms:   536 frames,  536 acknowledged, 0 timed out; ack p50 16.9 ms, p95 22.8 ms, max 29.7 ms
  wave every 100ms:  300 frames,  300 acknowledged, 0 timed out; ack p50 16.8 ms, p95 21.8 ms, max 30.7 ms
  ```

  At 16 ms the stream ran at about 57 frames a second, paced by the
  acknowledgement. Floor 16 ms.
- **E3, addressing.** After an all-black frame, a frame naming only 0x29 lit
  Esc blue and nothing else; on the red board Esc changed and the rest stayed
  red, so an unnamed light keeps its colour. Key 0x00 alone (green): nothing
  changed, so it is not a broadcast on this board. Keys 0x32, 0x46, 0x47,
  0x48 and 0x64 (white): all acknowledged, nothing lit.
- **E4, stopping.** Steady blue for 10 s (179 frames, all acknowledged), then
  nothing: the board was still blue, unchanged, five minutes later. Release
  sends 0x41.
- **E5, sharing.** OpenRGB's server started (Apex left on its onboard
  effect), then a 30 s wave at 16 ms: 1253 frames acknowledged in the first
  20 s and the wave was clean. At 20 s OpenRGB was stopped; its exit rebooted
  the keyboard, the next frame failed with `device gone: no such device`, one
  frame timed out at the moment of the reboot, and Release returned
  `ErrGone`. The board showed the onboard rainbow. Written into
  `docs/contention.md`.
- **The bench**, on the cable with the user watching (steady red, pulsing
  red, a rainbow wave, then the reboot to the onboard rainbow):

  ```
  steelseries  SteelSeries Apex Pro TKL Wireless Gen 3 (1038:1646)  /dev/hidraw4  85 keys, floor 16ms
    steady   every 16ms: 500 frames, 500 acknowledged, 0 timed out; ack p50 5.9 ms, p95 8.7 ms, max 15.9 ms
    breathe  every 16ms: 500 frames, 500 acknowledged, 0 timed out; ack p50 5.9 ms, p95 7.9 ms, max 14.7 ms
    wave     every 16ms: 500 frames, 500 acknowledged, 0 timed out; ack p50 5.8 ms, p95 8.0 ms, max 13.8 ms
    release: ok (0.4 ms); watch the board for what the firmware shows
  ```

- **Battery on the same handle.** Before a `light` run: 85% charging
  (3.1 ms). 3 s after Release: no reading on the new node (hidraw4 moved to
  hidraw5; the keyboard was still booting). 13 s after: 85% charging
  (3.6 ms). Through the receiver, before any frame: 85% discharging.
- **Other devices.** `sanshoku-bench verify` on the desktop: Kraken coolant,
  pump and fan and the G502 X PLUS agree with liquidctl and solaar; the
  Arctis Nova Pro reads (headset off, not compared). On cachyos the Razer
  Mouse Dock Pro gave no reading on both this branch and a `main` build (the
  mouse asleep), so unchanged. No Apple device was connected on either
  machine.

## Gaps found

- **Frame encoding lives in `steelseries`, not `lighting`.** The acceptance
  criterion says "`lighting` has unit tests for frame encoding", but R1.1
  keeps device facts out of `lighting`, and the 0x61 id, the 641-byte width
  and the 159-pixel cap are the Apex's. `encodeFrame` is an unexported pure
  function in `steelseries/lighting.go`, tested in
  `steelseries/lighting_test.go`. The criterion is read as "the encoding has
  unit tests".
- **GG's key order is not the order R2.3 lists the ranges in.** Re-reading
  the capture's first frame: A–Z, 1–0, Esc, Backspace, Tab, `-`, `=`, `[`,
  `]`, `;`, `'`, `` ` ``, `,`, `.`, `/`, Caps Lock, F1–F12, 0xFB, the
  navigation block, the modifiers without Right Alt, 0xF0, then Space,
  Enter, `\` and Right Alt last. The set is R2.3's; `apexKeys` carries the
  capture's order.
- **The acknowledgement echoes the count.** Every ack in the capture starts
  `61 00 55`: the id, a zero, and n (0x55 = 85). The driver matches on the
  id alone as R2.4 says; the count is noted in case two frames in flight ever
  need telling apart.
- **The frame is a feature report with no report ID** (SET_REPORT wValue
  0x0300 in the capture), which is what R2.2's leading zero report number
  produces through hidraw.
- **E1's 0x21 cannot go through the API**, by design (R2.2 fixes the id per
  product). It is sent with a scratch tool built on `hidraw` that is not
  committed.
