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

## Colours and speed (spec 032)

`SetEffectWith` writes an effect's colours by its shape (one into `30`–`32`,
two or three into the colour blocks) and its speed into `38`, the nearest of
`04`, `02` and `01`, slow to fast. The card's zone is a single light.

## Other cards (spec 033)

Every card OpenRGB pairs with this controller at `0x68` is on the driver's
list, matched on all four PCI IDs, because a subsystem ID repeats across cards
(O). The static colour is the same on all of them. Six cards lack Stack and
Rhythm, and the RTX 50 cards take the coloured Rhythm as mode `0B` (O). OpenRGB
gives the RTX 50 cards four effects more (Twisting `11`, Laminating `1D`,
Marquee `18`, Arrow `21`), whose colours it does not record; they are not
offered. Only the RTX 4090 Suprim Liquid X has been benched.

| Card | PCI device | Subsystem | Effects |
|---|---|---|---|
| MSI GeForce RTX 4060 Gaming X | `10de:2882` | `1462:5160` |  |
| MSI GeForce RTX 4060 Gaming X NV Edition | `10de:2882` | `1462:5164` |  |
| MSI GeForce RTX 4060 Ti Gaming X | `10de:2803` | `1462:5152` |  |
| MSI GeForce RTX 4060 Ti 16GB Gaming X | `10de:2805` | `1462:5172` |  |
| MSI GeForce RTX 4060 Ti 16GB Gaming X Slim White | `10de:2805` | `1462:5171` | no Stack or Rhythm |
| MSI GeForce RTX 4070 Gaming X Trio | `10de:2786` | `1462:5136` |  |
| MSI GeForce RTX 4070 Gaming X Slim | `10de:2786` | `1462:513f` |  |
| MSI GeForce RTX 4070 Gaming X Slim White | `10de:2786` | `1462:e131` |  |
| MSI GeForce RTX 4070 SUPER Gaming X Slim | `10de:2783` | `1462:513f` | no Stack or Rhythm |
| MSI GeForce RTX 4070 SUPER Gaming X Slim White | `10de:2783` | `1462:e131` |  |
| MSI GeForce RTX 4070 SUPER Gaming X Slim MLG | `10de:2783` | `1462:e134` |  |
| MSI GeForce RTX 4070 Ti Gaming X Trio | `10de:2782` | `1462:5132` |  |
| MSI GeForce RTX 4070 Ti Gaming X Trio White | `10de:2782` | `1462:5139` |  |
| MSI GeForce RTX 4070 Ti Gaming X Slim White | `10de:2782` | `1462:e132` |  |
| MSI GeForce RTX 4070 Ti Suprim X Trio | `10de:2782` | `1462:5133` |  |
| MSI GeForce RTX 4070 Ti SUPER Gaming X Trio White | `10de:2705` | `1462:5139` | no Stack or Rhythm |
| MSI GeForce RTX 4070 Ti SUPER Gaming Slim | `10de:2705` | `1462:e130` |  |
| MSI GeForce RTX 4070 Ti SUPER Gaming X Slim | `10de:2705` | `1462:e13d` |  |
| MSI GeForce RTX 4070 Ti SUPER Gaming White X Slim | `10de:2705` | `1462:e13e` |  |
| MSI GeForce RTX 4070 Ti SUPER Gaming White X Slim | `10de:2705` | `1462:e132` |  |
| MSI GeForce RTX 4080 Gaming X Slim White | `10de:2704` | `1462:511a` | no Stack or Rhythm |
| MSI GeForce RTX 4080 Gaming X Trio | `10de:2704` | `1462:5111` |  |
| MSI GeForce RTX 4080 Gaming X Trio White | `10de:2704` | `1462:5115` |  |
| MSI GeForce RTX 4080 Suprim X | `10de:2704` | `1462:5110` |  |
| MSI GeForce RTX 4080 SUPER Gaming X Slim | `10de:2702` | `1462:5117` |  |
| MSI GeForce RTX 4080 SUPER Gaming X Slim White | `10de:2702` | `1462:511a` | no Stack or Rhythm |
| MSI GeForce RTX 4080 SUPER Suprim X | `10de:2702` | `1462:5110` |  |
| MSI GeForce RTX 4080 SUPER Gaming X Trio | `10de:2702` | `1462:5111` | no Stack or Rhythm |
| MSI GeForce RTX 4090 Gaming X Trio | `10de:2684` | `1462:5103` |  |
| MSI GeForce RTX 4090 Gaming X Slim | `10de:2684` | `1462:510b` |  |
| MSI GeForce RTX 4090 Suprim Liquid X | `10de:2684` | `1462:5104` | tested (spec 029) |
| MSI GeForce RTX 4090 Suprim X | `10de:2684` | `1462:5102` |  |
| MSI GeForce RTX 4090 Suprim X Classic | `10de:2684` | `1462:5105` |  |
| MSI GeForce RTX 5060 Ti Gaming OC | `10de:2d04` | `1462:5354` | Rhythm in colour is mode `0B` |
| MSI GeForce RTX 5060 8G Gaming Trio OC | `10de:2d05` | `1462:5372` | Rhythm in colour is mode `0B` |
| MSI GeForce RTX 5060 Ti 16G Gaming Trio OC | `10de:2d04` | `1462:5353` | Rhythm in colour is mode `0B` |
| MSI GeForce RTX 5070 Gaming Trio | `10de:2f04` | `1462:5322` | Rhythm in colour is mode `0B` |
| MSI GeForce RTX 5070 Ti Gaming Trio OC Plus | `10de:2c05` | `1462:5315` | Rhythm in colour is mode `0B` |
| MSI GeForce RTX 5070 Ti VANGUARD SOC | `10de:2c05` | `1462:5314` | Rhythm in colour is mode `0B` |
| MSI GeForce RTX 5080 Gaming Trio OC | `10de:2c02` | `1462:5315` | Rhythm in colour is mode `0B` |
| MSI GeForce RTX 5080 VANGUARD OC SOC | `10de:2c02` | `1462:5314` | Rhythm in colour is mode `0B` |
| MSI GeForce RTX 5080 SUPRIM SOC | `10de:2c02` | `1462:5313` | Rhythm in colour is mode `0B` |
| MSI GeForce RTX 5080 SUPRIM LIQUID SOC | `10de:2c02` | `1462:5312` | Rhythm in colour is mode `0B` |
| MSI GeForce RTX 5090 Gaming Trio OC | `10de:2b85` | `1462:5303` | Rhythm in colour is mode `0B` |
| MSI GeForce RTX 5090 VANGUARD SOC | `10de:2b85` | `1462:5302` | Rhythm in colour is mode `0B` |
| MSI GeForce RTX 5090 SUPRIM SOC | `10de:2b85` | `1462:5301` | Rhythm in colour is mode `0B` |
| MSI GeForce RTX 5090 SUPRIM LIQUID SOC | `10de:2b85` | `1462:5300` | Rhythm in colour is mode `0B` |

Cards on MSI's older controller (GTX 10 to RTX 30, Radeon) answer at the same
address with another register map, and are not on the list.
