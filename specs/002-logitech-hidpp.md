# 002 — Logitech HID++ batteries

**Issue**: #2

## Status: COMPLETE

## Context

hayami reads Logitech batteries by speaking HID++ over hidraw to receivers
and wired devices (`internal/peripherals/hidpp.go`, `hidpp10.go`,
`logitech.go`, `battery.go` at 54d357c; specs 008, 017, 018). It replaced a
`solaar show` subprocess that took 3.5 s with a read that takes 5 ms. Every
number in it was measured on a G502 X PLUS on a Lightspeed receiver and a
K800 on a Unifying receiver.

This spec ports that code as the `logitech` driver on the transport spec 001
built. Behaviour is preserved; the source of record is hayami at that commit
and its tests. The only change of shape is the one `docs/design.md` requires:
one `Device` per hidraw node, opened once and held, rather than a reader that
opens every node on every poll.

## Requirements

### R1. Driver and discovery

- R1.1 `logitech.Driver{Timeout time.Duration}` (default 300 ms, hayami
  `RequestTimeout`). `Name()` is `"logitech"`.
- R1.2 `Find` calls `hidraw.Nodes(0x046D, hidraw.HasReportID(page >= 0xFF00,
  0x10))`: a vendor page with report ID 0x10 in it (hayami `speaksHIDPP`).
  One `Candidate` per node. Matching is by descriptor, never by product.
- R1.3 `Open` opens the node `O_RDWR` and returns a `Device` satisfying
  `battery.Source`. It does not probe at open; the first `Batteries` call
  discovers.

### R2. HID++ protocol

Ported from `hidpp.go` with its comments.

- R2.1 Short report `[0x10, index, feature, function<<4|swid, p0, p1, p2]`;
  a reply may be short (0x10) or long (0x11) and both are accepted.
- R2.2 Software ID from `pid % 14` over {1..15} minus 0x0B (solaar's), so a
  reply to solaar is recognisable as not ours.
- R2.3 Error forms: HID++ 1.0 `r[2] == 0x8F`, code at `r[5]` (0x08 no
  device, 0x04/0x09/0x07 not reachable, 0x01 old protocol, 0x02 unknown
  feature); HID++ 2.0 `r[2] == 0xFF`, code at `r[4]` (0x01 unknown feature).
  The two code spaces overlap and the comment says so (hayami issue #66).
- R2.4 Unrelated traffic on the node is skipped by matching device index,
  feature and function through `hidraw.Exchange`. An error reply (either
  form) is matched on device index alone, as hayami does; see Risks.
- R2.5 A HID++ 2.0 request is retried only on silence, 5 attempts (a mouse
  idle for 6 s needed four). A HID++ 1.0 register read makes one attempt, as
  hayami's does. Silence after the last attempt is `errSilent`, swallowed: the
  reading is withheld, not an error.
- R2.6 `featureIndex(feature)` via root feature 0x0000 function 0; index 0
  means absent.

### R3. HID++ 2.0 batteries

- R3.1 Feature 0x1004 Unified Battery, function 1: `p[0]` state of charge
  (≤ 100), `p[2]` 1/2 charging, 3 full. Exported decoder
  `DecodeUnifiedBattery(p []byte) (battery.Battery, error)`.
- R3.2 Feature 0x1000 Battery Status, function 0: `p[0]` level, `p[2]` 1/4
  charging, 2/3 full, 5/6/7 fault treated as discharging. Exported decoder
  `DecodeBatteryStatus`.
- R3.3 Feature 0x0005 Device Name and Type: function 0 length, function 1
  chunk at offset, function 2 type (0 keyboard, 2 numpad, 3 mouse, 4
  touchpad, 5 trackball → `battery.Kind`). The name requires the full
  declared length, all bytes printable, at most 64 bytes (the phantom "Q",
  hayami issue #58).

### R4. HID++ 1.0 batteries

- R4.1 `GET_REGISTER` `[0x10, index, 0x81, reg, 0, 0, 0]`. Only getters are
  ever sent.
- R4.2 Register 0x0D (charge percent) first, register 0x07 (band) as the
  fallback. Band map 0x01 Critical, 0x03 Low, 0x05 Good, 0x07 Full into
  `battery.Band`. A charge byte of 0 means discharging.
- R4.3 A 1.0 device is named by the kernel (`HID_NAME` of the paired child
  node), because 1.0 has no name feature hayami reads.

### R5. Device discovery within a node

Ported from `logitech.go` `discover`.

- R5.1 When nothing is remembered (the first `Batteries`, or after every
  remembered index has failed), probe index 0xFF (wired) and 1..6 with a `featureIndex(0x1004)` lookup: OK or unknown
  feature → a 2.0 device; old protocol on a paired child → a 1.0 device; not
  reachable → counted as quiet.
  *Amended by spec 007*: a node whose `HID_PHYS` carries a paired index
  (`hidraw.PairedIndex`, the `:N` a paired child's node ends in) is probed at
  that index alone, with the same lookup, classification and attempts; a
  child node answers for its one device and no other. Any other node probes
  0xFF and 1..6 as above. On a child node `Quiet` is 0 or 1.
- R5.2 Located devices are remembered per `Device`. An index whose read fails
  is dropped; the node is rediscovered when nothing is left, which is
  hayami's rule. A second device that fails while another still reads is not
  looked for again until the other fails too.
- R5.3 `(*Device).Presence() Presence{Nodes, Quiet int; TooOld []string}`
  exported, because hayami's doctor shows it. This is the one driver-specific
  export beyond the decoders, and it is asserted by interface
  (`logitech.Presencer`), not by concrete type.

### R6. Results

- R6.1 `Batteries` returns one `battery.Battery` per located device, partial
  results beside a joined error, silence swallowed.
- R6.2 A node that has gone (`ENODEV`) returns `sanshoku.ErrGone`.

### R7. Tests

- R7.1 Decoders table-tested from the byte sequences in hayami's tests
  (`hidpp_test.go`, `band_test.go`, `phantom_test.go`).
- R7.2 One fake transport (hayami `fakeEndpoint`) carrying: a long reply to
  a short request, a solaar reply to skip, silence for N attempts then an
  answer, the 1.0 error form, the truncated name. Nothing else.
- R7.3 `live_test.go`: skips on `ErrAbsent`; every found node opens; every
  reading is plausible (0..100); if `solaar` is on PATH, levels agree.
- R7.4 Bench `verify` compares against `solaar show` when present.

### R8. Documentation

- R8.1 `docs/devices.md` is regenerated from the entries in R8.3.
- R8.2 README table row and changelog entry. `all.Drivers()` gains
  `logitech.Driver{}`.
- R8.3 `logitech.Support()`: a `Tested` entry, with the bench date, for every
  HID++ device the bench reads on the desk; devices not present stay
  `Expected`. On the development machine that is the G502 X PLUS via
  Lightspeed (`Tested`) and the K800 via Unifying (`Expected`, measured by
  hayami, not on this bench). `Expected`
  entries for "any HID++ 2.0 device with feature 0x1004 or 0x1000" and
  "any HID++ 1.0 device with register 0x0D or 0x07". `make check-support`
  passes.

## Acceptance Criteria

- [x] `make test` and `make lint` pass; no test opens a device.
  (Unit tests drive a fake `hidraw.ReportDevice`; `live_test.go` opens the node by
  design and skips on `ErrAbsent`.)
- [x] The decoder tables reproduce hayami's expected values for Unified
  Battery, Battery Status, register 0x0D and register 0x07.
- [x] The fake transport test shows a reading survives one solaar reply and
  four silent attempts, and a fifth silent attempt withholds the reading
  without an error.
- [x] The phantom-name case falls back to the vendor name, never a
  truncated one.
- [x] `sanshoku-bench read --driver logitech` reports every HID++ device on
  the desk with plausible levels and `verify` agrees with solaar; devices not
  present stay `Expected`. Output pasted below.
- [x] `logitech.Support()` entries exist and `make check-support` passes;
  README and `all` updated in the same commit.

## Risks & Assumptions

- **Contention with solaar and the desktop applet** is handled by the
  software ID only. Two processes asking the same receiver at once can each
  see the other's silence; the retry covers it in practice.
- **Error replies are matched on device index only.** A HID++ 1.0 (0x8F) or
  2.0 (0xFF) error reply for the right index is taken as the answer without
  checking the function byte, as hayami does, so an error meant for solaar
  can be taken as ours. Tightening it is a later spec.
- **Rollback**: revert; no consumer yet.
- The K800 is the only 1.0 device measured. Register 0x0D's absence path is
  measured on it; other 1.0 devices are by protocol.

## Verification

Run on 2026-09-29 on the development machine: one Logitech Lightspeed
receiver (046d:c547) with a G502 X PLUS paired at index 1. No Unifying
receiver and no HID++ 1.0 device is attached; `solaar show` lists the same
single device. `solaar` 1.1.20 is on PATH.

`make test` (race detector; the live test ran against the receiver and
agreed with solaar), `make lint`, `CGO_ENABLED=0 make build` and
`make check-support` pass. The output below is from the run after the
review's hidraw changes (`ReportDevice`, `ErrSilent`, `BusUSB` and
`BusBluetooth`); the scan is unchanged from the first run.

```
$ ./sanshoku-bench scan
DRIVER    DEVICE                             PATH                      CAPABILITIES  TIER
hwmon     acpitz (0000:0000)                 /sys/class/hwmon/hwmon0   -             not in the support table
hwmon     nvme (0000:0000)                   /sys/class/hwmon/hwmon1   -             not in the support table
hwmon     coretemp (0000:0000)               /sys/class/hwmon/hwmon10  -             tested
hwmon     spd5118 (0000:0000)                /sys/class/hwmon/hwmon11  -             not in the support table
hwmon     iwlwifi_1 (0000:0000)              /sys/class/hwmon/hwmon12  -             not in the support table
hwmon     nvme (0000:0000)                   /sys/class/hwmon/hwmon2   -             not in the support table
hwmon     nvme (0000:0000)                   /sys/class/hwmon/hwmon3   -             not in the support table
hwmon     nct6798 (0000:0000)                /sys/class/hwmon/hwmon4   -             not in the support table
hwmon     corsairpsu (0000:0000)             /sys/class/hwmon/hwmon5   -             not in the support table
hwmon     asus (0000:0000)                   /sys/class/hwmon/hwmon6   -             not in the support table
hwmon     spd5118 (0000:0000)                /sys/class/hwmon/hwmon7   -             not in the support table
hwmon     spd5118 (0000:0000)                /sys/class/hwmon/hwmon8   -             not in the support table
hwmon     spd5118 (0000:0000)                /sys/class/hwmon/hwmon9   -             not in the support table
logitech  Logitech USB Receiver (046d:c547)  /dev/hidraw12             battery       tested
14 found

$ ./sanshoku-bench read --driver logitech
logitech  Logitech USB Receiver (046d:c547)  /dev/hidraw12  [tested]
  battery: G502 X PLUS  77%  discharging  mouse (71.1 ms)

$ ./sanshoku-bench verify
liquidctl: on PATH; no cooling reading to compare
G502 X PLUS battery: agree (ours 77%, solaar 77%)
headsetcontrol: on PATH; headsets are out of scope, not compared
```

Of the receiver's three nodes (hidraw10, 11, 12) only hidraw12 is matched,
by descriptor. A read with the mouse awake takes 30 to 80 ms; after ten
seconds idle it takes 0.4 to 1.4 s, which is the silence retry waking it
(hayami's measurement). Three `verify` runs shortly after an idle spell read
nothing in about 6 ms with no error. A fast empty answer with no error can
only be a 1.0 refusal during discovery, most likely "not reachable" for index
1, which is counted in `Presence.Quiet` and not retried, as in hayami;
`Presence` was not captured in those runs, so this is inferred. The next run
read the mouse, and a 90-second probe at ten-second intervals read it every
time with `Quiet` 0.

`read` now prints a HID++ node's Presence beside a withheld reading
(`battery: no reading (6.5 ms); nodes: 1, quiet: 1, too old: none`). Ten
reads at fifteen-second intervals after the change all read the mouse
(27 ms to 0.9 s), so that line has not been seen on the hardware yet.


### Other machines

Run on 2026-09-29 on the machine with the Unifying receiver, after its
devices were woken by hand (they answered nothing asleep, as did the
kernel's own `hidpp_battery_0`). The K800 promotes to Tested. The
Performance MX is a pairing the receiver still lists for a mouse that no
longer exists; the receiver node answers "not reachable" for every index in
224 ms, and each child node costs nine seconds of silence, which is issue
#11.

```
$ ./sanshoku-bench read --driver logitech
logitech  Logitech USB Receiver (046d:c52b)  /dev/hidraw1  [expected]
  battery: no reading (134.9 ms); nodes: 1, quiet: 5, too old: none
logitech  Logitech K800 (046d:2010)  /dev/hidraw7  [expected]
  battery: Logitech K800  Good  discharging  other (9713.1 ms)
logitech  Logitech Performance MX (046d:101a)  /dev/hidraw8  [expected]
  battery: no reading (9023.3 ms); nodes: 1, quiet: 1, too old: none
```
