# Credits

Every protocol in this module was learned from someone else's work before it
was written here. This page says whose, what was taken, and under what
licence, because the projects are open source and saying so is the least
they are owed. Where code was translated rather than a protocol relearned,
the page says that too.

| Project | Licence | What this module owes it | Where |
|---|---|---|---|
| [liquidctl](https://github.com/liquidctl/liquidctl) | GPL-3.0 | The NZXT Kraken 2023 / Elite protocol: the command table (`74 01` status, the `30`/`32`/`36`/`38` bucket commands, the bulk header), draining queued reports before an ask (`clear_enqueued_reports`), twelve reads per attempt, and the bucket memory placement, written in Go after reading liquidctl's `_get_bucket_memory_offset`, because the device refuses an address it did not arrive at itself. hotaru's spec 012 read the protocol off liquidctl and wrote its own driver; sanshoku's spec 005 ported hotaru. | `nzxt` |
| [HeadsetControl](https://github.com/Sapd/HeadsetControl) | GPL-3.0 | The Arctis Nova Pro Wireless base station's battery exchange: interface 4, request `06 b0`, the level in byte 6 on a 0–8 scale, the status in byte 15 (`steelseries_arctis_nova_pro_wireless.hpp`). Reimplemented from a reading of the source; the decoder was written from a probe's bytes. | `steelseries` (spec 006) |
| [Solaar](https://github.com/pwr-Solaar/Solaar) | GPL-2.0 | The HID++ 1.0 and 2.0 knowledge behind the Logitech driver: feature numbers 0x0000, 0x0005, 0x1000, 0x1004, the 1.0 registers 0x07 and 0x0D, the error forms, the software-ID convention (this module never uses Solaar's own 0x0B so the two can share a receiver), and the device table that named the K800. hayami's specs 008 and 018; the bench's `verify` compares against `solaar show`. | `logitech` |
| [OpenRazer](https://github.com/openrazer/openrazer) | GPL-2.0 | The Razer report format (90 bytes, the XOR checksum over the body, status codes), the command classes for battery level and charging, and the dock's RF relay transaction ID 0x1F from its unmerged PR #2817. hayami's spec 016. | `razer` |
| [rivalcfg](https://github.com/flozz/rivalcfg) | WTFPL | The SteelSeries battery commands (0x92, and 0xD2 through a dongle), the reply's shape, and the product table the allow-list started from. hayami's spec 017. | `steelseries` |
| [LibrePods](https://github.com/librepods-org/librepods) | GPL-3.0 | Apple's accessory protocol over L2CAP PSM 0x1001: the handshake bytes, the battery packet's layout and the cell and status codes. hayami's spec 009. | `apple` |
| The Linux kernel | GPL-2.0 | hidraw, usbfs and hwmon are its interfaces; `hid-logitech-dj`'s `HID_PHYS` suffix is how a paired child node is told from its receiver; `nzxt-kraken3` documents the status report this module reads. | `hidraw`, `usbfs`, `hwmon` |
| [hayami](https://github.com/ushineko/hayami) and [hotaru](https://github.com/ushineko/hotaru) | MIT | The same author's programs, whose device code this module is the maintained copy of (specs 001–005). | everything |

## On licences

This module is MIT. What it took from the projects above is knowledge: which
bytes to send, how to read what comes back, which order the device insists
on. Protocol facts and algorithms are not what copyright protects, and none
of this module's Go is a translation of another project's source; where a
comment says "ported from liquidctl" it means the approach was learned
there, and the Go was written from that understanding. A reader who wants
to compare is welcome to; the rows above say where to look.

## Adding a line

A new driver's spec names the project it learned from, and this page gets a
row in the same commit. Politeness is not a release step; it is part of
the work.
