# 031 — The K4 HE's own effects

**Issue**: #86

## Status: COMPLETE

## Context

The `keychron` driver (spec 028) draws the K4 HE as a canvas and leaves out
the firmware's own effects. hotaru's switch to sanshoku (hotaru spec 075)
found that this matters. The maintainer's keyboard runs Solid Reactive
Multinexus in a colour, recorded through OpenRGB, and scenes choose VIA
effects. Without them, hotaru drew the keyboard Static.

OpenRGB 1.1's QMK Keychron table names 22 effects, numbered 1 to 22, and
marks which take a colour. The keyboard already answers the commands they
need (`docs/keychron.md`): the effect (`07/08 03 02`), the colour as hue and
saturation (`07/08 03 04`), and the brightness (`07/08 03 01`).

## Requirements

- R1 The canvas is `lighting.Onboard` and `lighting.Catalogue` too. `Effects`
  names the 22 effects as OpenRGB does, in order; `Catalogue` gives each its
  number and, for the 7 that show a colour, one colour. No speeds: the
  firmware keeps one speed for every effect.
- R2 `Effect` reads the running effect and, for a coloured one, the stored
  colour at the board's brightness. The per-key effect (23), or a number off
  the list, is `lighting.ErrUnknownEffect`.
- R3 `SetEffect` sets the effect only where another runs. Frames after it
  claim the keyboard again, recording this effect, which `Release` then puts
  back.
- R4 `SetColour` writes the hue and saturation, and the value as the board's
  brightness, each only where it differs; the effect is kept. Nothing is
  saved (`A8 02`), as OpenRGB saves nothing.
- R5 Docs: `docs/keychron.md`, credits, README; the support entry gains
  `onboard`.

## Acceptance Criteria

- [x] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries` pass.
- [x] Tests on the fake keyboard: the names and catalogue; the running effect
      and its colour read, and the per-key effect unknown; an effect set only
      where it differs, then a frame claims and release gives it back; a
      colour written only where it differs; colour round trip.
- [x] Live, with someone watching, through hotaru: a keyboard recorded in
      Solid Reactive Multinexus comes back in it, and a scene's Solid Reactive
      Multiwide in white shows.
- [x] Docs per R5.

## Risks & Assumptions

- **OpenRGB's table** is right for this firmware (v1.1.1): effect 20 is what
  the keyboard ran and showed as Multinexus.
- **Rollback:** revert; the canvas is unchanged.

## Verification

### Tests

`keychron/effects_test.go` on the fake keyboard, which now stores a colour
(H `3B`, S `86`, as the bench read): 22 names, Solid Color first and Solid
Splash last; Multinexus number 20 with one colour, Cycle All 4 with none; the
running Multinexus read with H `3B` S `86` at brightness 200, and the per-key
effect unknown; Multiwide set, then not set again, Direct refused; a frame
then claims, and release gives back Multiwide; a colour whose hue and
saturation are stored writes only the brightness, then nothing, and red
writes all three; `rgb` undoes `hsv` within 3. `make test`, `lint`,
`check-api`, `check-support` and `check-no-binaries` pass.

### Live (2026-11-17, through hotaru spec 075, OpenRGB stopped, the maintainer watching)

- The keyboard had been left in the per-key effect. hotaru listed the 22
  effects; setting the recorded colour through the keyboard's rule
  (`solid_modes: [Solid Reactive Multinexus, direct]`) wrote "Solid Reactive
  Multinexus" in `#9fc95f`: "there it is, fixed."
- After a restart, hotaru read Multinexus back and left it.
- The scene `custom3`'s Solid Reactive Multiwide in `#ffffff`: "yes, white
  multiwide." Put back to Multinexus after.
