# MSI graphics cards over I2C: the RTX 4090 Suprim Liquid X

What an MSI graphics card's lighting controller takes, over the card's own
I2C adapter: the GeForce RTX 4090 Suprim Liquid X's (PCI `10de:2684`,
subsystem `1462:5104`). This page is the source for the `msi` driver (spec
029).

## Sources

- **O**: OpenRGB 1.1's MSI GPU v2 driver
  (`Controllers/MSIGPUController/MSIGPUv2Controller`, tag `release_1.1`,
  commit `8e16de27`). GPL-2.0, so its code is not translated. See
  [credits.md](credits.md).
- **H**: hotaru's live-effects spike (hotaru #205) and its
  `docs/hardware.md`, "One GPU that needs time between frames".
- **B**: the bench, 2026-11-17, through the `msi` driver, recorded in spec
  029's Verification.

## Finding it

- The controller is at address `0x68` on the NVIDIA driver's adapter named
  "NVIDIA i2c adapter 1 at …" (O). On Linux that is `/dev/i2c-N` through the
  `i2c-dev` module; `/sys/class/i2c-dev/i2c-N/name` gives the name, and the
  adapter's parent in sysfs is the card's PCI device, with its vendor,
  device and subsystem IDs (B).
- OpenRGB matches the card by those four IDs and the adapter, and reads
  nothing from the controller (O). The controller answers no question.

## Registers

Every write is an SMBus byte-data write, or an I2C block write for a colour
block, followed by 20 ms (O).

| Register | Meaning |
|---|---|
| `22` | the mode; `1C` while the card is set up (O) |
| `2E` | unknown; always written `00` first (O) |
| `30`, `31`, `32` | one colour, R, G, B (O) |
| `27`, `28`, `29` | colour blocks 1 to 3, three bytes in **B, G, R** order (O) |
| `36` | brightness: 20 × a step from 1 to 5 (O) |
| `38` | speed: `04`, `02`, `01`, slow to fast (O) |
| `46` | control: bit 1 leftwards, bit 0 "use the colour blocks" (O) |
| `3F` | save to the card; OpenRGB writes it only when asked (O) |

## A colour

`2E` = `00`, `22` = `1C`, `30`–`32` the colour, `36` the brightness, then
`22` = `13` (static) (O). With the 20 ms waits that is about 150 ms, and the
card must stay in static for 70 to 100 ms or the next colour cancels the
change: about four colours a second at most (H).

## Effects

The modes OpenRGB offers for this card, and how each takes its colours (O):

| Mode | Effect | Colours |
|---|---|---|
| `01` | Off | none; no brightness or speed |
| `08` | Rainbow Wave | none; `46` = `00` written first |
| `07` | Magic | none; `46` = `00` written after the idle mode |
| `07` | Color Cycle | blocks 1 to 3, `46` = `01` |
| `08` | Wave | blocks 1 to 3, `46` = `01` written first |
| `05`, `16`, `0D`, `0C`, `09`, `0F`, `02`, `03` | Patrolling, Meteor, Stack, Rhythm, Flowing, Whirling, Flashing, Double Flashing | one, registers `30`–`32` |
| `14`, `04` | Fade In, Breathing | blocks 1 and 2 |
| `06`, `15` | Streaming, Lightning | none |

Each is `2E` = `00`, `22` = `1C`, its colours, `36` and `38`, then `22` = the
mode (O). OpenRGB starts each with red, green and blue (O).
