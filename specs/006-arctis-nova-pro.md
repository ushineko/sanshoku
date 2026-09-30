# 006 — Arctis Nova Pro Wireless battery, direct

**Issue**: #13

## Status: COMPLETE

## Context

The Arctis Nova Pro Wireless base station sits on the SteelSeries usage
page 0xFFC0 the `steelseries` driver already scans. Spec 003 listed it so
the driver would never write to it, and hayami reads its battery through a
`headsetcontrol` subprocess (`internal/peripherals/headset.go`), one of the
inherited tool calls `docs/design.md` names as a debt.

The protocol is one report each way. HeadsetControl 4.0.0
(`lib/devices/steelseries_arctis_nova_pro_wireless.hpp`) is the source of
record: on the base station's interface 4, write `06 b0`; the reply carries
the level in byte 6 on a 0–8 scale and the headset's status in byte 15. A
probe on 2026-09-29 against the base station on the desk (1038:12e5) read
`06 b0 00 00 01 00 06 08 0a 00 00 0a 04 00 08 08 …` in 15 ms: level 6 of 8,
which is the 75% headsetcontrol reported at the same moment, status 0x08
online. The reply echoes the request's two bytes, which is the match rule.

This spec makes the base station a device of the `steelseries` driver with
a battery, and promotes its entry from Listed to Tested.

## Requirements

### R1. Discovery

- R1.1 Products 0x12E0 (Nova Pro Wireless base station) and 0x12E5 (Nova
  Pro Wireless X base station) join the driver's allow-list as the headset
  family, distinct from the keyboard and mouse families. `Find` lists the
  base station's 0xFFC0 node as it does today; `Open` now succeeds on it.
  The kernel names both "SteelSeries Arctis Nova Pro Wireless".
- R1.2 The other nodes of the base station (consumer control on page
  0x0C) are not on the usage page and are not candidates, as now.

### R2. Protocol

- R2.1 Request: a 31-byte report `[0x06, 0xB0, 0…]` (HeadsetControl's
  `PACKET_SIZE_31`; the report ID is 0x06 and is the first byte on hidraw).
  Reply: a 64-byte input report read through `hidraw.Exchange` with the
  match `reply[0] == 0x06 && reply[1] == 0xB0`. Timeout the driver's
  `Timeout` (300 ms; the probe answered in 15 ms).
- R2.2 Exported `DecodeNovaPro(reply []byte) (battery.Battery, error)`:
  fewer than 16 bytes is an error; level `reply[6] * 100 / 8` with a value
  above 8 an error; status `reply[15]`: 0x01 headset offline, 0x02 charging
  on the cable, and any other value online (0x08 is the one seen; like
  HeadsetControl, the decoder does not require it). The name is the
  kernel's, `Kind` is `KindHeadset`.
- R2.3 Offline is not an error and not silence: the base station is present
  and says the headset is not. Following hayami's `headset.go`, the reading
  is returned with `HasLevel` false, so a consumer shows the headset present
  with no level. Charging on the cable is `Charging`; online is
  `Discharging`; a level of 100 while charging is `Full`. A base station
  that does not answer within the timeout, or answers with a reply that does
  not decode, is an error, as in HeadsetControl: it is on USB and does not
  sleep, so its silence is not a headset's.
- R2.4 The driver never sends any other command to the base station. The
  sidetone, lights, inactivity and equaliser commands in the source of
  record are writes to the device's settings and stay out.

### R3. Support and bench

- R3.1 The Listed Arctis entry becomes `Tested`, Hardware "SteelSeries
  Arctis Nova Pro Wireless, base station 1038:12e5", Capabilities battery,
  with the bench date; 0x12E0 is `Expected` in the same entry's Match words
  (one entry, two products, the tier is the tested one's; say in Notes that
  12e0 is by protocol).
- R3.2 `sanshoku-bench verify` compares a headset reading against
  `headsetcontrol -o json` when it is on PATH: level exact, status
  `BATTERY_CHARGING` ↔ `Charging` (and `Full`, which headsetcontrol does
  not distinguish), `BATTERY_AVAILABLE` ↔ `Discharging`. A reading with no
  level (headset off) is reported and not compared. The "headsets are out of
  scope" line goes.
- R3.3 The "Out of scope" text the bench embeds for `docs/devices.md` drops
  the Arctis candidate sentence; `make generate`.
- R3.4 `docs/design.md` "What belongs here": the headsetcontrol sentence
  becomes past tense for this headset (other headsets stay a candidate).

### R4. Tests

- R4.1 `DecodeNovaPro` table: the probe's captured reply (75%, online), a
  synthesised offline reply (byte 15 = 0x01), a charging reply (0x02, level
  8 → Full), a short reply, a level above 8.
- R4.2 The existing fake `ReportDevice`: one exchange that answers `06 b0`
  behind a stale unrelated report, so the match rule is exercised. No new
  fake.
- R4.3 The unlisted-product test keeps failing on any write to a product
  outside the allow-list; 0x12E5 is now inside it.
- R4.4 `live_test.go` gains the base station: skip on `ErrAbsent`; when
  present the level is 0–100 and, if `headsetcontrol` is on PATH, equal to
  its level.

## Acceptance Criteria

- [x] `make test` and `make lint` pass.
- [x] `DecodeNovaPro` reproduces the probe's 75% online reading and the
  synthesised offline, charging and error cases.
- [x] `sanshoku-bench read --driver steelseries` on the desk reports the
  Arctis level and `verify` agrees with headsetcontrol. Output pasted below.
- [x] With the headset switched off, `read` shows the headset present with
  no level and no error (the offline path, observed 2026-09-29 at the desk;
  output under "Headset off" below).
- [x] The support entry is Tested, `make check-support` passes, README and
  `docs/design.md` updated in the same commit.

## Risks & Assumptions

- **Two product IDs, one measured.** 0x12E0 is by HeadsetControl's table.
- **The base station answers `06 b0` unasked?** Not seen in the probe: after
  each reply the node was silent for 500 ms. If it broadcasts, the match
  rule still holds.
- **Rollback**: revert; hayami keeps headsetcontrol until phase 2.

## Verification

2026-09-29, base station 1038:12e5 on `/dev/hidraw14`, headset on,
headsetcontrol reporting 75% `BATTERY_AVAILABLE` at the same time.

`make test` (all packages ok; the steelseries live test opened the base
station and compared with headsetcontrol), `make lint` (0 issues),
`CGO_ENABLED=0 make build`, `make generate`, `make check-support` (no diff).

```
$ ./sanshoku-bench read --driver steelseries
steelseries  SteelSeries Arctis Nova Pro Wireless (1038:12e5)  /dev/hidraw14  [tested]
  battery: SteelSeries Arctis Nova Pro Wireless  75%  discharging  headset (4.0 ms)

$ ./sanshoku-bench read --driver steelseries --repeat 5 --interval 300ms | grep battery
  battery: SteelSeries Arctis Nova Pro Wireless  75%  discharging  headset (9.2 ms)
  battery: SteelSeries Arctis Nova Pro Wireless  75%  discharging  headset (14.3 ms)
  battery: SteelSeries Arctis Nova Pro Wireless  75%  discharging  headset (14.3 ms)
  battery: SteelSeries Arctis Nova Pro Wireless  75%  discharging  headset (13.3 ms)
  battery: SteelSeries Arctis Nova Pro Wireless  75%  discharging  headset (14.3 ms)

$ ./sanshoku-bench verify
NZXT, Inc. NZXT Kraken Elite V2 coolant: agree (ours 40.8, liquidctl 40.8)
NZXT, Inc. NZXT Kraken Elite V2 pump: agree (ours 2717, liquidctl 2717)
NZXT, Inc. NZXT Kraken Elite V2 fan: agree (ours 1428, liquidctl 1428)
G502 X PLUS battery: agree (ours 77%, solaar 77%)
SteelSeries Arctis Nova Pro Wireless battery: agree (ours 75% discharging, headsetcontrol 75% BATTERY_AVAILABLE)
```

The offline path (headset switched off) has not been observed on hardware;
see the unticked criterion.

### Headset off

Observed at the desk on 2026-09-29 with the headset switched off and the
base station plugged in. headsetcontrol reported `BATTERY_UNAVAILABLE`,
level -1, at the same moment.

```
$ ./sanshoku-bench read --driver steelseries
steelseries  SteelSeries Arctis Nova Pro Wireless (1038:12e5)  /dev/hidraw14  [tested]
  battery: SteelSeries Arctis Nova Pro Wireless  no level  off  headset (14.8 ms)
exit 0
$ ./sanshoku-bench verify
SteelSeries Arctis Nova Pro Wireless battery: headset off, no level; not compared
```
