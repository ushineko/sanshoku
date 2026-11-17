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

## Order and names

OpenRGB orders the keys by the firmware's LED index, in one zone named
"Keyboard", and names each key from the VIA keymap (`04`, layer 0) at the key's
row and column (O).
