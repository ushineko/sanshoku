# MSI mainboards: the Super I/O RGB header

The one RGB header that some MSI mainboards drive from their Super I/O chip,
a Nuvoton NCT6795D or NCT6797D: the B450 Tomahawk Max (MS-7C02) first. This
page is the source for `msi.Board` (spec 045). These boards have no USB
lighting controller; MSI's later boards use Mystic Light over USB, which this
driver does not speak.

## Sources

- **O**: OpenRGB 1.1's MSI-RGB controller (`Controllers/MSIRGBController`,
  `super_io/super_io.h`, tag `release_1.1`). GPL, so its code is not
  translated. See [credits.md](credits.md).
- **B**: the bench, recorded in spec 045's Verification.

## Finding the board

- The DMI maker is MSI (`Micro-Star International Co., Ltd.` or `MSI`), and
  the DMI board name carries one of 50 MS- numbers OpenRGB lists. On 21 of
  them the header's colours are inverted (O). Named `MSI ` and the DMI name.
- The chip answers at port `0x2E` or `0x4E`: its device ID (registers `20`,
  `21`), masked with `FFF8`, is `D350` (NCT6795D) or `D450` (NCT6797D) (O).

## Readying the chip

OpenRGB does this once (O); the driver does it before its first frame:

1. Logical device `09`, register `2C`: clear bit 3 and set bit 4 (OpenRGB:
   without it, pulsing does not work).
2. Logical device `12`, the RGB controller: if `E0`'s top three bits are not
   all set, write `E0`.
3. `E4` = `00`: no pulse or blink.
4. `FF` = `E2`, or `FE` on a board with inverted colours: the header on.
5. `FD` = `00`: the rainbow mode off.

## A colour

Four bits a channel. Each channel has four registers, one per step of the
chip's colour sequence; for one colour all four hold the same value, the four
bits in both halves: red `F0`–`F3`, green `F4`–`F7`, blue `F8`–`FB` (O).

The header is one light, `MSI LED` in the zone `MSI Zone` (O).
