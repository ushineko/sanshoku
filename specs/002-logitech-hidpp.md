# 002 — Logitech HID++ batteries

**Issue**: #2

## Status: INCOMPLETE

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
  feature and function through `hidraw.Exchange`.
- R2.5 Retry only on silence, 5 attempts (a mouse idle for 6 s needed four).
  Silence after the last attempt is `errSilent`, swallowed: the reading is
  withheld, not an error.
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

- R5.1 On the first `Batteries` and after any read failure, probe index 0xFF
  (wired) and 1..6 with a `featureIndex(0x1004)` lookup: OK or unknown
  feature → a 2.0 device; old protocol on a paired child → a 1.0 device; not
  reachable → counted as quiet.
- R5.2 Located devices are remembered per `Device` and rediscovered only
  when a read fails.
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

- R8.1 `docs/devices.md` rows: G502 X PLUS via Lightspeed (measured), K800
  via Unifying (HID++ 1.0, measured), "any HID++ 2.0 device with feature
  0x1004 or 0x1000" (by protocol).
- R8.2 README table row and changelog entry. `all.Drivers()` gains
  `logitech.Driver{}`.

## Acceptance Criteria

- [ ] `make test` and `make lint` pass; no test opens a device.
- [ ] The decoder tables reproduce hayami's expected values for Unified
  Battery, Battery Status, register 0x0D and register 0x07.
- [ ] The fake transport test shows a reading survives one solaar reply and
  four silent attempts, and a fifth silent attempt withholds the reading
  without an error.
- [ ] The phantom-name case returns the kernel name, not a truncated one.
- [ ] `sanshoku-bench read --driver logitech` on the development machine
  reports the mouse and keyboard on the desk with plausible levels, and
  `verify` agrees with solaar if installed. Output pasted below.
- [ ] `docs/devices.md`, README and `all` updated in the same commit.

## Risks & Assumptions

- **Contention with solaar and the desktop applet** is handled by the
  software ID only. Two processes asking the same receiver at once can each
  see the other's silence; the retry covers it in practice.
- **Rollback**: revert; no consumer yet.
- The K800 is the only 1.0 device measured. Register 0x0D's absence path is
  measured on it; other 1.0 devices are by protocol.

## Verification

Bench output goes here when the spec is done.
