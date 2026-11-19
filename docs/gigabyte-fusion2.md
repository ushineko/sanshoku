# Gigabyte RGB Fusion 2 USB: the mainboard controllers

What a Gigabyte RGB Fusion 2 USB mainboard controller takes and returns for
its lights: the Z790 AORUS MASTER X's two (`048d:5702`, ITE). This page is the
source for the `gigabyte` driver (spec 039).

## Sources

- **O**: OpenRGB 1.1's Fusion 2 USB driver
  (`Controllers/GigabyteRGBFusion2USBController`, tag `release_1.1`). GPL, so
  its code is not translated. See [credits.md](credits.md).
- **B**: the bench, on the Z790 AORUS MASTER X, recorded in spec 039's
  Verification.

## Transport

- One HID collection on vendor page `0xFF89`, with two feature reports: `5A`
  (16 bytes) and `CC` (63 bytes after the ID). No input or output report (B,
  from the report descriptor).
- Every command is report `CC`, 64 bytes with the ID, sent as a feature
  report. A question is a command followed by a read of feature report `CC`
  (O).
- The board can carry two controllers, each its own USB device. On the Z790
  AORUS MASTER X one is an IT5701 and the other an IT5702, both product
  `5702` (B).

## The report

`CC 60`, then a read of `CC`, returns the controller's report (O). The bytes
the driver uses, counting the ID as byte 0:

| Byte | Meaning | Z790 AORUS MASTER X (B) |
|---|---|---|
| `02` | the controller's number on the board | 0 and 1 |
| `0B` | flags: `01` Gen2 strips, `02` lamp array | 0 on both |
| `0C`–`27` | the product string | `IT5701-GIGABYTE V7.0.18.1`, `IT5702-Z490XE V1.11.1.0` |
| `2C`–`2F` | header 1's colour order | GRB on both |
| `30`–`33` | header 2's colour order | GRB on controller 0 |
| `38`–`3B` | the chip ID | `0x57010100`, `0x57020100` |

A colour order is a little-endian word: blue's position in the low byte, then
green's, then red's (O). Zero means none was set.

**The report does not say which lights the board has.** OpenRGB keeps each
board's layout in a table, keyed by the board's DMI name and the controller's
number (O). The driver keeps a table too, of boards that have been measured.

## Start

What OpenRGB sends after the report, and the driver with it (O):

- `CC 48 00`, lamp array off, where flag `02` is set.
- `CC 20 00 00` to `CC 27 00 00`, clearing the effect registers, then
  `CC 28 FF 00`, committing them.
- `CC 31 00`, beat (sound-reactive) mode off.

## Addressable headers

- `CC 34 <h2 h1> <h4 h3> <h6 h5>`: each header's length class in four bits,
  0 for up to 32 lights, 1 for 64, 2 for 256, 3 for 512, 4 for 1024. IT57xx
  controllers take at most 256 (O).
- `CC 32 <mask>`: turns a header's own effect off where its bit is set,
  header 1 `01` and header 2 `02` (O).
- A frame: `CC <header> <offset, 2 bytes> <count>` and up to 19 lights, three
  bytes each, in the header's colour order. Header 1 is `58` and header 2
  `59`. The offset and count are in bytes, not lights (O).

## Single-colour lights

A single-colour light is given a static effect in one colour, in its effect
register `20` + its index (O):

| Byte after the ID | Meaning |
|---|---|
| `00` | the register, `20` + the light's index |
| `01`–`04` | the light's bit in the zone mask |
| `0A` | the effect, `01` static |
| `0B` | the most brightness, `FF` |
| `0D`–`0F` | the colour, blue, green, red |

`CC 28 <mask, 4 bytes>` then commits the registers named by the mask (O).

## Commands never sent

OpenRGB has these behind settings that are off by default (O). The driver
refuses to send them:

| Command | What it does |
|---|---|
| `CC 33` | writes the colour-order calibration |
| `CC 38`–`CC 3D` | scans a header for Gen2 strips, and rewrites the length classes |
| `CC 47` | turns persistent lighting on |
| `CC 5E` | saves the lighting to the board's flash |

## The Z790 AORUS MASTER X

| Controller | Zone | Kind | Index (O) |
|---|---|---|---|
| 0 | `ARGB_V2_1/ARGB_V2_3` | header 1 | packet `58` |
| 0 | `ARGB_V2_2` | header 2 | packet `59` |
| 0 | `LED_C` | single colour | 4 |
| 1 | `I/O Cover` | single colour | 0 |
