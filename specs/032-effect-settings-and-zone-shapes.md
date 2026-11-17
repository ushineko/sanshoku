# 032 — An effect's colours and speed, and a zone's shape

**Issue**: #89

## Status: COMPLETE

## Context

hotaru's spec 077 (ushineko/hotaru#218) makes every supported effect follow
across the devices that can show one, and closes the gaps hotaru's move to
sanshoku found. Three of them are this module's:

- **An effect's colours and speed cannot be set.** `lighting.Onboard` sets an
  effect and one colour. `lighting.Catalogue` lists each effect's speeds and
  colour count (spec 025), and nothing sets them. The Kraken's Tai Chi takes
  two colours and its Fading up to eight; the RTX 4090's Breathing takes two
  and Color Cycle three; and each has a speed. The drivers already build
  their commands from a colour list and a speed: the nzxt driver sends CAM's
  speed and fills every slot but the first with CAM's colours, and the msi
  driver writes its middle speed.
- **The K4 HE's speed** (`07 03 03`, one value for every effect) is not in
  its catalogue (spec 031).
- **A zone has no shape.** A consumer guesses one, and asks a keyboard how many
  things are chained on it. The Kraken knows more: each channel's accessories
  from its slots, so its fans are three of eight lights, not one line of 24.

## Requirements

### R1. The lighting package

- R1.1 `lighting.Tuned` is an `Onboard` and `Catalogue` device whose effects
  take their colours and speed:

  ```go
  type Settings struct {
      Colours []Colour // the effect's colours, first to last; nil keeps them
      Speed   *int     // in the catalogue's units; nil keeps it
  }

  type Tuned interface {
      Onboard
      Catalogue
      SetEffectWith(ctx context.Context, name string, s Settings) (wrote bool, err error)
  }
  ```

  `SetEffectWith` runs the named effect with the colours and speed given,
  and writes only what differs from what the handle set or read.
- R1.2 Colours: fewer than the effect's `MinColours` repeat the last; more
  than its `MaxColours` are `ErrTooManyColours`; any for an effect that takes
  none are `ErrNoColour`. Speed: within the lowest and highest of the effect's
  `Speeds`, else `ErrSpeed`; a device that takes only the listed values uses
  the nearest, and an effect with one speed takes only that.
- R1.3 `lighting.Effect` gains `Colours []Colour` (all of them; `Colour` stays
  the first) and `Speed *int`, where the device or the handle knows them.
- R1.4 `lighting.Zone` gains `Shape` (`ShapeSingle`, `ShapeLine`, `ShapeRing`,
  `ShapeGrid`, `ShapeSet` for lights in no order, and the zero value for not
  known) and `Parts []int`, the lights of each thing chained in the zone,
  first to last, where the device knows them.

### R2. The drivers

- R2.1 **nzxt** is `Tuned`: the colours fill the slots in order, the speed is
  the period sent. It is `Zoned` too: "Ring" and "Fan" (the names consumers
  use, hotaru spec 073), each a ring, with parts from the accessories in the
  channel's slots (Ring `[24]`, Fan `[8 8 8]` on the desk). A channel with
  nothing attached is no zone.
- R2.2 **msi** is `Tuned`: one colour into registers `30`–`32`, two or three
  into the colour blocks; the speed register takes `04`, `02` or `01`, and the
  catalogue lists them slowest first. Its zone is a single light.
- R2.3 **keychron** is `Tuned`: the colour as now, the speed into `07 03 03`
  and read back with `08 03 03`; every effect that moves lists 0 to 255, at
  127 by default. Its zone is a grid.
- R2.4 **asus**: the mainboard zone is a single light; a header's shape is not
  known. **corsair**: Left, Right and Logo are single lights. **logitech**: the
  mouse's zone is a set.

### R3. The bench and the docs

- R3.1 `sanshoku-bench onboard` takes `--colours` (several, comma-separated)
  and `--speed`, through `Tuned`.
- R3.2 `docs/kraken-lighting.md`, `docs/msi-gpu.md`, `docs/keychron.md`,
  `docs/design.md` (Lighting), README, and the API index.

## Acceptance Criteria

- [x] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries`, and `go vet` for windows, linux and darwin
      pass.
- [x] Tests on each driver's fake: colours and speed reach the bytes sent;
      fewer colours repeat, too many and a speed out of range are refused;
      only what differs is written; `Effect` reports the colours and speed;
      the zones' shapes and parts (the Kraken's from its slots).
- [x] Live, with someone watching, through the bench: Tai Chi in two chosen
      colours and Fading in three on the Kraken's ring and fans; a slower and
      a faster Breathing; the RTX 4090's Breathing in two colours at each of
      its speeds; the K4 HE's Solid Reactive Multinexus at a slow and a fast
      speed.
- [x] Docs per R3.2.

## Risks & Assumptions

- **An addition, not a change.** `Tuned` stands beside `Onboard`; `Zone` and
  `Effect` gain fields. A consumer that does not use them is untouched.
- **The K4 HE's speed is one value** for every effect: setting it for one sets
  it for all, which the catalogue says by giving each the same range.
- **Rollback:** revert.

## Verification

### Tests

`make test`, `make lint` (0 issues), `make check-api`, `make check-support`,
`make check-no-binaries`, and `go vet` for windows, linux and darwin pass.

- `lighting/fit_test.go`: fewer colours repeat; too many, colours for a
  colourless effect, and a speed out of range are refused; a speed between
  listed values is taken, or the nearest where asked.
- `nzxt`: Tai Chi's two colours in bytes 7 to 12 and `0x38` = 2; the same
  settings not sent again; Fading's three colours at speed 80, read back by
  `Effect`; a speed alone keeps the colours; refusals; zones Ring `[24]` and
  Fan `[8 8 8]` from the fake's accessory slots (`1E`, `1D`).
- `msi`: Breathing's two colours into blocks `27` and `28` (B G R), speed `01`
  in `38`; the same again sends nothing; speed 3 is the nearest, `04`; colours
  kept; `Effect` reads them back; refusals.
- `keychron`: the speed into `07 03 03` and read back; the same again writes
  nothing; Solid Color takes no speed; Breathing takes one colour.
- Shapes: asus mainboard single and headers unknown, corsair single ×3,
  logitech set, keychron grid, msi single.

### The accessory kits

The Kraken's fan accessory on the desk is `1D`, an F360 RGB Core: three 120 mm
fans of eight LEDs in one slot (OpenRGB's Hue 2 table; NZXT's product page).
`1C` (F280) is two of eight and `1F` (F420) three of eight.

### Live (2026-11-17, hotaru stopped, the maintainer watching)

`sanshoku-bench onboard --yes` from this branch:

1. Kraken, `--effect "Tai Chi" --colours ff00ff,00ff00`: wrote in 837 ms,
   read back `#ff00ff, #00ff00, speed 30`. "yes, magenta and green tai chi on
   both."
2. Kraken, `--effect Fading --colours ff0000,00ff00,0000ff`: 808 ms, speed 40.
   "yes, fading red green blue on both."
3. Kraken, `--effect Breathing --colours ffffff --speed 40`: "slow white
   breathing on both"; then `--speed 4`: "fast purple breathing on both". The
   colour was CAM's again because each bench run is a new handle: a speed
   alone keeps only what the same handle sent.
4. RTX 4090, `--effect Breathing --colours ff0000,0000ff` at `--speed 4`,
   `2` and `1`: 151 to 162 ms each. "yes" (slow), "faster red and blue",
   "fastest red and blue."
5. K4 HE, found on Solid Reactive Multinexus `#9dc85f` at speed 127: `--speed
   30` (17 ms), "slow fade"; `--speed 230`, a quick flash, "yes"; put back to
   127.

hotaru was started again and restored all six devices: "back to tan".
