# Corsair RGB memory over the chipset's SMBus

What Corsair's RGB memory modules take and return for their lights: the
Dominator Platinum RGB DDR5's first. This page is the source for the
`corsairdram` driver (spec 042).

## Sources

- **O**: OpenRGB 1.1's Corsair DRAM driver
  (`Controllers/CorsairDRAMController`, tag `release_1.1`). GPL, so its code
  is not translated. See [credits.md](credits.md).
- **B**: the bench, recorded in spec 042's Verification.

## Where a module is

- On the chipset's SMBus: OpenRGB looks only on the AMD FCH and the Intel
  controllers from ICH10 to Arrow Lake, by PCI ID (O).
- At `0x18` to `0x1F` (DDR5) or `0x58` to `0x5F` (DDR4) (O). A DDR5 module's
  SPD hub is at `0x50` to `0x57`, where the kernel's `spd5118` driver owns
  it; the kernel refuses another program that address.
- A module answers register `43` with `1A`, `1B` or `1C`, and `44` with `01`,
  `03` or `04` (O). Both are byte reads.

## Device information

Write `00` to `61` (select the block) and to `21` (start), then read `40` 32
times for the block's bytes, and `42` for its CRC-8 (O):

| Byte | Meaning (O) |
|---|---|
| 0–1 | vendor, low byte first |
| 2–3 | product, low byte first |
| 8–11 | firmware: `<9>.<8>.<11:10>` |
| 28 | protocol version |

The CRC is polynomial `07`, starting at 0, unreflected.

## Products (O)

| Product IDs | Name | LEDs | Order |
|---|---|---|---|
| `0100`, `0101` | Vengeance RGB Pro DDR4 | 10 | first to last |
| `0200`, `0201` | Dominator Platinum RGB DDR4 | 12 | last to first |
| `0300`, `0301` | Vengeance RGB Pro SL DDR4 | 10 | first to last |
| `0400`, `0401` | Vengeance RGB RS DDR4 | 6 | first to last |
| `0600`, `0601` | Dominator Platinum RGB DDR5 | 12 | last to first |
| `0800`, `0801`, `0810`, `0811` | Dominator Titanium RGB DDR5 | 12 | last to first |
| `0700`, `0701`, `0900`, `0901`, `0910`, `0911` | Vengeance RGB DDR5 | 10 | first to last |
| `0A00`, `0A01`, `0A10`, `0A11` | Vengeance Shugo Series DDR5 | 10 | first to last |
| `0B00`, `0B01` | Vengeance RGB RS DDR5 | 6 | first to last |

## Direct frames

Protocol 4 and later (O): the LED count, then each LED's red, green and blue
in the product's order, then a CRC-8 of all of that. The first 32 bytes are an
SMBus block write to `31`, the rest one to `32`. The module answers nothing.

## Registers the driver never writes

The driver writes only `61`, `21`, `31` and `32`. OpenRGB names others it
uses for effects and the bootloader (O):

| Register | What it does |
|---|---|
| `0B` | resets the buffer |
| `20` | writes a byte to the buffer |
| `23` | switches between the bootloader and normal mode |
| `26` | selects a configuration buffer |
| `82` | writes and applies a configuration |

## On the bench (B)

Four Dominator Platinum RGB DDR5 modules at `0x18` to `0x1B` on an Intel
Raptor Lake SMBus, beside SPD hubs the kernel owns at `0x50` to `0x53`: each
identified, read its device information with a good CRC, and took direct
frames, about 7 ms each. A module keeps its last frame with nothing sent, for
minutes at least.
