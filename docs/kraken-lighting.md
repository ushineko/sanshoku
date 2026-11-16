# The Kraken's lighting protocol

What the NZXT Kraken 2024 Elite (`1e71:3012`) takes and returns for its
lighting: the pump ring and the fans chained to it. This page is the source
for the `nzxt` driver's lighting (#70). It records facts about the device, and
the driver is written from this page, not from another project's code.

## Sources

Each fact below carries its source.

- **O**: OpenRGB's Hue 2 driver (`Controllers/NZXTHue2Controller`, tag
  `release_1.1`, commit `8e16de27`). OpenRGB drives this cooler with
  `DetectNZXTKrakenElite`: two RGB channels, two fan channels, and the 2023
  effect format. GPL-2.0, so its code is not translated. See
  [credits.md](credits.md).
- **C**: a capture of NZXT CAM 4.76.9 on Windows 11, 2026-11-16, recorded by
  QEMU from the cooler passed through to a virtual machine. The capture holds
  this device's traffic only. It stays private, because every acknowledgement
  carries the device's serial number.
- **O+C**: both sources agree.

The capture is of firmware 1.2.12, after CAM updated the cooler from 1.2.8.
`bcdDevice` reads 01.02 on both, so it does not report the firmware (see
"Firmware version").

**Nothing on this page is verified through this module yet.** A fact is
verified when `sanshoku-bench` has sent it to the hardware and the spec
records the result.

## Transport

- Interface 1 of the device is HID, on a vendor usage page (`0xFF00`, C). The
  driver already uses it for status, through hidraw.
- The driver writes 64-byte reports today, and OpenRGB does the same (O). The
  capture shows 512-byte transfers whose bytes past 64 are zero. That may be
  how QEMU records the endpoint. Which length the firmware needs is not
  settled.
- Every open handle receives every input report ([contention.md](contention.md)).
  A reply is found by its first two bytes and a deadline, never by counting
  reports (see "The accessory list").

## Channels

| Channel | What is on it | LEDs |
|---|---|---|
| 1 | the pump ring | 24 |
| 2 | the fans' chain | 24 on this desk (one F360 Core, case version) |

A command names its channel in one byte. OpenRGB writes a mask (`1 << n` for
channel index `n`), and CAM writes `01` and `02`. For two channels those are
the same bytes, so the two readings cannot be told apart on this device.

## Firmware version

| Ask | Reply | Source |
|---|---|---|
| `10 01` | `11 01`, the version in bytes 17, 18 and 19 | O |
| `10 02` | `11 02`, the version in bytes 17, 18 and 19 (`01 02 0c` = 1.2.12) | C |

Both replies arrive in about 2 ms (C). The `11 02` reply also carries an
identifier in ASCII, which is not to be logged.

## The accessory list

- **Ask:** `20 03` (O+C).
- **Reply:** `21 03`. Channel `n` (from 0) has six accessory slots, at bytes
  `0x0F + 6n` to `0x14 + 6n`. Each slot holds an accessory id or zero (O).
- **Time:** 577 to 597 ms (C). The cooler is slow to answer.
- **Read it by a deadline, and match the reply.** OpenRGB 1.1 reads at most
  ten reports, then parses whatever it read last. When other programs on the
  same node are busy, the ten run out before the reply and both channels read
  0 LEDs (hotaru #206). A missing reply is a failure, not an empty list.

Accessory ids and their LED counts (O). The two on this desk are confirmed by
the capture.

| Id | Accessory | LEDs |
|---|---|---|
| `01` | Hue 1 strip | 10 |
| `02` | Aer 1 fan | 8 |
| `04` | Hue 2 strip, 10 LEDs | 10 |
| `05` | Hue 2 strip, 8 LEDs | 8 |
| `06` | Hue 2 strip, 6 LEDs | 6 |
| `08` | Hue 2 cable comb | 14 |
| `09` | Hue 2 underglow, 300 mm | 15 |
| `0A` | Hue 2 underglow, 200 mm | 10 |
| `0B` | Aer 2 fan, 120 mm | 8 |
| `0C` | Aer 2 fan, 140 mm | 8 |
| `10` | Kraken X3 ring | 8 |
| `11` | Kraken X3 logo | 1 |
| `13` | F120 RGB fan | 18 |
| `14` | F140 RGB fan | 18 |
| `15` | F120 RGB Duo fan | 20 |
| `16` | F140 RGB Duo fan | 20 |
| `17` | F120 RGB Core fan | 8 |
| `18` | F140 RGB Core fan | 8 |
| `19` | F120 RGB Core fan, case version | 8 |
| `1C` | F280 RGB Core fan, case version | 16 |
| `1D` | F360 RGB Core fan, case version (C: this desk, channel 2) | 24 |
| `1E` | Kraken Elite ring (C: this desk, channel 1) | 24 |
| `1F` | F420 RGB Core fan, case version | 24 |

An id that is not in the table has an unknown LED count. The driver reports
it and does not guess a count.

## Setting the LEDs

Two command families set every LED of a channel. Both use G, R, B order.

| | The `22` family (O) | The `26` family (C) |
|---|---|---|
| LED data | `22 1g <ch> 00` + up to 20 LEDs. `g` is 0 for LEDs 1 to 20 and 1 for 21 to 40 | `26 04 <ch> 00` + all 24 LEDs in one report |
| Apply | `22 A0 <ch> 00 01 00 00 28 00 00 80 00 32 00 00 01` | `26 06 <ch> 00 01 00 00 18 00 00 80 00 32 00 00 01` |
| Acknowledged | not read by OpenRGB | `26 04` in 102 to 335 ms (median 334, n=16); `26 06` in about 2 ms |

- The two apply commands differ in byte 7 only: `28` (40, the channel's
  maximum) from OpenRGB, `18` (24, the channel's LED count) from CAM.
- CAM uses the `26` family for a fixed colour, for per-LED colours and after
  a change of brightness (C). hotaru drives this cooler through OpenRGB's `22`
  family today, so that family works on firmware 1.2.8 and 1.2.12.
- **Brightness is a multiply on the host** (C). At about three quarters of
  CAM's slider, blue `00 00 FF` was sent as `00 00 BC`. No command carries a
  brightness.

### The live write

`26 14 <ch> 01` + 24 LEDs (G, R, B) (C).

- CAM sends it once a second while a channel's "Sync" setting ties the ring to
  the screen's colour. It stopped 0.19 s after Sync was switched off.
- **It is acknowledged in 1.3 to 3.1 ms** (n=206), against about 335 ms for
  `26 04`. This is the command for frames drawn on the host.
- `26 14 01 01` with no colours was sent when an audio effect was chosen. What
  it does is not known.
- CAM never sent it faster than once a second, so the fastest rate the cooler
  takes is not known. That is the first thing to measure.

## Firmware effects

`2A 04 <ch> <ch> <mode> <speed> <colours> ...` (O+C). The cooler animates the
effect itself, and nothing more is sent.

| Byte | Meaning |
|---|---|
| 2, 3 | the channel, twice |
| 4 | the mode (table below) |
| 5, 6 | the speed, 16 bits, little-endian. It is a period, so a smaller number is faster |
| 7 to 30 | up to eight colours, G, R, B |
| `0x37` | a flag. OpenRGB writes `02` for direction. CAM wrote `01` for Alternating and Starry Night, and `00` otherwise |
| `0x38` | the number of colours |
| `0x39` | a modifier. OpenRGB writes `08` for Fading, Pulse and Breathing. CAM wrote `08`, `05`, `04` or `00` |
| `0x3A` | OpenRGB writes `08`. CAM writes `18`, the channel's LED count |
| `0x3B` | `03`, and `04` for Alternating (C) |

The cooler acknowledges `2A 04` in about 118 ms (C, n=15).

| Mode | Effect | CAM's speed | OpenRGB's speeds, slowest to fastest |
|---|---|---|---|
| `01` | Fading | 40 | 80, 60, 40, 20, 10 |
| `02` | Spectrum Wave | 250 | 350, 300, 250, 150, 80 |
| `04` | Covering Marquee | 150 | 50, fixed |
| `05` | Alternating | 500 | 1600, 1300, 1000, 800, 600 |
| `06` | Pulse | 15 | 25, 20, 15, 7, 4 |
| `07` | Breathing | 20 | 40, 30, 20, 10, 4 |
| `08` | Candle | 50 | 50, fixed |
| `09` | Starry Night | 15 | 25, 20, 15, 7, 4 |
| `0B` | Rainbow Flow | 250 | 350, 300, 250, 150, 80 |
| `0C` | Super Rainbow | 150 | 350, 300, 250, 150, 80 |
| `0D` | Rainbow Pulse | 250 | 350, 300, 250, 150, 80 |
| `0E` | Tai Chi | 30 | (not in OpenRGB) |
| `0F` | Water Cooler | 30 | (not in OpenRGB) |
| `10` | Loading | 10 | (not in OpenRGB) |

OpenRGB also lists Marquee as mode `03`. CAM draws Marquee and Wave another
way: a `26 04` buffer, then `26 06 <ch> 00 02 <speed> 00 18 ...`, with byte 4
`02` and the speed in byte 5 (Marquee `29`, Wave `35`) (C). The cooler moves
the buffer.

## What is out of scope

- **Pump and fan duty.** CAM sends `72 01 01 00` and `72 02 01 01`, each with
  40 duty bytes, every second (C). This module does not write duty. After the
  capture session the pump read 76% and the fans 39%, close to the 78% and
  40% CAM sent. Whether the cooler kept CAM's values or runs its own curve is
  not known.
- **The screen.** Bucket uploads are the `nzxt` driver's today. Streamed frames
  are #27.
- **Firmware updates.** The bootloader is `1e71:3011`, firmware 0.2.0. This
  module sends nothing to it.
- **The temperature and audio effects.** They were not set up during the
  capture.

## Not known yet

1. The fastest rate `26 14` takes, and whether the next frame must wait for
   the acknowledgement.
2. Whether the `22` and `26` families are the same on the device, and whether
   either needs the other's apply.
3. Whether the cooler holds the last colours with nothing sent. hotaru's
   static scenes suggest that it does, because hotaru writes once and the
   colours stay.
4. The report length the firmware needs: 64 bytes or 512 bytes.
5. The meanings of `2A 04` bytes `0x37`, `0x39`, `0x3A` and `0x3B` beyond what
   the two sources wrote.
