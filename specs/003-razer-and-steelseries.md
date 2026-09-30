# 003 — Razer and SteelSeries batteries

**Issue**: #3

## Status: INCOMPLETE

## Context

hayami reads a Razer mouse's battery through its Mouse Dock Pro and a
SteelSeries Apex Pro TKL Wireless Gen 3's battery over hidraw
(`internal/peripherals/razer.go`, `steelseries.go` at 54d357c; specs 016
and 017). Both were measured on one machine. Razer devices declare no
output report, so the exchange is a feature report through ioctls with a
settle time between set and get. SteelSeries uses 64-byte reports with no
report ID and the rivalcfg command set, and hayami holds a product
allow-list because the Arctis Nova Pro exposes the same usage page and must
never be written to.

This spec ports both as the `razer` and `steelseries` drivers. Two drivers
in one spec because they share the shape (one battery per node, getters
only, an allow-list) and each is under 300 lines.

## Requirements

### R1. `razer` driver

- R1.1 `razer.Driver{Settle time.Duration; Timeout time.Duration}` (defaults
  70 ms and 300 ms). `Name()` is `"razer"`.
- R1.2 `Find`: `hidraw.Nodes(0x1532, usage page 0xFF00 or 0xFF01)`. One
  candidate per node.
- R1.3 Report: 91 bytes with a leading report number 0; body `[status,
  transaction, remaining(2), protocol, size, class, command, args(80), crc,
  reserved]`; CRC is XOR over `body[2:88]` and is verified on the reply.
  Exported `Decode(reply []byte) (level int, charging bool, err error)` over
  the two commands: the reply's echoed class and command say which it
  answers; the level command fills `level`, the charging command fills
  `charging`, and the other result is zero.
- R1.4 Commands, getters only: class 0x07 command 0x80 battery level
  (`arg[1]` 0..255 scaled to percent), class 0x07 command 0x84 charging.
  Status 0x02 OK; 0x01 busy, 0x04 timeout, 0x05 not supported are silence;
  0x03 fail is an error.
- R1.5 Exchange: `SetFeature`, sleep `Settle` (below 50 ms the device
  returns the previous answer), `GetFeature`, each bounded by the context
  per spec 001 R3.7.
- R1.6 Transaction IDs tried in order {0x1F, 0x3F, 0xFF, 0x9F, 0x00}; 0x1F is
  the dock's RF relay to the mouse. The one that answers is remembered per
  device (per open `Device`; hayami remembered it per node path in a
  long-lived reader); busy or timeout does not unlearn it.
- R1.7 Kind from product: 0x007E Mouse Dock, 0x0088 Basilisk Ultimate
  dongle, 0x00A4 Mouse Dock Pro → `KindMouse`; else `KindOther`. The
  battery is named after the node (the dock), as hayami does.

### R2. `steelseries` driver

- R2.1 `steelseries.Driver{Timeout time.Duration}` (default 300 ms).
  `Name()` is `"steelseries"`.
- R2.2 `Find`: `hidraw.Nodes(0x1038, hidraw.UsagePage(0xFFC0))`.
- R2.3 Allow-list (hayami spec 017), the only products ever written to:
  modern battery (0x92 then 0xD2): 0x1644/0x1646 Apex Pro TKL Wireless Gen 3
  (measured), 0x1838/0x183A Aerox 3, 0x1852/0x1854 Aerox 5, 0x1858/0x185A
  Aerox 9, 0x1840/0x1842 Prime Wireless. Legacy (0xAA 0x01, three-byte
  reply): 0x1830 Rival 3 Wireless, 0x1872 Rival 3 Gen 2, 0x172B Rival 650,
  **listed and still unimplemented**, as in hayami; `Open` on one returns
  `sanshoku.ErrUnsupported`.
- R2.4 A product not in the table: `Find` still lists it as a candidate so
  the bench shows it, and `Open` returns `ErrUnsupported` with the kernel
  name. It is never written to. This replaces hayami's `Unsupported()` side
  channel.
- R2.5 Protocol: write `[0x00, cmd, 0...]` (65 bytes), read until `buf[0]
  == cmd` (and more than one byte) or the deadline, through
  `hidraw.Exchange`. Exported `DecodeModern(reply []byte) (battery.Battery,
  error)`: `v = reply[1]`, bit 7 charging, level `((v & 0x7F) - 1) * 5`;
  charging at 100 is Full. A reply shorter than two bytes, or a value whose
  level falls outside 0..100 (0x00, 0x80, 0x7F), is an error and not a
  reading, as hayami's `decodeModernBattery` has it.
- R2.6 Kind is `KindOther`; hayami had no product-to-kind table here.

### R3. Tests

- R3.1 Razer: `Decode` table from hayami `razer_test.go` bytes including a
  bad CRC; one fake feature device carrying the transaction search (the
  first two IDs return not-supported, the third answers) and a busy status.
- R3.2 SteelSeries: `DecodeModern` table; one fake carrying the 0x92 then
  0xD2 fallback; a test that an unlisted product is never written to (the
  fake fails on any write).
- R3.3 `live_test.go` per driver, skipping on `ErrAbsent`. There was no live
  test for either in hayami; the bench is the oracle.

### R4. Documentation

- R4.1 `docs/devices.md` is regenerated from the entries in R4.3.
- R4.2 README, changelog, `all.Drivers()`, and `docs/udev.md` rows
  confirmed.
- R4.3 `razer.Support()` and `steelseries.Support()`: `Tested` for the
  Mouse Dock Pro and the Apex Pro TKL Wireless Gen 3 once this module's
  bench has read them; `Expected` for the Mouse Dock, the Basilisk dongle
  and the allow-listed Aerox and Prime mice; `Listed` for the Rival 3
  Wireless, Rival 3 Gen 2 and Rival 650. Neither the dock nor the Apex was
  on the desk at the bench run below, so both are `Expected` (hayami
  measured them) until a bench run promotes them.

## Acceptance Criteria

- [x] `make test` and `make lint` pass.
- [x] The Razer CRC check rejects a corrupted reply and the transaction
  search remembers the answering ID.
- [x] The SteelSeries fake proves an unlisted product receives no write.
- [ ] `sanshoku-bench read --driver razer` and `--driver steelseries` on the
  machine with the dock and the Apex report levels that match the devices'
  own indicators. Output pasted below. **Open**: neither the Mouse Dock Pro
  nor the Apex Pro TKL Wireless Gen 3 was plugged in at the bench run; the
  only SteelSeries device present was the Arctis Nova Pro, refused as
  unlisted. The two entries stay `Expected` until this runs.
- [x] Both `Support()` tables exist and `make check-support` passes;
  README and `all` updated in the same commit.

## Risks & Assumptions

- **The feature ioctl has no interrupt**; spec 001 R3.7 bounds it at the
  edges only.
- **Legacy SteelSeries** stays a stub. Implementing 0xAA needs a device.
- **Rollback**: revert.

## Verification

Bench run 2026-09-29, `CGO_ENABLED=0 make build`, udev rules installed. On
the desk: a Logitech Lightspeed receiver and a SteelSeries Arctis Nova Pro
Wireless (1038:12e5). **No Razer device and no Apex Pro TKL Wireless Gen 3**
were plugged in.

`make test`: every package `ok`; `razer` and `steelseries` live tests skip
("no Razer control node on this machine"; "every SteelSeries node here is a
product this driver does not speak to"). `make lint`: `0 issues.` (with a
fresh `GOLANGCI_LINT_CACHE`; the shared cache held results for a removed
worktree's paths). `make check-support`: no diff.

`sanshoku-bench scan` (the steelseries and logitech lines; the rest is
hwmon):

```
logitech     Logitech USB Receiver (046d:c547)                 /dev/hidraw12             battery       tested
steelseries  SteelSeries Arctis Nova Pro Wireless (1038:12e5)  /dev/hidraw14             unsupported   listed
15 found
```

The Arctis has a Listed entry in `steelseries.Support()`, so the bench names
it rather than saying "not in the support table", and a refused `Open` is a
state, not a failure.

`sanshoku-bench read --driver razer`: no output, exit 0 (no Razer node, so
`ErrAbsent`).

`sanshoku-bench read --driver steelseries`, exit 0:

```
steelseries  SteelSeries Arctis Nova Pro Wireless (1038:12e5)  /dev/hidraw14  [listed]
  unsupported: recognised by the driver and never written to
```

The Arctis is never opened, let alone written to: under
`strace -f -e trace=openat,write,ioctl`, the only accesses to hidraw14 are
its sysfs `uevent` and `report_descriptor`; `/dev/hidraw14` is not opened.
