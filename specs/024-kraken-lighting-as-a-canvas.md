# 024 — The Kraken's lighting as a canvas

**Issue**: #70

## Status: INCOMPLETE

## Context

The `nzxt` driver reads the Kraken 2024 Elite's status and draws on its
screen (specs 005 and 021). Its lighting, the pump ring and the fans chained to
it, is driven by OpenRGB, through the same hidraw node. Two programs writing
one node have cost hotaru three specs and a bug:

- hotaru spec 068: a frame sent straight after a mode change interleaved with
  it on the channel, and the ring stuck on one wrong LED.
- hotaru spec 070: the firmware turned one ring LED green after the screen
  changed hands, and only a resend from the host put it right.
- hotaru #206: OpenRGB 1.1 reads the cooler's accessory list once, at start,
  and gives up after ten reports. When hotaru's panel traffic fills those ten,
  both channels read 0 LEDs and no client can repair it. It failed once in 15
  restarts under ordinary dashboard traffic.

The fix for all of them is one owner of the node. This module already owns it
for status and the screen, so the lighting moves here, and OpenRGB's detector
for the cooler is switched off where this module is used.

`docs/kraken-lighting.md` is the protocol, written down from OpenRGB's Hue 2
driver and a capture of NZXT CAM 4.76.9 on firmware 1.2.12. Two of its facts
shape this spec:

- **`26 14` is a live write.** It sets every LED of a channel and is
  acknowledged in 1.3 to 3.1 ms. CAM sends it once a second to keep the ring in
  step with the screen. `26 04`, the write CAM uses for a fixed colour, is
  acknowledged in about 335 ms. A canvas that waits for each frame's
  acknowledgement, as the Apex's does (spec 010), can only run on `26 14`.
- **Its rate is not known.** CAM never sent `26 14` faster than once a second.
  The floor, and whether the cooler holds a frame, are measured here first
  (R6), and the requirements that depend on them are amended to the results.

The consumer is hotaru, whose live-effects spike (hotaru #205) has the Kraken
as the device it most wants effects on. hotaru's adoption is a hotaru spec.

## Requirements

### R1. The Kraken as a canvas

- R1.1 The `nzxt` driver's device for the Kraken Elite (`1e71:3012`)
  satisfies `lighting.Canvas` on the handle it already holds for status, and
  `sanshoku.Capabilities` reports `"lighting"` for it. `Describe` and the
  support entry gain the capability.
- R1.2 **The accessory list.** Before the first `Keys` or `Frame` on an open
  handle, the driver asks `20 03` and reads until a report starting `21 03`
  arrives, skipping every other report, within a deadline of 2 s (the cooler
  answered in 577 to 597 ms). No reply within the deadline is
  `hidraw.ErrSilent`; no frame is sent, and the question is asked again at the
  next call. The answer is kept for the life of the handle.
- R1.3 **Keys.** One key per LED: channel 1 first, then channel 2, each in the
  channel's own order. Ids run from 0 with no gaps. Names are `ring 1` to
  `ring 24` for the Kraken Elite ring (`1E`) and `fan 1` and on for a channel
  of fans. A channel whose accessory ids are all known has the sum of their
  LED counts. A channel holding an id the table in `docs/kraken-lighting.md`
  does not know contributes no keys, and `Keys` still returns the other
  channel's. An empty channel contributes no keys.
- R1.4 **Frame.** `Frame` keeps what it last showed and merges `px` into it,
  since the interface leaves a light not in `px` as it was. A `Pixel` whose
  `ID` is not in `Keys` is an error and nothing is sent. For each channel whose
  colours changed, the driver sends `26 14 <ch> 01` and the channel's LEDs in
  G, R, B order, and waits for the acknowledgement (`FF 01`, then `26 14` at
  bytes 14 and 15) within a deadline. A channel that did not change is not
  sent. *To be amended by E1 and E3:* if `26 14` does not show or does not
  hold, the frame is `26 04` followed by `26 06`, and R1.6 says so.
- R1.5 **No acknowledgement** within the deadline is `hidraw.ErrSilent` for
  that frame. The held frame keeps the new colours, so the next `Frame` sends
  them again.
- R1.6 **Floor** is what E2 measures. Until then it is 100 ms.
- R1.7 **Release.** *To be decided by E7:* the cooler has no "onboard" state
  that CAM hands back to. The default is that `Release` sends nothing, forgets
  the held frame and never fails on an open device. The cooler keeps the last
  frame.
- R1.8 **The cooler holds a frame** unless E1 shows otherwise. If it does not,
  the device also satisfies `lighting.Fading` with the hold E1 measures.
- R1.9 **One exchange at a time.** A frame, a status question and a screen
  command never interleave on the node: they share the driver's existing
  exchange path. A frame waits for a screen transfer in progress. This is the
  lesson of hotaru spec 068.
- R1.10 **A frame shows whatever mode the channel is in.** A firmware
  effect is a mode over the channel's LEDs, not a replacement for them: the
  channel may be running one because the firmware started it, or because
  another program set it. The first frame on a handle takes each channel it
  names over from any mode. *To be amended by E9:* if `26 14` alone does not
  take a channel over from a running effect, the driver first sends
  `26 06 <ch> 00 01 ...`, the fixed mode, once per channel per handle, and
  R1.11 gains it.
- R1.11 **What the driver sends for lighting:** `20 03` and `26 14`, and
  `26 04` and `26 06` only if R1.4 is amended to them. It never sends duty (`72 xx`), never addresses the bootloader
  (`1e71:3011`), and never logs a reply's bytes 2 to 9, which carry the
  device serial.

### R2. Firmware effects: a separate spec

The cooler runs 16 effects of its own at no cost to the host (`2A 04` and
`26 06`, see the protocol page). They are not in this spec; #72 has them.
`lighting.Onboard` (spec 023) fits the cooler badly: it reads the running
effect back, and no read for the Kraken's effect is known; and it assumes the
device stores each effect's settings, where the Kraken takes them with every
command. #72 decides between `Onboard` and a new interface, after this spec
has settled the channels, the accessory list and the acknowledgements.

While OpenRGB's detector for the cooler is off, a consumer has the canvas and
no firmware effects on it. #72 also enumerates the effects: what the firmware
offers, with each one's number, speed range and colour count. The canvas does
not depend on it: per-LED frames work whatever effect is running (R1.10).

### R3. The bench

- R3.1 `sanshoku-bench light --yes` runs on the Kraken as it does on the Apex
  and the F75: steady, breathe and wave.
- R3.2 `sanshoku-bench light --keys` lists the Kraken's keys, with each
  channel's accessory ids.
- R3.3 `sanshoku-bench read --driver nzxt` reads status on the same handle
  before, during and after a `light` run.
- R3.4 The bench's screen command runs during a `light` run without either
  failing (R1.9).

### R4. Tests

- R4.1 Encoding, as pure functions: the `26 14` report for a known frame byte
  for byte, G, R, B order, the channel byte for each channel.
- R4.2 The accessory list: the reply's slots per channel; the known ids'
  counts; an unknown id; an empty channel; **a reply that arrives after 25
  other reports is found** (the hotaru #206 case); no reply is
  `hidraw.ErrSilent`, and the question is asked again.
- R4.3 On the fake cooler, extended with only measured behaviour: a frame
  merges and sends only changed channels; an unknown `ID` writes nothing; no
  acknowledgement is `ErrSilent` and the next frame resends; a frame waits for
  a screen transfer in progress.
- R4.4 A guard: across every test in the package, the fake cooler has been
  sent no lighting command outside R1.11.
- R4.5 `make test` still opens no device.

### R5. Documentation

- R5.1 `docs/devices.md` (generated): the Kraken Elite with `cooling, screen,
  lighting`, and its firmware column corrected. It reads 1.2.0 from
  `bcdDevice`, which does not follow the firmware. The cooler on this desk is
  1.2.12.
- R5.2 `docs/design.md`, "What belongs here": the Kraken's lighting joins the
  Apex's frames as an exception to "lighting is OpenRGB's", with the reason
  (one owner of the node).
- R5.3 `docs/contention.md`: OpenRGB must not drive the cooler's lighting
  while this module does. Switch off its detector for the cooler
  (`"NZXT Kraken 2024 ELITE Series RGB"` in `OpenRGB.json`).
- R5.4 `docs/kraken-lighting.md`: each open question answered by R6, and the
  facts R6 confirms marked as verified.
- R5.5 README: the changelog entry under `### Unreleased`. `docs/api.md`
  regenerated.

### R6. Experiments, run on the desk before the code is final

Each is one bench run with the user watching, with hotaru stopped and OpenRGB
stopped or its detector for the cooler off. Results go in Verification.

- E1 **Does `26 14` show and hold?** One frame of one colour on the ring, then
  nothing for 60 s: does it show, and does it stay? Then the same with `26 06`
  sent after it. This settles R1.4 and R1.8.
- E2 **The floor.** A moving frame on the ring, each frame waiting for its
  acknowledgement, at 100, 50, 33, 20 and 16 ms, 20 s each: is it even, and
  what are the acknowledgement times? Then 16 ms without waiting: does it lag,
  drop or stall? This settles R1.6.
- E3 **Report length.** E1 with 64-byte and with 512-byte reports.
- E4 **`26 04` against `26 14`.** The acknowledgement time of each at the
  length E3 chose, and whether a `26 04` after a stream of `26 14` shows.
- E5 **The screen.** A held frame across a bucket upload, the firmware
  readout, and the dashboard's return: does any LED change (hotaru spec 070's
  green LED)? Then a moving frame during a bucket upload: does it stall, and
  for how long?
- E6 **The accessory list under load.** 50 reads while the bench uploads to
  the screen: every one answered within the deadline.
- E7 **Release.** After `Release`, what the ring shows, and whether it stays.
- E8 **OpenRGB's family on 1.2.12.** One frame by `22 10` and `22 A0` (the
  bench only, not the driver): does it still show? This says whether OpenRGB
  and this module would fight over the same LEDs, which R5.3 warns about.
- E9 **A frame over a firmware effect.** The bench sets Breathing with
  `2A 04` (an experiment only; the driver does not send it), then sends one
  `26 14` frame: does the frame show, does the effect stop, and does anything
  of the effect come back? Then the same with `26 06 ... 01` sent before the
  frame. Then, after `Release`, does the effect return? This settles R1.10,
  and tells #72 how its effects and the canvas hand a channel to each other.

## Acceptance Criteria

- [ ] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries` and `CGO_ENABLED=0 make build` pass.
- [ ] `go test ./...` passes on Windows, and `go vet ./...` passes for
      windows, linux and darwin.
- [ ] E1 to E9 run on the Kraken with the user watching, recorded in
      Verification, and R1.4, R1.6, R1.7, R1.8 and R1.10 amended to what was
      measured.
- [ ] `sanshoku-bench light --yes` shows steady, breathe and wave on the ring
      and the fans, starting from a channel running a firmware effect. This is the integration-boundary criterion: a real cooler,
      not a fake.
- [ ] `sanshoku-bench read --driver nzxt` reads status during that run, and
      the screen command runs during it (R3.3, R3.4).
- [ ] The accessory read finds the reply behind 25 foreign reports in a test,
      and that test fails with the reply-matching removed; restored.
- [ ] Docs per R5, including the changelog entry.

## Out of scope

- **hotaru.** Drawing on the cooler, switching OpenRGB's detector off, and
  hotaru #206's closure are a hotaru spec.
- **Streaming the screen** (#27).
- **Pump and fan duty.** CAM sends it every second. This module does not.
- **The firmware effects** (#72), the temperature and audio effects, and
  `26 14 01 01` with no colours.
- **Other Hue 2 devices.** The protocol is shared, but only this cooler is on
  a desk.

## Risks & Assumptions

- **Two writers.** If OpenRGB still drives the cooler, its `22` writes and this
  module's frames compete for the same LEDs. R5.3 documents this, and E8 shows
  whether it is real on 1.2.12. hotaru's adoption spec turns the detector off.
- **One firmware.** Everything is measured on 1.2.12. A cooler on another
  firmware may differ; the support entry names the version tested.
- **`26 14` is inferred** from CAM's use of it. E1 decides; `26 04` with
  `26 06` is the fallback, at about 335 ms a frame.
- **The serial.** Every acknowledgement carries the device serial. It is never
  logged and never in a fixture.
- **Rollback:** revert. The capability is additive, and with OpenRGB's detector
  still on, hotaru behaves as it does today.

## Alternatives Considered

Considered keeping the cooler's lighting in OpenRGB and working around #206 in
hotaru (quiet the panel while OpenRGB detects); rejected because two writers on
one node is the cause of 068, 070 and #206, and a workaround covers only the
restarts hotaru can see.

Considered OpenRGB's `22` family as the frame command, since it already drives
the cooler; rejected as the default because its writes are not acknowledged,
so a stream cannot be paced by the device. E8 keeps it measured.

## Verification

Not yet run.
