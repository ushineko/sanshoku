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

## Timing and hold (B)

- A static effect and its commit take about 3.8 ms to write. A breathe on a
  single-colour light was smooth at 33 ms and at 16 ms.
- The controller keeps what it was given with nothing sent, and with every
  handle closed, for minutes at least.

## The Z790 AORUS MASTER X

| Controller | Zone | Kind | Index (O) |
|---|---|---|---|
| 0 | `ARGB_V2_1/ARGB_V2_3` | header 1 | packet `58` |
| 0 | `ARGB_V2_2` | header 2 | packet `59` |
| 0 | `LED_C` | single colour | 4 |
| 1 | `I/O Cover` | single colour | 0 |

On the bench machine `LED_C` drives two chained light strips, one colour for
the whole strip, and nothing is plugged into the addressable headers. So the
header packets have not been seen on this board (B).

## Expected boards (spec 040)

The driver also opens the boards OpenRGB 1.1's table lists on product `5702`,
with the layouts that table gives them (O). None has been benched; a bench
report from one promotes it. Left out are the boards OpenRGB lists twice with
different layouts, because the board's name does not say which revision it
is: A620M DS3H, A620M GAMING X, A620M GAMING X AX, A620M H, A620M S2H, B650M
DS3H, B650M GAMING X AX, B650M K and Z690 AORUS ELITE AX.

317 boards:

- A520 AORUS ELITE
- A520I AC
- A520M AORUS ELITE
- A520M DS3H
- A520M DS3H AC
- A520M H
- A520M S2H
- A620I AX
- A620M C
- B450M DS3H V3
- B450M DS3H WIFI
- B460 AORUS PRO AC
- B460M AORUS ELITE
- B460M AORUS PRO
- B460M DS3H AC
- B460M DS3H V2
- B550 AORUS ELITE
- B550 AORUS ELITE AX
- B550 AORUS ELITE AX V2
- B550 AORUS ELITE AX V3
- B550 AORUS ELITE V2
- B550 AORUS MASTER
- B550 AORUS PRO
- B550 AORUS PRO AC
- B550 AORUS PRO AX
- B550 AORUS PRO V2
- B550 EAGLE
- B550 EAGLE WIFI6
- B550 GAMING X
- B550 GAMING X V2
- B550 UD AC
- B550 UD AC-Y1
- B550 VISION D
- B550 VISION D-P
- B550I AORUS PRO AX
- B550M AORUS ELITE
- B550M AORUS ELITE AX
- B550M AORUS PRO
- B550M AORUS PRO AX
- B550M AORUS PRO-P
- B550M DS3H
- B550M DS3H AC
- B550M DS3H AC R2
- B550M DS3H R2
- B550M GAMING
- B550M GAMING X WIFI6
- B550M H
- B550M S2H
- B560 AORUS PRO AX
- B560 DS3H AC
- B560 HD3
- B560I AORUS PRO AX
- B560M AORUS ELITE
- B560M AORUS PRO
- B560M AORUS PRO AX
- B560M D2V
- B560M D3H
- B560M DS3H
- B560M DS3H AC
- B560M DS3H PLUS
- B560M DS3H V2
- B560M GAMING HD
- B560M H
- B560M POWER
- B650 AERO G
- B650 AORUS ELITE
- B650 AORUS ELITE AX
- B650 AORUS ELITE AX ICE
- B650 AORUS ELITE AX V2
- B650 AORUS ELITE V2
- B650 AORUS PRO AX
- B650 EAGLE
- B650 EAGLE AX
- B650 GAMING X
- B650 GAMING X AX
- B650 GAMING X AX V2
- B650 UD AC
- B650 UD AX
- B650E AORUS ELITE X AX ICE
- B650E AORUS MASTER
- B650E AORUS PRO X USB4
- B650E AORUS TACHYON
- B650I AORUS ULTRA
- B650I AX
- B650M AORUS ELITE
- B650M AORUS ELITE AX
- B650M AORUS ELITE AX ICE
- B650M AORUS PRO AX
- B650M C
- B650M C V2
- B650M C V3
- B650M D2H
- B650M D2HP
- B650M D3HP
- B650M D3HP AX
- B650M GAMING PLUS WIFI
- B650M GAMING WIFI
- B650M GAMING WIFI6E
- B650M H
- B650M S2H
- B660 AORUS ELITE AX DDR4
- B660 AORUS ELITE DDR4
- B660 AORUS MASTER
- B660 AORUS MASTER DDR4
- B660 DS3H AC
- B660 DS3H AC DDR4
- B660 DS3H AX DDR4
- B660 DS3H DDR4
- B660 GAMING X
- B660 GAMING X AX DDR4
- B660 GAMING X DDR4
- B660I AORUS PRO DDR4
- B660M AORUS ELITE AX DDR4
- B660M AORUS ELITE DDR4
- B660M AORUS PRO
- B660M AORUS PRO AX
- B660M AORUS PRO AX DDR4
- B660M AORUS PRO DDR4
- B660M D2H DDR4
- B660M D3H DDR4
- B660M DS3H AX DDR4
- B660M DS3H DDR4
- B660M GAMING AC
- B660M GAMING AC DDR4
- B660M GAMING DDR4
- B660M GAMING X
- B660M GAMING X AX
- B660M GAMING X AX DDR4
- B660M GAMING X DDR4
- B660M POWER DDR4
- B760 AORUS ELITE
- B760 AORUS ELITE AX
- B760 AORUS ELITE AX DDR4
- B760 AORUS ELITE DDR4
- B760 AORUS MASTER DDR4
- B760 DS3H
- B760 DS3H AC
- B760 DS3H AC DDR4
- B760 DS3H AX
- B760 DS3H AX DDR4
- B760 DS3H AX V2
- B760 DS3H DDR4
- B760 GAMING X
- B760 GAMING X AX
- B760 GAMING X AX DDR4
- B760 GAMING X DDR4
- B760I AORUS PRO
- B760I AORUS PRO DDR4
- B760M AORUS ELITE
- B760M AORUS ELITE AX
- B760M AORUS ELITE AX DDR4
- B760M AORUS ELITE DDR4
- B760M AORUS ELITE X AX
- B760M AORUS PRO
- B760M AORUS PRO AX
- B760M AORUS PRO AX DDR4
- B760M AORUS PRO DDR4
- B760M C
- B760M C V2
- B760M D2H
- B760M D2H DDR4
- B760M D3H
- B760M D3H DDR4
- B760M D3HP
- B760M D3HP DDR4
- B760M D3HP WIFI6
- B760M DS3H
- B760M DS3H AX
- B760M DS3H AX DDR4
- B760M DS3H DDR4
- B760M G AX
- B760M GAMING
- B760M GAMING AC
- B760M GAMING AC DDR4
- B760M GAMING DDR4
- B760M GAMING PLUS WIFI DDR4
- B760M GAMING WIFI
- B760M GAMING WIFI PLUS
- B760M GAMING X
- B760M GAMING X AX
- B760M GAMING X AX DDR4
- B760M GAMING X DDR4
- B760M H V2
- B760M POWER
- B760M POWER DDR4
- H470 AORUS PRO AX
- H470 HD3
- H470I AORUS PRO AX
- H470M DS3H
- H610M D3H DDR4
- H610M D3H WIFI DDR4
- H610M D3W
- H610M D3W WIFI6
- H610M GAMING WIFI DDR4
- TRX50 AERO D
- W480 VISION D
- X570S AERO G
- X570S AORUS ELITE
- X570S AORUS ELITE AX
- X570S AORUS MASTER
- X570S AORUS PRO AX
- X570S GAMING X
- X570S UD
- X570SI AORUS PRO AX
- X670 AORUS ELITE AX
- X670 GAMING X AX
- X670 GAMING X AX V2
- X670E AORUS MASTER
- X670E AORUS PRO X
- X670E AORUS XTREME
- Z490 AORUS ELITE
- Z490 AORUS ELITE AC
- Z490 AORUS MASTER
- Z490 AORUS MASTER WATERFORCE
- Z490 AORUS PRO AX
- Z490 AORUS ULTRA
- Z490 AORUS ULTRA G2
- Z490 AORUS XTREME
- Z490 AORUS XTREME WATERFORCE
- Z490 GAMING X
- Z490 GAMING X AX
- Z490 UD
- Z490 UD AC
- Z490 VISION D
- Z490 VISION G
- Z490I AORUS ULTRA
- Z490M
- Z490M GAMING X
- Z590 AORUS ELITE
- Z590 AORUS ELITE AX
- Z590 AORUS MASTER
- Z590 AORUS PRO AX
- Z590 AORUS TACHYON
- Z590 AORUS ULTRA
- Z590 AORUS XTREME
- Z590 AORUS XTREME WATERFORCE
- Z590 D
- Z590 GAMING X
- Z590 UD
- Z590 UD AC
- Z590 VISION D
- Z590 VISION G
- Z590I AORUS ULTRA
- Z590I VISION D
- Z590M
- Z590M GAMING X
- Z690 AERO D
- Z690 AERO G
- Z690 AERO G DDR4
- Z690 AORUS ELITE
- Z690 AORUS ELITE AX DDR4
- Z690 AORUS ELITE AX DDR4 V2
- Z690 AORUS ELITE DDR4
- Z690 AORUS MASTER
- Z690 AORUS PRO
- Z690 AORUS PRO DDR4
- Z690 AORUS TACHYON
- Z690 AORUS ULTRA
- Z690 AORUS XTREME
- Z690 AORUS XTREME WATERFORCE
- Z690 GAMING X
- Z690 GAMING X DDR4
- Z690 GAMING X DDR4 V2
- Z690 UD
- Z690 UD AC
- Z690 UD AX
- Z690 UD AX DDR4
- Z690 UD AX DDR4 V2
- Z690 UD AX V2
- Z690 UD DDR4
- Z690 UD DDR4 V2
- Z690I AORUS ULTRA
- Z690I AORUS ULTRA DDR4
- Z690I AORUS ULTRA LITE
- Z690I AORUS ULTRA LITE DDR4
- Z690I AORUS ULTRA PLUS
- Z690I AORUS ULTRA PLUS DDR4
- Z690M AORUS ELITE AX DDR4
- Z690M AORUS ELITE DDR4
- Z690M DS3H DDR4
- Z790 AERO G
- Z790 AORUS ELITE
- Z790 AORUS ELITE AX
- Z790 AORUS ELITE AX DDR4
- Z790 AORUS ELITE AX ICE
- Z790 AORUS ELITE AX-W
- Z790 AORUS ELITE DDR4
- Z790 AORUS ELITE X
- Z790 AORUS ELITE X AX
- Z790 AORUS ELITE X WIFI7
- Z790 AORUS MASTER
- Z790 AORUS PRO X
- Z790 AORUS PRO X WIFI7
- Z790 AORUS TACHYON
- Z790 AORUS TACHYON X
- Z790 AORUS XTREME
- Z790 AORUS XTREME X
- Z790 AORUS XTREME X ICE
- Z790 D
- Z790 D AC
- Z790 D AX
- Z790 D DDR4
- Z790 D WIFI
- Z790 EAGLE
- Z790 EAGLE AX
- Z790 GAMING PLUS AX
- Z790 GAMING X
- Z790 GAMING X AX
- Z790 S DDR4
- Z790 S WIFI DDR4
- Z790 UD
- Z790 UD AC
- Z790 UD AX
- Z790I AORUS ULTRA
- Z790M AORUS ELITE
- Z790M AORUS ELITE AX
- Z790M AORUS ELITE AX ICE
