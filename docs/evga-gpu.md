# EVGA Ampere graphics cards: the lighting controller over I2C

What the lighting controller on EVGA's Ampere (RTX 30) cards takes and
returns: the RTX 3080 FTW3 Ultra LHR's first. This page is the source for the
`evga` driver (spec 041).

## Sources

- **O**: OpenRGB 1.1's EVGA GPU v3 driver
  (`Controllers/EVGAAmpereGPUController`, tag `release_1.1`). GPL, so its
  code is not translated. See [credits.md](credits.md).
- **B**: the bench, recorded in spec 041's Verification.

## Transport

- Address `0x2D` on the card's first NVIDIA I2C adapter ("NVIDIA i2c adapter
  1"), for every card OpenRGB lists (O).
- The card is matched by its PCI IDs: NVIDIA's vendor and the GPU's device,
  EVGA's subsystem vendor (`3842`) and the card's subsystem device (O).
- Reads and writes are I2C block transfers. The first byte of a register's
  block is its length: the number of bytes after it (O).

## Registers

| Register | Bytes | Meaning (O) |
|---|---|---|
| `B1` | 6 | the firmware: version `1.<byte 4>.<byte 5>`. OpenRGB takes a full read as the controller being there |
| `B2` | 5 | write `04 C6 EB EA 15` before the card takes a command |
| `C0` | 10 | a mode per zone (bytes 1 to 4), an LED count per zone (5 to 8), and whether the zones run in step (9). `FF` leaves a field as it is |
| `C1` + zone | 5 | the static colour: brightness, red, green, blue |
| `90` | 5 | stores the lighting in the card. The driver never writes it |

Modes are 0 off, 1 static, 2 breathing, 3 rainbow, 4 colour cycle, 5 rainbow
wave, 6 wave, 7 star and 8 colour stack (O).

## Zones

| Zone | Name (O) |
|---|---|
| 0 | Front Logo |
| 1 | End plate Logo |
| 2 | Back Logo |
| 3 | Addressable Header |

A zone with no LEDs in `C0` is not there. Each zone shows one colour; the
controller offers no per-LED colour (O).

## Direct colour

OpenRGB's Direct mode sets every zone to static, with the zones in step, then
writes each zone's static register (O). The driver does the same, after the
unlock. It reads `C0` and each zone's static register at open, and writes them
back on release.

## On the bench (B)

The RTX 3080 FTW3 Ultra LHR has all four zones. A frame of four zones took 11
to 14 ms to write at the median; steady, breathe and wave at 50 ms showed on
every zone, and writing back what was read at open returned the card to its
earlier look.
