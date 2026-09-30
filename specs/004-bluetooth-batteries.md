# 004 — Bluetooth batteries: BlueZ Battery1 and Apple AAP

**Issue**: #4

## Status: INCOMPLETE

## Context

hayami reads Bluetooth batteries two ways (`internal/peripherals/bluez.go`,
`bluetooth.go`, `l2cap.go`, `aap.go` at 54d357c; spec 009): every connected
device that exposes `org.bluez.Battery1` on the system bus, and AirPods
through Apple's Accessory Protocol over an L2CAP socket on PSM 0x1001, which
gives per-ear and case levels that BlueZ does not. AAP was chosen over
BlueZ's experimental battery provider because it needs no daemon setting.
The L2CAP dial has one measured trap: the address goes in written order
because `x/sys` reverses it, and reversing it yourself gives
`ECONNREFUSED`.

This spec ports the two as the `l2cap` and `bluez` transport packages, the
`bluez` generic-battery driver and the `apple` driver. The BlueZ D-Bus
listing is a transport the way sysfs is: it is how a Bluetooth driver finds
its candidates.

## Requirements

### R1. `l2cap` package

- R1.1 `Dial(ctx, addr [6]byte, psm uint16) (*Conn, error)`:
  `AF_BLUETOOTH`, `SOCK_SEQPACKET`, `BTPROTO_L2CAP`; non-blocking connect,
  `EINPROGRESS` awaited with `poll` on `POLLOUT` then `SO_ERROR`; `EINTR`
  retried with the deadline recomputed. The address is passed in written
  order, with hayami's comment.
- R1.2 `(*Conn).Send([]byte) error`, `Receive(ctx) ([]byte, error)`,
  `Close`. `Receive` honours the context deadline through `poll`.
- R1.3 `ParseAddress(s string) ([6]byte, error)`.

### R2. `bluez` package

- R2.1 `Devices(ctx) ([]Device, error)` connects to the system bus, calls
  `org.bluez` `/` `ObjectManager.GetManagedObjects`, and returns every
  `org.bluez.Device1` with `Connected` true: `Device{Name, Address string;
  Level int; HasLevel bool; Apple, Audio bool; Kind battery.Kind}`. Name
  from Alias then Name; Apple when Modalias contains `v004C`; Audio when
  Icon starts with `audio-` or the UUIDs include 0000110b/0000110d; Kind
  from hayami's icon map; Level from `org.bluez.Battery1.Percentage`
  (several integer widths). `ErrNoBlueZ` wraps `sanshoku.ErrAbsent`.
- R2.2 `bluez.Driver{}`; `Name()` is `"bluez"`. `Find` lists every
  connected device with `HasLevel` as a candidate with `BusBluetooth`, the
  `Identity.Path` being the D-Bus object path and `Name` the alias.
  `Open` returns a `Device` satisfying `battery.Source` whose `Batteries`
  re-reads `Battery1.Percentage` for that path. An Apple audio device is
  **not** listed by this driver; it belongs to `apple` (R3), which falls
  back to Battery1 itself.
- R2.3 `github.com/godbus/dbus/v5` is added to `go.mod` by this spec and is
  the second and last runtime dependency.

### R3. `apple` driver (AAP)

- R3.1 `apple.Driver{Timeout time.Duration}` (default 6 s, hayami
  `AAPTimeout`). `Name()` is `"apple"`. `Find` lists every connected BlueZ
  device that is Apple and Audio.
- R3.2 `Open` returns a `Device` satisfying `battery.Source`. `Batteries`
  dials PSM 0x1001, runs the handshake, and on any failure falls back to the
  device's Battery1 level as hayami's `read` does.
- R3.3 Handshake, from `aap.go` with its byte strings: send
  `00000400010002000000000000000000`; on ack `01000400` send
  `040004004d00d700000000000000`; on ack `040004002b00` send
  `040004000f00ffffffffff`; wait for a packet with prefix `040004000400`,
  ignoring other chatter.
- R3.4 Exported `DecodeBattery(packet []byte) ([]battery.Cell, error)`:
  six-byte prefix, a count, N five-byte records `[cell, const, level,
  status, const]`; cells 0x01 headset, 0x02 right, 0x04 left, 0x08 case;
  status 0x01 charging, 0x02 draining, 0x04 not on body (dropped). Cells
  sorted left, right, case. The battery's level is the minimum of the ears,
  ignoring the case. `ErrNoBatteryPacket` is silence, swallowed.

### R4. Tests

- R4.1 `DecodeBattery` table from hayami `aap_test.go`, including a
  not-on-body record and an empty packet.
- R4.2 One fake channel carrying: the three-step handshake, chatter before
  the battery packet, and a refused dial (fallback to Battery1).
- R4.3 `bluez.Devices` with `DBUS_SYSTEM_BUS_ADDRESS` pointing at a
  nonexistent socket returns `ErrNoBlueZ` (hayami's test).
- R4.4 `live_test.go`: skips without BlueZ or without a connected device;
  AirPods over AAP when present. Failure messages never print an address
  (hayami's assertion policy, kept as a test that greps its own output).

### R5. Documentation

- R5.1 `docs/devices.md` is regenerated from the entries in R5.3. hayami
  issue #24 notes the Battery1 path was never verified on hardware; the
  tier says so until the bench has.
- R5.2 README, changelog, `all.Drivers()`. `docs/udev.md` already says
  Bluetooth needs no rule.
- R5.3 `apple.Support()`: `Tested` for the AirPods generation measured
  (named as the product reports it), `Expected` for other AAP accessories.
  `bluez.Support()`: `Expected` for "any connected device with Battery1",
  promoted to `Tested` only if the bench confirms it on a real device
  during this spec.

## Acceptance Criteria

- [ ] `make test` and `make lint` pass with no BlueZ on the machine.
- [ ] `DecodeBattery` reproduces hayami's expected cells and minimum level.
- [ ] The fake channel test shows the handshake order, chatter tolerance
  and the Battery1 fallback on a refused dial.
- [ ] `bluez.Devices` reports `ErrNoBlueZ` for an unreachable bus.
- [ ] `sanshoku-bench read --driver apple` with AirPods connected reports
  left, right and case; `--driver bluez` reports any other connected device
  that has a level. Output pasted below, addresses absent from it.
- [ ] Both `Support()` tables exist and `make check-support` passes;
  README and `all` updated in the same commit.

## Risks & Assumptions

- **Battery1 generic path unverified** on real hardware in hayami; the bench
  run is the first verification and the spec records whether it worked.
- **AAP is Apple's private protocol**, measured on one pair of AirPods; a
  different generation may differ, and the driver withholds rather than
  guesses.
- **Rollback**: revert.

## Verification

Bench output goes here when the spec is done.
