# Corsair's V2 peripheral protocol: the MM700

What the Corsair MM700 RGB mousepad (`1b1c:1b9b`) takes and returns for its
lighting. This page is the source for the `corsair` driver (spec 026).

## Sources

- **O**: OpenRGB 1.1's Corsair V2 driver (`Controllers/CorsairPeripheralV2Controller`,
  tag `release_1.1`, commit `8e16de27`). GPL-2.0, so its code is not
  translated. See [credits.md](credits.md).
- **B**: the bench, 2026-11-17: raw reports written to the pad's hidraw node,
  with someone watching it. Recorded in spec 026's Verification.

## Transport

- The pad has two HID interfaces. Interface 1, on vendor usage page `0xFF42`
  usage 1, takes 64-byte output reports and returns 64-byte input reports,
  with no report ID (B, from its report descriptor). The other, usage 2, is
  input only.
- A write is `00` and 64 bytes. Byte 0 of the 64 is `08`, a wired device; a
  device behind Corsair's wireless receiver uses `09` (O). Byte 1 is the
  command.
- A reply is `00`, the command echoed, a status, then a 32-bit little-endian
  value (B). Status `00` is success, `05` unsupported, `09` refused (B); O
  names `01` invalid and `03` failed.

| Command | Bytes | Meaning |
|---|---|---|
| `01` | `08 01 <prop> 00 <value>` | set a property (O+B) |
| `02` | `08 02 <prop>` | get a property (O+B) |
| `0D` | `08 0D 00 <endpoint>` | open an endpoint (O+B) |
| `06` | `08 06 00 <len lo> <len hi> 00 00 <data>` | the first block of a write, 57 data bytes (O+B) |
| `07` | `08 07 00 <data>` | a later block, 61 data bytes (O) |
| `05` | `08 05 01 00` | close the endpoint (O+B) |

## Properties

| Property | Value on the MM700 (B) |
|---|---|
| `03` | the render mode: `01` hardware, `02` software |
| `11` | the vendor, `1b1c` |
| `12` | the product, `1b9b`. OpenRGB asks it up to 8 times, 120 ms apart, because a freshly enumerated device does not answer at once (O) |
| `13` | `00050001` |
| `5F` | status `05`, unsupported. OpenRGB asks it when it takes the pad (O) |

## Lighting

- **Software mode first.** In hardware mode (`03` = `01`) the pad shows its
  stored effect, and **a frame is accepted and not shown**: every status reads
  `00` (B). `03` = `02` takes it to software mode.
- **Endpoint `01`.** Opening `22`, which OpenRGB tries first, is refused with
  status `09`; `01` opens (B).
- **A frame** is open, one `06` block, close. Its data is planar: every
  light's red, then every green, then every blue (O). Three lights, nine bytes.
  Each step is answered in about 1 to 2 ms, and a frame takes about 17 ms (B).
- **An endpoint left open locks the pad.** While endpoint `01` is open, every
  open is refused with status `03`, across restarts of the program drawing,
  until something closes it (B, 2026-11-17). It happened after a stop that
  cut a frame off between its open and its close, and while a game under
  Wine held the pad. So a frame closes the endpoint once the open is
  answered, whatever happens next, and an open refused with `03` is followed
  by one close and one more open (spec 036).
- **The lights** are Left, Right and Logo, in that order (O+B).
- **A frame holds.** The pad kept one for 28 minutes in software mode with
  nothing sent and the handle closed (B). OpenRGB resends a frame every 30 s
  for Corsair keyboards that revert after about a minute (O); the MM700 does
  not need it.
- **Back to hardware mode** returns the pad to its stored effect (B).
  Software mode otherwise lasts until the next USB enumeration (O).
- OpenRGB lists firmware effects for this family (Static `207E`, Spectrum
  `7BFF` and others) but does not send them (O). This module does not either.

## Other products (spec 033)

The MM700 3XL (`1bc9`) has the MM700's entry in OpenRGB's V2 table: Left,
Right and Logo, one light each (O). It is opened as expected. The V2 mice,
keyboards and receivers need tables and protocol work of their own (#92).
