# 025 — The Kraken's firmware effects

**Issue**: #72

## Status: COMPLETE

## Context

The Kraken 2024 Elite runs lighting effects in its own firmware, at no cost to
the host. Each is one `2A 04` command per channel carrying the effect, its
speed and its colours (`docs/kraken-lighting.md`). Until spec 024 a consumer
reached them only through OpenRGB, whose detector for the cooler is now off.

`lighting.Onboard` (spec 023) is the interface for a firmware effect. It fits
the Kraken in two places badly, and this spec decides how:

- **It reads the running effect back.** No read for the Kraken's effect is
  known. `Effect` reports the effect this handle last set, and is an error
  before it has set one.
- **It assumes the device stores each effect's settings.** The Kraken takes
  speed and colours with every command. The driver keeps them per handle,
  starting from CAM's.

The maintainer chose `Onboard` plus a catalogue: hotaru's spec 071 already
lists and sets `Onboard` effects, and a small optional interface enumerates
each effect's number, speeds and colour count for a consumer that wants more.

Measured from NZXT CAM 4.76.9 on firmware 1.2.12 (the capture spec 024
used), per effect: the speed CAM sends, the colours it sends (byte `0x38`),
and bytes `0x37`, `0x39` and `0x3B`:

| Mode | Effect | Speed | Colours | `0x37` | `0x39` | `0x3B` |
|---|---|---|---|---|---|---|
| `01` | Fading | 40 | 3 | 00 | 08 | 03 |
| `02` | Spectrum Wave | 250 | 0 | 00 | 00 | 03 |
| `04` | Covering Marquee | 150 | 2 | 00 | 00 | 03 |
| `05` | Alternating | 500 | 2 | 01 | 00 | 04 |
| `06` | Pulse | 15 | 1 | 00 | 08 | 03 |
| `07` | Breathing | 20 | 1 | 00 | 08 | 03 |
| `08` | Candle | 50 | 1 | 00 | 00 | 03 |
| `09` | Starry Night | 15 | 1 | 01 | 00 | 03 |
| `0B` | Rainbow Flow | 250 | 0 | 00 | 00 | 03 |
| `0C` | Super Rainbow | 150 | 0 | 00 | 00 | 03 |
| `0D` | Rainbow Pulse | 250 | 0 | 00 | 00 | 03 |
| `0E` | Tai Chi | 30 | 2 | 00 | 05 | 03 |
| `0F` | Water Cooler | 30 | 2 | 00 | 05 | 03 |
| `10` | Loading | 10 | 1 | 00 | 04 | 03 |

Byte `0x3A` is the channel's LED count (`18` for 24). OpenRGB's Hue 2 driver
gives five speeds for most effects and 1 to 8 colours for Fading, Pulse,
Breathing, Covering Marquee and Candle, and 1 to 2 for Alternating.

## Requirements

### R1. The Kraken is `lighting.Onboard`

- R1.1 `Effects` names the 14 effects above in mode order. CAM's names.
- R1.2 `SetEffect(name)` (case-insensitive) sends `2A 04` to each channel
  with keys, with the effect's speed and colours (R1.5), and waits for each
  acknowledgement within 1 s. An unknown name is `lighting.ErrNoEffect`. It
  writes nothing, and says so, when this handle already set that effect with
  the same colours.
- R1.3 `SetColour(c)` sends the running effect again with `c` in every
  colour slot CAM fills for it (three for Fading, two for Tai Chi, one for
  Breathing), and keeps it for later effects. An effect with no colours is
  `lighting.ErrNoColour`. With no effect set on this handle it is
  `lighting.ErrUnknownEffect`.
- R1.4 `Effect` is the effect this handle last set, with its colour where it
  takes one, and `lighting.ErrUnknownEffect` before it has set one. It sends
  nothing.
- R1.5 Each effect's speed is CAM's. Its colours are the colour this handle
  last set through `SetColour`, in every slot, or else CAM's colours.
- R1.6 Setting an effect forgets the canvas's held frame on the channels it
  set, so the commit before a readout (spec 024 R1.13) does not replace the
  effect. A frame drawn afterwards draws over the effect, which the next
  readout brings back (spec 024 E9).
- R1.7 Only `2A 04` is added to what the driver sends (spec 024 R1.11).

### R2. The catalogue

- R2.1 `lighting.Catalogue` is a new optional interface: `Catalogue()
  []lighting.EffectInfo`, each with the effect's `Name`, the device's
  `Number` for it, its `Speeds` from slowest to fastest in the device's own
  units, the `Speed` it runs at, and the fewest and most colours it takes.
- R2.2 The Kraken's speeds are OpenRGB's five where OpenRGB has them, else
  CAM's one. Its colour counts are OpenRGB's ranges where it has them, else
  what CAM sends.
- R2.3 `ErrUnknownEffect` joins `ErrNoColour` and `ErrNoEffect` in
  `lighting`.

### R3. The bench

- R3.1 `sanshoku-bench onboard` lists each effect with the catalogue where a
  device has one, and `--effect` and `--colour` with `--yes` set them on the
  Kraken (the F75's block save and restore stay the F75's).

### R4. Tests

- R4.1 The `2A 04` report for an effect of each shape, byte for byte:
  colours in G, R, B, `0x37` to `0x3B` as the table, the channel twice.
- R4.2 On the fake: `SetEffect` sends one `2A 04` per channel with keys and
  nothing for an unchanged call; `SetColour` fills the slots; the errors of
  R1.2 to R1.4; a frame's held state is forgotten so a readout commits
  nothing.
- R4.3 The guard of spec 024 R4.4 allows `2A 04`.

### R5. Documentation

- R5.1 `docs/kraken-lighting.md`: the table above, from the capture, and
  what the bench confirmed.
- R5.2 `docs/devices.md`: the Kraken gains `onboard`. README changelog.

## Acceptance Criteria

- [x] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries` and `CGO_ENABLED=0 make build` pass.
- [x] Tests per R4.
- [x] Live, with someone watching: `sanshoku-bench onboard --effect` with
      `--yes` runs Breathing, Spectrum Wave, Alternating, Tai Chi and Loading
      on the ring and the fans; `--colour` changes Breathing's colour; a
      `light` frame afterwards draws over the effect, and a readout brings the
      effect back.
- [x] Docs per R5.

## Out of scope

- Marquee and Wave as CAM draws them (a `26 04` buffer moved by `26 06` mode
  `02`): not `2A 04`, and not needed for the 14.
- Choosing a speed or several colours through `Onboard`: the catalogue says
  what exists; a call that sets them is a later spec if a consumer asks.
- The temperature and audio effects.
- Whether the cooler keeps its effect across a power cycle.

## Risks & Assumptions

- **The effect is per handle.** A consumer that opens the cooler afresh has
  no effect until it sets one, and the cooler may still be running the last
  one. hotaru reads `Effect` at attach and logs the error.
- **CAM-only effects** (Tai Chi, Water Cooler, Loading) are known from one
  capture each. The bench checks they run.
- **Rollback:** revert. `Onboard` on the Kraken is additive.

## Verification

### Tests

`nzxt/effects_test.go`: the `2A 04` reports of Breathing, Spectrum Wave and
Alternating byte for byte; the catalogue (14 effects, each running at a speed
it lists); `SetEffect` sends one `2A 04` per channel with keys and nothing
for the same effect again; `Effect` before a set is `ErrUnknownEffect`;
`SetColour` fills Tai Chi's two slots, is `ErrNoColour` on Rainbow Flow, and
the colour carries to the next effect; an effect set over a drawn frame is not
committed over by the readout. The package guard allows `2A 04` and no other
`2A` command.

### Live (2026-11-16, firmware 1.2.12, the maintainer watching)

`sanshoku-bench` from this branch, hotaru stopped, OpenRGB's detector for the
cooler off.

- `onboard --driver nzxt`: the catalogue of 14, and "effect: not reported by
  the device".
- `--effect Breathing --yes` (971 ms, with the accessory list): ring and fans
  breathed purple, CAM's `#390058`.
- `--effect Breathing --colour 00ff80 --yes`: teal-green at the same pace.
- `--effect "Spectrum Wave"`: a rainbow moving on both.
- `--effect Alternating`: purple and teal alternating on both.
- `--effect "Tai Chi"`: red and blue chasing on both.
- `--effect Loading`: a purple that chases and fades around the outside, on
  both.
- With Loading running, a `26 14` frame to each channel from the probe: both
  steady green, the effect stopped. `screen --yes`: after the readout, Loading
  ran again on both.
