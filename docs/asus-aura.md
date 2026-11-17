# ASUS Aura USB: the mainboard controller

What an ASUS Aura USB mainboard controller takes and returns for its lights:
the ROG Maximus Z790 Hero's (`0b05:18f3`, "AURA LED Controller"). This page
is the source for the `asus` driver (spec 027).

## Sources

- **O**: OpenRGB 1.1's Aura USB driver
  (`Controllers/AsusAuraUSBController/AsusAuraUSBController`, tag
  `release_1.1`, commit `8e16de27`). GPL-2.0, so its code is not translated.
  See [credits.md](credits.md).
- **B**: the bench, 2026-11-17, on the Z790 Hero: raw reports to its hidraw
  node, and the `asus` driver, recorded in spec 027's Verification.

## Transport

- One HID collection on vendor page `0xFF72`, report ID `EC` in both
  directions, 64 bytes after the ID (B, from the report descriptor). OpenRGB
  matches the product alone (O).
- Questions are answered with a report whose first byte after the ID says
  what it is. Lighting commands are not answered (O+B).

## Questions

| Ask | Reply | Meaning |
|---|---|---|
| `EC 82` | `EC 02` + 16 ASCII bytes | the firmware's version; `AULA3-6K75-0309` on the Hero (O+B) |
| `EC B0` | `EC 30 xx xx` + 60 bytes | the configuration table (O+B) |

The table's bytes that OpenRGB uses (O), and the Hero's values (B):

| Byte | Meaning | Hero |
|---|---|---|
| `02` | addressable (ARGB) headers | 4 |
| `1B` | the board's own lights | 1 |
| `1D` | how many of those are 12 V RGB headers | 3 |

OpenRGB treats a board with fewer lights than RGB headers as having no RGB
headers (O), so on the Hero the board's lights are one light, and its three
12 V headers are not addressed. What `1D` = 3 with `1B` = 1 means is not
known.

After the table OpenRGB sends `EC 52 53 00 01` once, and its source calls it
only a custom command (O). Its effect is not known.

## Channels

| Channel | Effect channel | Direct channel | Lights |
|---|---|---|---|
| the board's own | 0 | `04` | `1B` |
| ARGB header *i* (from 1) | *i* | *i* − 1 | set by the consumer |

**A header's length is not the board's.** Nothing is sent that says how many
LEDs a strip has: a frame is however many lights are written (O). OpenRGB
keeps the length in its own profile, from 0 to 500 (O).

## Direct frames

- Before a channel's first frame, its effect is set to direct:
  `EC 35 <effect channel> 00 00 FF` (O).
- A frame: `EC 40 <direct channel> <offset> <count>` and up to 20 lights in R,
  G, B. The last packet of a channel ORs `80` (apply) into the channel byte
  (O). Past light 255, `10` is ORed in and the offset counts from 256 (O).
- Each channel is written and applied apart. No reply (O).

## Firmware effects

`EC 35 <effect channel> 00 <shutdown> <mode>` sets an effect, and `EC 36`
with a light mask its colour; modes 0 (off) to 19 (red pulse) (O). Saving to
the board's memory is `EC 3F 55`, which OpenRGB sends only when asked (O).
This module sends neither: hotaru draws the board itself (hotaru #212).
