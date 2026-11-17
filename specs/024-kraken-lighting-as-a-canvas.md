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
- **Its rate was not known.** CAM never sent `26 14` faster than once a
  second. R6 measured the floor and the hold on the desk, and the requirements
  below are amended to the results (see Verification).

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
  colours changed, the driver sends `26 14 <ch> <bit>` and the channel's LEDs
  in G, R, B order, where `<bit>` is the channel's bit in the mask of
  channels to show (`01` the ring, `02` the fans; E11), and waits for the acknowledgement (`FF 01`, then `26 14` at
  bytes 14 and 15) within a deadline of 500 ms. A channel that did not change
  is not sent. The next frame is not sent before the acknowledgement: without
  the wait the firmware drops frames it cannot keep up with (E2).
- R1.5 **No acknowledgement** within the deadline is `hidraw.ErrSilent` for
  that frame. The held frame keeps the new colours, so the next `Frame` sends
  them again.
- R1.6 **Floor** is 16 ms. Every period from 100 to 16 ms ran even with no
  frame lost, with acknowledgements at a median of 1.9 ms (E2).
- R1.7 **Release commits the held frame.** For each channel the canvas has
  drawn, `Release` writes the held colours with `26 04` and shows them with
  `26 06 <ch> 00 01 ...`, each waiting for its acknowledgement within 1 s
  (`26 04` takes about 335 ms). The colours become the channel's own, so a
  later readout shows them (E5, E7). Then it forgets the held frame. A channel
  never drawn is not sent. This replaces any firmware effect on the channel.
- R1.8 **The cooler holds a frame** with nothing sent (E1). The device is not
  `lighting.Fading`.
- R1.9 **One exchange at a time.** A frame, a status question and a screen
  command never interleave on the node: they share the driver's existing
  exchange path. A frame waits for a screen transfer in progress. This is the
  lesson of hotaru spec 068.
- R1.10 **A frame shows whatever mode the channel is in.** A firmware
  effect is a mode over the channel's LEDs, not a replacement for them: the
  channel may be running one because the firmware started it, or because
  another program set it. The first frame on a handle takes each channel it
  names over from any mode. `26 14` alone does this: it draws over a running
  effect, and the effect stops showing (E9). No `26 06` is sent first. The
  channel's mode is unchanged, so a readout brings the effect back (R1.13).
- R1.11 **What the driver sends for lighting:** `20 03`, `26 14`, and `26 04`
  each followed by `26 06` (R1.7). It never sends a `26 06` without a `26 04`
  before it: during an effect that freezes the effect's last frame (E9). It
  never sends the `22` family, whose split write leaves LED 22 unset (E8). It
  never sends duty (`72 xx`), never addresses the bootloader (`1e71:3011`),
  and never logs a reply's bytes 2 to 9, which carry the device serial.
- R1.12 **Report length.** Every lighting report is 512 bytes, the length the
  report descriptor declares. A 64-byte `26 14` lights only LEDs 1 to 20 (E3).
- R1.13 **The readout.** The firmware readout re-applies each channel's mode
  150 to 300 ms after the switch, which replaces a `26 14` frame (E5, E10).
  Before the driver's own switch to the readout, in the panel's `Readout` and
  in `Close`'s handback, it commits the held frame as `Release` does (R1.7)
  for each channel the canvas has drawn. The re-apply then shows the same
  colours. It does not resend after the switch: that depends on timing the
  re-apply, and the buffer shows until the resend (E10). A handle with no held
  frame sends nothing.

- R1.14 **`screen.Panel.Floor(bytes)` becomes `ImageFloor(bytes)`.** The
  Kraken is a `screen.Panel` and a `lighting.Canvas`, and `Canvas` has a
  `Floor()` of its own; one type cannot carry both. A breaking change,
  recorded in the changelog.
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
- R3.2 `sanshoku-bench light --keys` lists the Kraken's keys. The key names
  say which accessory a channel holds (`ring`, `fan`); the ids themselves are
  not part of `lighting.Canvas`, and the bench does not print them.
- R3.5 `sanshoku-bench light` takes `--driver`, so a run on the Kraken does
  not stream to every other canvas on the desk (the Apex reboots on release),
  and `--readout`, which returns a screen to its readout on the same handle
  before release, to check R1.13.
- R3.3 `sanshoku-bench read --driver nzxt` reads status on the same handle
  before, during and after a `light` run.
- R3.4 The bench's screen command runs during a `light` run without either
  failing (R1.9).

### R4. Tests

- R4.1 Encoding, as pure functions: the `26 14`, `26 04` and `26 06` reports
  for a known frame byte for byte, 512 bytes long, G, R, B order, the channel
  byte for each channel.
- R4.2 The accessory list: the reply's slots per channel; the known ids'
  counts; an unknown id; an empty channel; **a reply that arrives after 25
  other reports is found** (the hotaru #206 case); no reply is
  `hidraw.ErrSilent`, and the question is asked again.
- R4.3 On the fake cooler, extended with only measured behaviour: a frame
  merges and sends only changed channels; an unknown `ID` writes nothing; no
  acknowledgement is `ErrSilent` and the next frame resends; a frame waits for
  a screen transfer in progress; `Release` sends `26 04` then `26 06` for each
  drawn channel and nothing for the others; `Readout` and `Close` send the
  held frame again after the readout, and nothing when no frame is held.
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
- E11 **The fans' channel byte**, found during the acceptance runs: which
  `26 14` layout draws the fans.
- E10 **When the readout re-applies.** A held `26 14` frame, the switch to the
  readout, and the same frame sent again at once: does it stay for 60 s? If
  it does not, the delay after which a resend stays. Then the frame committed
  with `26 04` and `26 06` before the switch. This settles R1.13.

## Acceptance Criteria

- [x] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries` and `CGO_ENABLED=0 make build` pass.
- [ ] `go test ./...` passes on Windows, and `go vet ./...` passes for
      windows, linux and darwin.
- [x] E1 to E9 run on the Kraken with the user watching, recorded in
      Verification, and R1.4, R1.6, R1.7, R1.8 and R1.10 amended to what was
      measured.
- [x] E10 run with the user watching, recorded in Verification, and R1.13
      amended to it.
- [x] After the driver's own switch to the readout during a held frame
      (`sanshoku-bench light --readout`), the lights show the held frame, not
      the buffer (R1.13), with the user watching.
- [x] After `Release` and a readout, the ring shows the released frame
      (R1.7), with the user watching.
- [x] `sanshoku-bench light --yes` shows steady, breathe and wave on the ring
      and the fans, starting from a channel running a firmware effect. This is the integration-boundary criterion: a real cooler,
      not a fake.
- [x] `sanshoku-bench read --driver nzxt` reads status during that run, and
      the screen command runs during it (R3.3, R3.4).
- [x] The accessory read finds the reply behind 25 foreign reports in a test,
      and that test fails with the reply-matching removed; restored.
- [x] Docs per R5, including the changelog entry.

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

- **Two writers.** If OpenRGB still drives the cooler, its `22` writes and
  this module's frames compete for the same LEDs: E8 showed the `22` family
  writes the same buffer `Release` commits. R5.3 documents this. hotaru's
  adoption spec turns the detector off.
- **One firmware.** Everything is measured on 1.2.12. A cooler on another
  firmware may differ; the support entry names the version tested.
- **The readout's timing** is assumed to be at the switch, which E10 checks.
  If the re-apply comes later, R1.13's resend waits for it, and a frame drawn
  in that window is lost until the resend.
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
the cooler; rejected because it commits to the buffer at 335 to 452 ms a
frame, and its split 64-byte write leaves LED 22 unset (E8).

Considered committing every frame with `26 04` and `26 06`, so a readout never
loses one; rejected because the floor would be about 335 ms. Considered a new
optional interface for the consumer to say a frame is still; rejected for
now because the driver performs every switch to the readout itself (R1.13).

## Verification

### R6, the experiments (2026-11-16, firmware 1.2.12)

Run from a throwaway probe that writes raw reports to the hidraw node, and
`sanshoku-bench screen --yes` for the screen. hotaru and OpenRGB were
stopped; hayami's tray and TUI held the node and read status throughout. The
user watched the ring for every run. Channel 1, the ring, unless said.

**E3, report length (run first, because E1's first frame showed it).** A
`26 14` frame in a 64-byte report lit LEDs 1 to 20 and left 21 to 24 as they
were: 4 header bytes and 72 colour bytes do not fit. In a 512-byte report all
24 LEDs lit. The report descriptor declares every output report (`10`, `20`,
`22`, `26`, `2A`, `30`, `36`, `70`, `72`, `74` and others) as a report ID and
511 bytes, and the interrupt endpoint's packet size is 512. A 64-byte report
is a short report the firmware accepts when the command fits. Every run below
used 512 bytes.

**E1, `26 14` shows and holds.** Green showed with the acknowledgement in
2.1 ms, and was still green after 60 s with nothing sent and the handle closed.
A blue `26 14` followed by `26 06 01 00 01 ...` did not stay blue: the ring
showed the stored buffer (white, LED 22 green), and `26 06` was acknowledged
in 119.9 ms.

**E4, `26 04` against `26 14`.** `26 04` (magenta) was acknowledged in
334.7 ms and the `26 06` after it in 1.8 ms; the ring showed all 24 LEDs
magenta. `26 14` was acknowledged in 1.4 to 9.0 ms throughout E2. `26 06`
applies the `26 04` buffer.

**E2, the floor.** Two white dots chasing on dim blue, each frame waiting for
its acknowledgement, 20 s per period. The user saw every run as smooth.

| Period | Frames | Rate | Acknowledgement (min, median, p95, max) | Unacknowledged |
|---|---|---|---|---|
| 100 ms | 200 | 10.0 fps | 1.4, 1.9, 2.8, 2.9 ms | 0 |
| 50 ms | 400 | 20.0 fps | 1.4, 1.9, 2.9, 3.9 ms | 0 |
| 33 ms | 607 | 30.3 fps | 1.4, 1.9, 2.3, 3.0 ms | 0 |
| 20 ms | 1000 | 50.0 fps | 1.4, 1.9, 1.9, 4.3 ms | 0 |
| 16 ms | 1250 | 62.5 fps | 1.5, 1.9, 2.9, 7.7 ms | 0 |
| 16 ms, no wait | 1250 | 62.5 fps | (1250 of 1250 acknowledged) | 0 |

Two runs beyond the spec found the edge. With no period and no wait for 10 s,
the endpoint took 77,554 frames (7,755 a second) and the firmware acknowledged
2,462 (about 246 a second); the user saw the chase jump. With no period and a
wait for each acknowledgement, 4,975 frames in 10 s (497.5 fps), all
acknowledged (median 2.0 ms, max 9.0 ms); the user saw an even blur. The
firmware drops frames it cannot keep up with and does not acknowledge them.
Waiting for the acknowledgement paces a stream without loss.

**E5, the screen.** An orange `26 14` frame stayed orange through a bucket
upload and a 20 s hold. When the screen went back to the firmware readout the
ring showed the `26 04` buffer (magenta) and stayed so. The readout re-applies
the channel's mode, which a `26 14` frame does not change. A 33 ms stream
during an upload and readout: 452 frames, all acknowledged, one in 86.9 ms
during the 94 ms upload, 105 ms behind schedule in all. The user saw no hitch
and no flash of the buffer.

**E6, the accessory list under load.** 50 of 50 `20 03` answered by `21 03`
within 2 s (693 to 762 ms, median 757 ms) while 25 uploads and readouts ran
from another handle, all of which succeeded. Channel 1 held `1E` and channel 2
`1D`.

**E7, release.** Closing the handle sends nothing, and the last `26 14` frame
stays (E1) until the next readout, which replaces it with the channel's mode
(E5).

**E8, OpenRGB's family on 1.2.12.** `22 10` and `22 11` in 64-byte reports with
`22 A0` showed all 24 LEDs, with `22 A0` acknowledged in 452 ms. The `22`
family writes the same buffer as `26 04`: data written by either family shows
through either apply (`26 04` green, then `22 A0`; `22 10` red, then `26 06`).
After a readout:

| `22` write | Showed | After the readout, LED 22 |
|---|---|---|
| `22 10` + `22 11`, 64 bytes each, `22 A0` count `28` (OpenRGB's) | all cyan | green |
| the same, `22 A0` count `18` | all yellow | off |
| one 512-byte `22 10` with 24 LEDs, count `18` | all white | white |
| `26 04`, 512 bytes (E5) | all magenta | magenta |

The split write shows correctly and does not set LED 22 in the buffer. With
count `28` the LED keeps its previous value, so the readout re-applies a stale
LED (shown as green here, and blue after the E10 runs below). With count `18`
it read off. This is hotaru spec 070's green LED: OpenRGB always splits,
because it writes 64-byte reports. Why count `18` reads off is not known. With data, `22 A0` was acknowledged in 452 ms (split) or 335 ms
(one report); with no new data, in 2.1 to 2.3 ms. The slow part is committing
new data.

**E9, a frame over a firmware effect.** Breathing (`2A 04`, mode `07`, red,
acknowledged in 118.0 ms) was replaced by one blue `26 14` frame: solid blue,
the effect stopped, and it did not come back with the handle closed. The next
readout brought Breathing back. A bare `26 06` during Breathing turned the ring
off, twice; during Spectrum Wave (mode `02`) it froze the wave as it was. The
firmware runs an effect by writing the same buffer, and `26 06` shows the
buffer as the effect last wrote it.

**The model these give.** Each channel has one LED buffer and a mode. `26 04`
and `22 1g` write the buffer; `26 06` and `22 A0` set the fixed mode and show
it; `2A 04` sets an effect, which draws into the buffer. `26 14` draws over
whatever the mode shows and changes neither. The firmware readout re-applies
the mode.

### E10, when the readout re-applies (2026-11-16)

Same setup. A green `26 14` frame was held, `sanshoku-bench screen --yes`
uploaded and returned to the readout, and the green frame was sent again
after a delay. The buffer held hotaru's colours with a stale LED 22.

| Resend after the readout | Acknowledgement | The ring |
|---|---|---|
| 55 ms | 1.4 ms | the buffer took over and stayed |
| 150 ms | 1.4 ms | the buffer took over and stayed |
| 300 ms | 74.2 ms | green, seamless |
| 1 s | 1.4 ms | the buffer flashed, then green, and stayed |
| 5 s | 1.8 ms | green, and stayed |

The re-apply starts 150 to 300 ms after the readout command, which returns in
about 4 ms. A frame sent during it is acknowledged late and shows after it.
A held blue `26 14` frame was then committed with `26 04` (335.2 ms) and
`26 06` (119.0 ms; it takes about 2 ms when no live frame is showing) before
the switch. The ring stayed blue through the upload and the readout, with no
flicker.

**Before E1, `26 06` showed white with LED 22 green.** That was hotaru's ring
colours, written through OpenRGB, with a stale LED 22. hotaru's pale tans read
as white on the ring. After hotaru restored the ring through OpenRGB, a
readout showed the same white with LED 22 blue, the value of the probe's last
commit. The probe's own split write of hotaru's colours gave the same result.
OpenRGB wrote nothing at start, and detected 24 LEDs on each channel.

### E11, the fans' channel byte (2026-11-16)

Found during the acceptance runs below. The driver first sent the fans
`26 14 02 01`, reading byte 2 as the channel, as in `26 04` and `26 06`. The
fans did not draw it: over Breathing they froze, and they turned green only
when the readout's commit (`26 04`, `26 06`) reached them. Single frames,
one at a time, with the user watching both channels:

| Sent | The ring | The fans |
|---|---|---|
| `26 14 02 01` yellow | no change | no change |
| `26 14 02 02` cyan | no change | cyan at once |
| `26 14 02 01` white, then `26 14 01 03` red 2 s later | red at the second | white at the second |
| `26 14 02 01` magenta, then `26 14 01 02` green | green | magenta |

Byte 2 is the channel the colours are for; byte 3 is a mask of the channels
to show. A channel's frame is held until a report with its bit arrives. A
first reading of these runs, made while the user watched the fans only, took
byte 3 for the channel. The driver briefly sent `26 14 01 02`, which drew the
fans' colours on the ring and showed a stale fan frame. The steady runs looked
right on the fans only because that stale frame was the same colour.

### The acceptance runs (2026-11-16)

`sanshoku-bench` built from this branch, hotaru and OpenRGB stopped, the
user watching. Before each run the probe set Breathing (`2A 04`, red) on
both channels.

- **`light --keys --driver nzxt`**, with hotaru and OpenRGB running on the
  same node: 48 keys, `ring 1` to `ring 24` and `fan 1` to `fan 24`, floor
  16 ms.
- **`light --yes --driver nzxt --pattern steady --color 0080ff --hold 5s`**
  over Breathing: both channels switched to steady light blue together. The
  first frame was acknowledged in 7.9 ms; the 312 after it were unchanged
  and not sent.
- **`--pattern breathe --hold 10s`**: 625 frames, all acknowledged (median
  7.9 ms, max 20.9 ms); the fans breathed in step with the ring.
- **`--pattern wave --hold 8s`**: 500 frames, all acknowledged (median
  7.9 ms, max 11.5 ms); the wave moved on both in step; `Release` committed it
  in 681.8 ms and it stayed, frozen.
- **`--pattern steady --color ff6000 --hold 5s --readout`** over Breathing:
  both turned orange together; the test card went up (96.8 ms) and the
  readout, with its commit, took 686.0 ms. The lights did not change through
  it or in the 10 s after, where an uncommitted frame would have given way to
  Breathing. `Release` sent nothing (0.0 ms): the frame was already committed.
- **`Release`, then a readout from another process:** the frozen frame stayed,
  and Breathing did not come back (with the first driver build, whose fan
  frames used the wrong layout; the commit is `26 04` and `26 06`, which
  that build had right).
- **R3.3 and R3.4**, with the first driver build: during a `light --yes` run,
  `read --driver nzxt` from another process read status (coolant 43.7 °C) and
  `screen --yes --hold 3s` uploaded (95.1 ms) and returned to the readout
  (3.9 ms). The light run's 1,499 frames were all acknowledged.

Not run: `go test ./...` on Windows. `go vet ./...` passes for windows, linux
and darwin.
