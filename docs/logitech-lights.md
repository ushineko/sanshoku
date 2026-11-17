# Logitech HID++ 2.0 lighting: the G502 X PLUS

What a Logitech mouse behind its Lightspeed receiver takes for its lights,
over HID++ 2.0: the G502 X PLUS's (receiver `046d:c547`). This page is the
source for the `logitech` driver's canvas (spec 030).

## Sources

- **O**: OpenRGB 1.1's HID++ 2.0 driver
  (`Controllers/LogitechController/LogitechHIDPP20Controller`, tag
  `release_1.1`, commit `8e16de27`). GPL-2.0, so its code is not translated.
  Much of it its authors took from captures of the vendor's program, and say
  so. See [credits.md](credits.md).
- **B**: the bench, 2026-11-17, on the G502 X PLUS, recorded in spec 030's
  Verification.

## Requests

- A request is the short report `10` (3 parameter bytes) or the long `11` (16):
  report, device index, feature index, function in the high nibble with the
  software ID in the low, parameters (O+B). The lighting writes are long.
- A HID++ 2.0 error is `10|11, device, FF, feature index, function|swid, code`
  (O+B). Code `08` is busy: the request is sent again (O).
- On the desk the mouse is device index 1 behind the receiver's `0xFF00` node,
  with no node of its own (B).

## Features (B)

| Feature | Index on the G502 X PLUS | What |
|---|---|---|
| `0x8071` | 9 | RGB effects: the effect engine |
| `0x8081` | 10 | per-key lighting |
| `0x8100` | 11 | onboard profiles |
| `0x8101` | none | (the newer onboard profiles) |

## Zones

`0x8081` function 0 `00 <page> 00`, pages 0 to 2, answers a bitmap in its
parameter bytes 2 to 15, bits from the low end; zone = page × 112 + bit, and
bit 0 of page 0 is not a zone (O). The G502 X PLUS: `ff 01`, zones 1 to 8 (B).
OpenRGB names them "LED A" to "LED H" in that order, in a zone "Mouse LEDs"
(O).

## Taking the lights

In this order (O), each answered (B):

1. `0x8071` function 5, SetSWControl `01 03 06`: the effect engine to software,
   leaving the firmware drawing until the first frame.
2. Function 8, SetRgbPowerMode `01 01`: active.
3. Function 7 (long), power configuration `01 00 00 00 3C 01 2C`: idle after
   60 s, sleep after 300 s.
4. `0x8100` function 1 `02`: host mode (or `0x8101` function 6 `05`).
5. **The prep**, where the mouse has an effect card (`0x8071` function 0, long,
   `FF 00 01 00 01`; bytes 10 and 11 of the answer, `20 64` on the G502 X PLUS
   (O+B)): SetEffectByIndex (function 1, long) `FF 02`, the card's two bytes at
   parameters 8 and 9, keep (parameter 12) `01`; then `FF <n>`, keep `01`, where
   *n* is cluster 0's effect count (4). OpenRGB's comment says per-key does not
   engage without it; nobody has said what it is (O).

## A frame

- `0x8081` function 1 (long), SetIndividual: four `zone R G B` a report (O).
  Each is answered (B).
- Function 7 (long, zeros), FrameEnd, shows it (O). OpenRGB sends it again on
  busy, 10 ms apart, for up to 250 ms (O). On the bench nothing was busy, and a
  frame of 8 zones was about 24 ms (B).
- After the first FrameEnd, SetSWControl `01 03 05` (O).
- A breathe was smooth at 50 ms (B).
- **A frame holds**: 75 s with nothing sent, past the 60 s idle timer (B).
- **Asleep, the mouse answers nothing**; a frame then fails, and the next after
  it wakes takes the lights again (B).

## Giving them back

SetSWControl `01 00 00`, then `0x8100` function 1 `01` (or `0x8101` function 6
`03`) (O). The mouse then shows its onboard profile's lighting: red on the
desk's (B). Handing the engine back is answered in 370 to 510 ms (B).
