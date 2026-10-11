# 019 — AULA F75 lighting by frames

**Issue**: #52

## Status: INCOMPLETE

## Context

The `aula` driver reads the F75's battery through its 2.4 GHz receiver
(`3554:fa09`, spec 013) and sends nothing else. Nothing open lights the
keyboard: OpenRGB 1.0.0 does not list it, and the vendor's program is the only
thing that does. hotaru's Windows port is being written at a desk whose only
candidate for a `lighting.Canvas` is this keyboard.

Two sources describe the receiver's lighting:

- OpenRGB issue 5166, for the AULA F99 on the same receiver, with captures of
  the vendor's program: command `0x88` "sends LED color information", in a
  mode for one colour and a mode for colours by key, and a model query `0x05`
  whose reply names the F99 as `0xa4` and the F75 as `0xcd`.
- deepan-alve/womier-l65-linux (MIT), for the Womier L65 on the same receiver:
  the config block (read `0x44`, write `0x04`), the palette (read `0x49`,
  write `0x09`), both stored in the keyboard's flash, and the statement that
  the receiver carries no per-key payload.

They disagree about per-key colour, so it was measured. A throwaway prototype
ran on 2026-10-10 on Windows 11 with the plain HID class driver, the keyboard
on battery at 69 %, with the user watching it. It is not in any repository.

Every packet is the 20-byte report `0x13` spec 013 describes: the command, the
number of packets in the message, this packet's index, a byte whose high
nibble is a mode and whose low nibble is the count of body bytes, 14 body
bytes, and the byte sum.

| Sent | Seen |
|---|---|
| `0x05`, empty | `13 05 01 00 0a 03 00 00 00 00 cd 01 …` in 9 ms: model `0xcd` |
| `0x44`, empty | 128 bytes in 10 packets, 47 ms, ending `5a a5` |
| `0x49`, empty | 490 bytes in 35 packets, 144 ms |
| `0x88` mode 2, `ff 00 00`, once | part of the left side red for a moment, then the firmware's effect again |
| the same, 90 times at 62 ms, red then green then blue | the whole board in each colour, then the effect again |
| `0x88` mode 1, `R G B count slot…` groups, 80 times | W, A, S, D red (slots 14, 9, 15, 21), U blue (44), Esc and backtick green (0, 1); every other key dark |
| 16 groups of 6 slots, a hue per column, moving, 100 frames | smooth on the whole board; 160 bytes, 12 packets, 36.6 ms a frame mean, 60.5 ms worst |
| 96 groups of 1 slot, 480 bytes, 35 packets | the left half lit, ending before the column of 8, I, K and comma; the rest dark |
| the same frame as two messages of 240 bytes | the whole board, flickering |

What that says:

- A slot is column × 6 + row, and row 0 is the F row. The L65's table in
  womier-l65-linux uses the same numbering one row down.
- A message is a whole frame. Keys it does not name are dark, and a second
  message replaces the first, so a frame cannot be split.
- A message holds about 240 bytes: 48 one-key groups showed and the 49th did
  not. Whether the limit is bytes, packets or groups is not known.
- Nothing is acknowledged. The write returns in about 3 ms a packet.
- Nothing is stored. When frames stop the firmware's effect is back within a
  second or two, and the config block and palette read back byte for byte as
  they were before the session.
- Typing was unaffected while frames ran.
- The statement that the receiver carries no per-key payload does not hold for
  the F75.

The keyboard on its cable is another device, `258a:010c`, a Sino Wealth chip
that calls itself "BY Tech Gaming Keyboard". A second prototype measured it
the same day, again with the user watching. Its vendor collection (page
`0xFF00`) has feature report 6, 520 bytes, which womier-l65-linux documents:
`06 command index 00 01 00 length-low length-high data`. Report 5 on the same
collection is the chip's bootloader channel.

| Sent on report 6 | Seen |
|---|---|
| `0x82` index 0, then the report read back | `00 10 00 00 00 00`: not a model |
| `0x84`, then read back | the 128-byte config block, the same as through the receiver but for byte `0x0E` |
| `0x83`, then read back | the key matrix: the key at each of 126 slots |
| `0x08`, 378 bytes of red, once | red, then the firmware's effect again |
| `0x08`, a colour for every slot, moving, 150 frames | the whole board, smooth; 66 ms a frame to write, 77 ms worst |
| `0x08`, blue, one frame every 500 ms for 6 s | steady blue, no flicker |

So the cable has no limit of colours, costs 66 ms a frame whatever is in it,
and fades as the receiver's does. Typing was unaffected and the config block
was unchanged afterwards. The key matrix names the key at every slot, which is
where the key table comes from: 80 keys, with slots 75 and 76 holding the two
keys only an ISO board has and slot 84 the knob's press.

After those runs the keyboard was switched back to 2.4 GHz for other work, so
one thing about the cable is read and not measured: OpenRGB's draft for this
chip (merge request 3062) asks the model with `0x82` and index 1 and reads it
from the sixth data byte. Index 0 was what the prototype sent.

## Requirements

### R1. The F75 as a canvas

- R1.1 The `aula` driver's device for product `0xFA09` satisfies
  `lighting.Canvas` on the same open handle as its battery, and
  `sanshoku.Capabilities` reports `"lighting"` for it. The driver's
  `Describe` and support entry gain the capability.
- R1.2 The receiver's product id is not the keyboard's: the F99 and the Womier
  L65 enumerate as `3554:fa09` too. Before the first frame the driver asks the
  model (`0x05`) once per open handle. A reply naming `0xcd` is an F75. Any
  other model is `lighting.ErrNoCanvas` wrapped with what was read, kept for
  the life of the handle, and no frame is sent. No reply is
  `hidraw.ErrSilent`, no frame is sent, and the question is asked again at the
  next frame: a keyboard that is asleep is not written off. The battery is
  unaffected either way.
- R1.3 `Keys` is the F75's 80 lights by slot, named for the key the keyboard's
  own matrix puts there, in slot order. Slots 75, 76 and 84, and 96 to 125,
  are left out: E1 lit them and nothing showed.
- R1.4 `Frame` keeps what it last showed. The interface says a light not in
  `px` is left as it was, and the keyboard darkens what a message does not
  name, so each call merges `px` into the held frame and sends the whole
  board. A `Pixel` whose `ID` is not in `Keys` is an error and nothing is
  sent. The Apex sends an unknown id and the keyboard ignores it (spec 010);
  here every slot costs a byte of a message that has few to spare.
- R1.5 A frame is one message of command `0x88`, mode 1: for each colour, `R G
  B count` and that many slots, in ascending order of the colour's first slot,
  so the same frame is always the same bytes. A light that is black is left
  out of the message, which is how the keyboard shows black. A frame that is
  black everywhere is sent as mode 2 with `00 00 00`.
- R1.6 A frame that does not fit one message is reduced to one that does, and
  is not refused: the two nearest colours are merged, weighted by how many
  keys each lights, until the message fits: 228 bytes, which E2 measured as the
  largest message shown whole. Encoding and reduction are pure
  functions of the held frame. The number of colours that fit with every key
  lit is exported as `aula.ReceiverColours` (37), so a consumer that wants its
  frame shown as drawn can stay under it.
- R1.7 Nothing is read after a frame, because nothing answers. A write that
  fails is the error. A keyboard that is asleep or out of range takes the
  write and shows nothing, and `Frame` cannot tell; the battery question is
  how a consumer asks whether the keyboard is there.
- R1.8 `Floor` is 80 ms, which E3 measured: a moving frame at the message
  limit looked even at 80 ms and rough at 60 ms.
- R1.9 `Release` sends nothing and forgets the held frame: the firmware takes
  the lights back by itself when frames stop (E4 says how soon). It never
  fails on an open device.
- R1.10 The driver sends three commands to this receiver and no others: the
  battery question (`0x4A`), the model question (`0x05`) and the frame
  (`0x88`). It never sends the config block or the palette (`0x04`, `0x09`),
  which write the keyboard's flash, nor their reads.

### R1b. The F75 on its cable

- R1b.1 `Find` also returns the keyboard on its cable: vendor `258a`, product
  `010c` only, declaring report 6 on page `0xFF00`. The vendor id is a chip
  maker's, so no other product of it is a candidate. The device is a
  `lighting.Canvas` and a `lighting.Fading` and has no battery reading.
- R1b.2 Before the first frame the keyboard is asked its model on report 6
  (`0x82`, index 1) and the report is read back after 50 ms. An answer that
  echoes the command and names `0xcd` is an F75. Another model is
  `lighting.ErrNoCanvas`; an answer that does not echo the command is an
  error and is asked again at the next frame. E9 measures this on the F75.
- R1b.3 A frame is one feature report, command `0x08`: a colour at three times
  each slot, black on a slot with no key. `Keys`, the held frame, the refusal
  of an unknown slot, `Floor`, `Hold` and `Release` are the receiver's.
- R1b.4 Two commands are sent on report 6 and no others: `0x82` and `0x08`.
  Report 5 is never sent.
- R1b.5 With both devices present the consumer has two candidates and chooses.
  The keyboard shows frames from the route its switch is set to.

### R2. A frame that fades

- R2.1 On the Apex a frame stays until the next. Here it lasts until the
  firmware takes the lights back, so a still scene has to be sent again. A new
  optional interface in `lighting`:

  ```go
  // Fading is a Canvas whose frame does not last: left alone, the device
  // goes back to its own lighting.
  type Fading interface {
      // Hold is how long a frame is shown before the device takes its
      // lights back. A consumer holding a still frame sends it again
      // sooner than this.
      Hold() time.Duration
  }
  ```

  The F75's canvases implement it as 500 ms. E4 measured a frame lasting
  between 1 s and 1.5 s through the receiver and more than 1.5 s on the cable,
  so the margin is a factor of two on the shorter. `Canvas` itself does not change, so the Apex's driver and
  every consumer compile as they are.
- R2.2 The library starts no goroutine to keep a frame up. When to send again
  is the consumer's clock, as every other pace in this module is.

### R3. The bench

- R3.1 `sanshoku-bench light --yes` runs on the F75 as it does on the Apex.
  Every pattern, steady included, sends a frame each interval, which keeps a
  fading canvas up. For a `Fading` canvas the bench prints its `Hold`.
- R3.2 `sanshoku-bench light --keys` lists the F75's slots and names.
- R3.3 `sanshoku-bench read --driver aula` reads the battery on the same
  handle before, during and after a `light` run.

### R4. Tests

- R4.1 Encoding, as pure functions: the vendor's captured red packet byte for
  byte; the frame that was watched (W, A, S, D, U, Escape, backtick) byte for
  byte with its checksums; packets of 14 body bytes with the count and index set;
  groups in first-slot order; black left out; all black as mode 2.
- R4.2 Reduction: a frame under the limit is unchanged; a frame of one colour
  for each key comes out at the limit, every key still lit; the same input
  gives the same bytes.
- R4.3 On the fake node: the model is asked once per handle and not again; a
  model other than `0xcd` is `ErrNoCanvas` and no `0x88` is written; silence
  to the model question is the same; a second `Frame` naming one key resends
  the first frame's keys with it; `Release` forgets them; an unknown `ID`
  writes nothing.
- R4.4 A guard: across every test in the package, the fake receiver has been
  written no command but `0x4A`, `0x05` and `0x88`, and the fake cable no
  report but 6 and no command but `0x82` and `0x08`.
- R4.6 The cable on its fake: found by product and report; the model asked
  once with index 1; a frame is one feature report and keeps the last one's
  keys; another model is `ErrNoCanvas`; an answer that is no answer sends no
  frame.
- R4.5 `make test` still opens no device.

### R5. Documentation

- R5.1 `docs/devices.md` (generated) shows the F75 with `battery, lighting`
  through its receiver and a second row for the cable. `docs/udev.md` and
  `packaging/60-sanshoku.rules` gain a rule for `258a:010c`, by product.
- R5.2 `docs/credits.md`: OpenRGB issue 5166 and its captures for the frame
  command and the model ids, and womier-l65-linux for the slot numbering and
  for which commands write flash.
- R5.3 `docs/design.md`: `lighting.Fading` beside `Canvas`, and a paragraph on
  a canvas that reduces a frame.
- R5.4 README: the F75's row, and a changelog entry under `### Unreleased`
  marked as new API. `docs/api.md` regenerated.

### R6. Experiments, run on the desk before the code is final

Each is one bench run with the user watching, recorded in Verification. Every
one of them sends only the frame command.

- E1 **The slots.** One slot lit at a time, 0 to 95, a second each: which key
  shows, which slots light nothing, and whether the knob, a side light or
  anything off the key matrix answers to a slot. Then slots 96 to 125, since
  the L65's table has 126. This fixes R1.3.
- E2 **The limit.** One-key groups, 44 to 52 of them, then two-key and six-key
  groups whose byte length crosses 238, 240, 252 and 256: whether the limit is
  bytes, packets or groups, and its exact value. This fixes R1.6.
- E3 **The floor.** A frame at the limit, moving, at 30, 50, 80 and 120 ms
  intervals, 30 s each: the shortest at which nothing tears, stalls or drops.
  Then the same with the user typing a paragraph, and with the mouse on the
  same hub. This fixes R1.8.
- E4 **The hold.** Steady blue for 10 s, then stop: the time until the
  firmware's effect returns, taken five times. Then one frame sent every 250,
  500 and 1000 ms: the longest at which the board never flickers back. This
  fixes R2.1. It also explains the first prototype run, where one message
  showed on part of the board only.
- E5 **Sleep.** Steady blue held by resending for longer than the keyboard's
  sleep timer with no key pressed: does it sleep under a stream, and what does
  the next frame after a keypress show. Written into the support entry.
- E6 **Battery.** The level by `0x4A` before and after 30 minutes of a steady
  frame held at E4's interval, and before and after 30 minutes of a moving
  one at the floor. Written into the `Fading` doc comment's neighbour on the
  canvas and into `docs/devices.md`.
- E7 **Brightness.** The keyboard's own brightness keys (Fn and the arrows)
  while a frame is up: whether they scale it, do nothing, or break the stream.
- E9 **The cable's model.** `sanshoku-bench light --yes` on the cable: the
  model question with index 1 answers `0xcd` and frames follow. If it does
  not, R1b.2 changes to what does. Then E3 and E4 on the cable.
- E8 **The cable in, receiver still plugged.** With the keyboard charging on
  its cable and switched to 2.4 GHz, and then switched to USB: whether frames
  through the receiver still show.

## Acceptance Criteria

- [ ] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries` and `CGO_ENABLED=0 make build` pass on Linux
      (CI).
- [x] `go test ./...` passes on Windows, and the pinned golangci-lint reports
      0 issues on Windows and with `GOOS=linux`.
- [x] `go vet ./...` passes for windows, linux and darwin.
- [ ] E1 to E9 run on Windows with the user watching, their outcome written
      into Verification, and R1.3, R1.6, R1.8 and R2.1 amended to what was
      measured.
- [x] `sanshoku-bench light --yes` shows steady, breathe and wave on the F75
      through its receiver, and again on its cable, and the firmware's effect
      returns afterwards. This
      is the integration-boundary criterion: a real keyboard, not a fake.
- [x] The keyboard's config block and palette, read by the prototype before
      and after that run, are byte for byte the same.
- [ ] `sanshoku-bench read --driver aula` reads the battery before, during and
      after a `light` run.
- [x] Falsified: with the model check removed, the test of a model other than
      `0xcd` fails; restored.
- [x] `sanshoku-bench light --yes` on the Apex is unchanged, or is recorded as
      not run with the reason (the Apex is on another desk).
- [x] Docs per R5, including the changelog entry.

## Out of scope

- **Effects, brightness and the palette.** They are the config block, which is
  flash. A `Canvas` has a frame and nothing else.
- **The F99 and the L65.** The same receiver and probably the same command.
  Neither is on a desk; R1.2 is what keeps frames from them until one is.
- **hotaru.** Drawing on it, sending a still scene again inside `Hold`, and
  what its cost rule says about that, are a hotaru spec.

## Risks & Assumptions

- **A still scene is not free on this keyboard.** On the Apex it is one frame
  and then nothing; here a held colour is a packet or more every `Hold`, all
  day, on a battery. That is accepted for this device (decided with the user,
  2026-10-10: every device is different), and hotaru's cost rule now says a
  still scene sends the least its device allows. E4 and E6 put the numbers on
  it, and they go into the support entry.
- **No acknowledgement.** A frame lost in the air is not known to be lost. A
  moving effect covers it with the next frame; a still one covers it at the
  next resend. The driver cannot report a frame as shown, only as written.
- **The limit coarsens a frame.** About 36 colours on the board at once. A
  gradient by column fits; a frame with a colour for each key is merged
  toward its nearest neighbours. R1.6 makes that the same every time and R3.1
  shows it.
- **A candidate is named before it is asked.** Both ids are named "AULA F75"
  when found, as the receiver has been since spec 013, and another keyboard
  on the same id carries that name until a frame is refused. Asking the model
  in `Find` would open a device, which `Find` never does.
- **Linux is unmeasured.** The same reports through hidraw, as the battery is.
  The support entry stays Expected there until a bench reads it.
- **Another program.** The vendor's program streams on the same collection.
  Two streamers alternate frames. This module cannot arbitrate; the support
  entry says so.
- **Wear.** Nothing here writes flash, R1.10 forbids the commands that do,
  and R4.4 holds the driver to it.
- **Rollback**: revert. The capability and `Fading` are additive; nothing
  consumes them until hotaru's spec does.

## Alternatives Considered

- **A goroutine in the driver that keeps the frame up.** It would make the F75
  behave like the Apex and leave `Canvas` alone. Rejected: the library would
  write to a device on its own clock after `Frame` returned, with errors
  nobody receives, and a consumer could not account for the cost. `Fading`
  tells the consumer the one thing it needs and leaves the clock with it.
- **`Hold` as a method of `Canvas`.** Every canvas would answer, the Apex with
  zero. Rejected for now: it breaks the Apex's driver and hotaru's fakes for
  one device. If a third canvas fades, the method moves in.
- **Refusing a frame over the limit.** Simpler and exact. Rejected: every
  consumer would write its own reduction, and hotaru's effects do not know
  the limit when they render.
- **Splitting a frame across messages.** Measured: each message replaces the
  frame, and the board flickers.
- **Setting a still colour through the config block and palette.** One write,
  no stream, no battery cost. Rejected: both are flash writes, they change
  what the keyboard does when nothing is driving it, and hotaru's inert rule
  is that what was not asked for is not changed.
- **Offering the canvas by product id alone.** Rejected: `3554:fa09` is three
  keyboards with different layouts, and `258a:010c` the same three.
- **The cable as a spec of its own.** It is the same keyboard, key table,
  held frame and fading, in the same driver, so it is here. Its tier is its
  own row, and stays Expected until its bench run.

## Verification

2026-10-10, Windows 11 Pro 26200, Go 1.27.0 (lint under Go 1.26.0). The
prototypes' measurements are in Context. Of the driver itself:

- `go test -count=1 ./...`: every package ok, with nothing but fakes.
- `go vet ./...` for windows, linux and darwin: clean.
- golangci-lint v2.12.2: 0 issues on Windows and with `GOOS=linux`.
- Falsified: with the model check made always to pass, the two tests of
  another keyboard (receiver and cable) fail; restored, they pass.
- `sanshoku-bench scan` lists the receiver as `battery,lighting`, and
  `sanshoku-bench light --keys` lists its 80 keys, floor 80 ms, "a frame fades
  after 500ms". Neither writes anything.

Later the same day, through the receiver, with the keyboard on its cable for
charge and switched to 2.4 GHz (`258a:010c` not present):

- `sanshoku-bench light --yes`: the model question answered `0xcd` and every
  frame was written.

  ```
  steady   every 80ms: 63 frames, 63 acknowledged, 0 timed out; ack p50 16.6 ms, p95 17.1 ms, max 40.1 ms
  breathe  every 80ms: 63 frames, 63 acknowledged, 0 timed out; ack p50 16.5 ms, p95 18.4 ms, max 27.0 ms
  wave     every 80ms: 63 frames, 63 acknowledged, 0 timed out; ack p50 52.9 ms, p95 77.1 ms, max 105.2 ms
  release: ok (0.0 ms)
  ```

  "Acknowledged" here is a write the system accepted: the keyboard does not
  answer a frame. A one-colour frame costs 17 ms and a 40-colour one 53 ms.
- The config block and the palette, read by the prototype afterwards, are
  byte for byte what they were before the driver existed.
- The battery read either side of the run reported no level, as it does while
  the cable is in.

The user watched that run and the ones below and said what the board did.
The first run showed all three patterns on the whole board.

- **E1, the slots.** With every key lit no key was dark. A frame naming only
  slots 75, 76, 84 and 96 to 125 lit nothing: no key, knob or side light.
  The slot walk was not run, since the keyboard's own matrix names each slot.
- **E2, the limit.** A still frame, the board blue in 35 to 39 groups and the
  last four keys (Page Up, Page Down, End, Right) red as the last group, sent
  every 250 ms:

  | Bytes | Packets | The last four keys |
  |---|---|---|
  | 224 | 16 | steady |
  | 228 | 17 | steady |
  | 232 | 17 | End and Right flickering, the other two steady |
  | 236 | 17 | all four flickering |
  | 240 | 18 | all four flickering |

  So the keyboard keeps about 230 bytes, counted in bytes and not packets.
  The limit was 240 from the prototype, and at 240 the moving pattern
  flickered on the last column. It is now 228, 37 colours with every key lit,
  and the same pattern showed the whole board evenly.
- **E3, the floor.** The moving pattern at the limit: even at 120 ms and at
  80 ms, rough at 60 ms. Writes took 50 ms at the median, 65 to 69 ms at the
  95th percentile and 116 ms at worst. In one 80 ms run, one frame of 123
  returned an error that was not recorded; no other run had one. The runs
  were 10 s, not 30 s, and typing and the mouse on the same hub were not
  tried.
- **E4, the hold.** A colour sent every 250 ms, 500 ms and 1 s stayed solid;
  every 1.5 s it flickered. The time for the effect to return was not taken
  with a clock.
- **E7, brightness.** With a frame held, Fn with the up and down arrows did
  nothing to it.
- **E8, the cable in.** All of the above was with the cable in for charge and
  the switch at 2.4 GHz, so frames through the receiver show in that state.
  With the switch at USB the receiver does not answer the model question and
  the driver sends no frame through it: 17 of 17 frames refused per pattern.
- **E9, the cable.** `sanshoku-bench light --yes` with the switch at USB: the
  model question with index 1 answered `03 00 00 00 00 cd`, and the user saw
  all three patterns on the whole board.

  ```
  steady   every 80ms: 62 frames, 62 acknowledged, 0 timed out; ack p50 66.4 ms, p95 73.7 ms, max 197.1 ms
  breathe  every 80ms: 63 frames, 63 acknowledged, 0 timed out; ack p50 65.9 ms, p95 70.3 ms, max 73.0 ms
  wave     every 80ms: 63 frames, 63 acknowledged, 0 timed out; ack p50 65.7 ms, p95 77.2 ms, max 79.7 ms
  ```

  On the cable a colour sent every 1 s and every 1.5 s both stayed solid. The
  config block read over the cable afterwards is what it was before.

Not run:

- **E5, sleep, and E6, battery.** Each needs the keyboard off its cable and
  left alone, for the sleep timer and for two runs of 30 minutes.
- The battery read while a `light` run is going. It was read before and after.
- The Apex, which is on another desk.
- Anything on Linux.
