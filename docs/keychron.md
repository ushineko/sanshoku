# Keychron over VIA: the K4 HE

What a Keychron keyboard takes and returns for its lights, over VIA and
Keychron's own RGB commands: the K4 HE's (`3434:0e40`). This page is the
source for the `keychron` driver (spec 028).

## Sources

- **O**: OpenRGB 1.1's QMK Keychron driver
  (`Controllers/QMKController/QMKKeychronController`, tag `release_1.1`,
  commit `8e16de27`). GPL-2.0, so its code is not translated. See
  [credits.md](credits.md).
- **B**: the bench, 2026-11-17, on the K4 HE (firmware `v1.1.1
  2025-06-17`), with someone watching. Recorded in spec 028 and on #77.

## Transport

- VIA's raw HID: usage page `0xFF60`, usage `0x61`, 32-byte reports with a
  report ID of 0 (O+B). Every command is answered by a report that echoes it:
  its first byte, and its second for a Keychron RGB command (`A8`) (O+B).
  Answers came in about 3 ms (B).

| Command | Meaning | On the K4 HE (B) |
|---|---|---|
| `01` | VIA protocol | `0x000C` |
| `A0` | Keychron protocol | 2 |
| `A1` | firmware, ASCII | `v1.1.1 2025-06-17-10:38:42` |
| `A2` | features; bit 7 is Keychron RGB | `0x8F` |
| `A8 01` | RGB protocol | 1 |
| `A8 05` | LED count, after a status byte | 100 |
| `A8 06 <row> FF FF FF` | the LED index at each column of a row, `FF` for none | 6 rows of 21 |
| `08 03 <id>` / `07 03 <id> <value>` | get / set an RGB matrix value: `01` brightness, `02` effect, `03` speed, `04` colour (H, S) | 200, 20, 127, `3B 86` |
| `A8 08 <type>` | the per-key type: `00` solid, `01` breathing, `02`–`04` reactive (O) | 0 |
| `A8 0A <start> <count>` + H S V × 9 | per-key colours, 9 a packet (O+B) | |

## Per-key colours

- **The per-key effect is 23** (O+B). OpenRGB's comment says the number is the
  count of a firmware's standard effects plus two, so another firmware may
  differ (O).
- A key goes as HSV: hue scaled so 256 is a full turn, saturation and value 0
  to 255 (O).
- **The firmware takes the hue and saturation, and ignores the value** (B).
  Keys at value 255, 80 and 0 showed one brightness, and a black key (value 0,
  saturation 0) showed white. This is what looked "hue-only" through OpenRGB.
- **Brightness is one value for the board** (`07 03 01`). 40 is dark and 100
  dim; a sweep of it at 30 steps a second breathed smoothly and went fully
  dark (B). It reads back 199 after 200 is set (B).
- A frame of 100 keys is 12 packets, each answered: about 62 ms. A hue wave
  sent flat out ran 16 frames a second, smoothly (B).
- QMK keeps the effect and colours in memory until the keyboard is replugged;
  `A8 02` saves them, and OpenRGB does not send it (O).

## The keyboard's own effects

The firmware runs 22 effects of its own, numbered 1 to 22 (`07 03 02 <n>`).
The driver names them as OpenRGB names them (O), so a record made through
OpenRGB finds them (spec 031):

| n | Effect | n | Effect |
|---|---|---|---|
| 1 | Solid Color (colour) | 12 | Dual Beacon |
| 2 | Breathing (colour) | 13 | Rainbow Beacon |
| 3 | Band Spiral (colour) | 14 | Jellybean Raindrops |
| 4 | Cycle All | 15 | Pixel Rain |
| 5 | Cycle Left Right | 16 | Typing Heatmap |
| 6 | Cycle Up Down | 17 | Digital Rain |
| 7 | Rainbow Moving Chevron | 18 | Solid Reactive Simple (colour) |
| 8 | Cycle Out In | 19 | Solid Reactive Multiwide (colour) |
| 9 | Cycle Out In Dual | 20 | Solid Reactive Multinexus (colour) |
| 10 | Cycle Pinwheel | 21 | Splash |
| 11 | Cycle Spiral | 22 | Solid Splash (colour) |

- The effects marked "colour" show the stored colour: its hue and saturation
  (`07 03 04 H S`), at the board's brightness (`07 03 01`) (O+B).
- OpenRGB sets the effect, the colour, the brightness and the speed together.
  The driver sets the effect alone, and the colour alone, each only where it
  differs. The speed (`07 03 03`) is one value for every effect, and the
  driver does not set it.
- Nothing is saved: the keyboard keeps the effect until it is replugged (B).

## Speed (spec 032)

The keyboard keeps one speed for every effect (`07 03 03`, read with
`08 03 03`), 0 to 255, at 127 as it came. The catalogue gives each effect that
moves that range; Solid Color, Jellybean Raindrops, Pixel Rain and Typing
Heatmap do not move at a speed. Setting it for one effect sets it for all. The
keyboard's zone is a grid.

## Order and names

OpenRGB orders the keys by the firmware's LED index, in one zone named
"Keyboard", and names each key from the VIA keymap (`04`, layer 0) at the key's
row and column (O).
