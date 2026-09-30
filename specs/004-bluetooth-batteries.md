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
  order, with hayami's comment. The socket is made blocking again once
  connected, as hayami's was for its reads and writes. A context with no
  deadline is given six seconds (hayami `AAPTimeout`). No error names the
  address.
- R1.2 `(*Conn).Send([]byte) error`, `Receive(ctx) ([]byte, error)`,
  `Close`. `Receive` honours the context deadline through `poll`; nothing
  by the deadline is an error wrapping `context.DeadlineExceeded`.
- R1.3 `ParseAddress(s string) ([6]byte, error)`. Unlike hayami's
  `parseAddress`, the error does not quote the input, which may be most of
  an address, and each field must be two hex digits.

### R2. `bluez` package

- R2.1 `Devices(ctx) ([]Device, error)` connects to the system bus, calls
  `org.bluez` `/` `ObjectManager.GetManagedObjects`, and returns every
  `org.bluez.Device1` with `Connected` true: `Device{Path, Name, Address
  string; Vendor, Product uint16; Level int; HasLevel bool; Apple, Audio
  bool; Kind battery.Kind}`. `Path` is the D-Bus object path; `Vendor` and
  `Product` are parsed from the Modalias (`bluetooth:vXXXXpYYYY…` or
  `usb:vXXXXpYYYY…`, hex), zero without one. Each call opens its own
  system-bus connection with the context and closes it (hayami used the
  shared `dbus.SystemBus()`, which cannot take a context). Name
  from Alias then Name; Apple when Modalias contains `v004C`; Audio when
  Icon starts with `audio-` or the UUIDs include 0000110b/0000110d; Kind
  from hayami's icon map; Level from `org.bluez.Battery1.Percentage`
  (several integer widths). `ErrNoBlueZ` wraps `sanshoku.ErrAbsent`.
- R2.2 `bluez.Driver{}`; `Name()` is `"bluez"`. `Find` lists every
  connected device with `HasLevel` as a candidate with `BusBluetooth`, the
  `Identity.Path` being the D-Bus object path, `Name` the alias, `Vendor`
  and `Product` from the Modalias (so Apple keys on 0x004C), and the
  address in `Phys`, which is never printed and never in an error; the
  bench masks the address segment of the object path it prints.
  `Open` returns a `Device` satisfying `battery.Source` whose `Batteries`
  re-reads `Battery1.Percentage` for that path by listing again; a path no
  longer listed is `ErrGone`. An Apple audio device is
  **not** listed by this driver; it belongs to `apple` (R3), which falls
  back to Battery1 itself.
- R2.3 `github.com/godbus/dbus/v5` is added to `go.mod` by this spec and is
  the second and last runtime dependency.

### R3. `apple` driver (AAP)

- R3.1 `apple.Driver{Timeout time.Duration}` (default 6 s, hayami
  `AAPTimeout`, bounding the dial and then the exchange, each, as hayami
  does). `Name()` is `"apple"`. `Find` lists every connected BlueZ
  device that is Apple and Audio.
- R3.2 `Open` returns a `Device` satisfying `battery.Source`. `Batteries`
  dials PSM 0x1001, runs the handshake, and on any failure falls back to the
  device's Battery1 level as hayami's `read` does, listing BlueZ again for
  a current level. With no level, silence is no reading and no error, and
  any other failure (a refused dial) is an error.
- R3.3 Handshake, from `aap.go` with its byte strings: send
  `00000400010002000000000000000000`; on ack `01000400` send
  `040004004d00d700000000000000`; on ack `040004002b00` send
  `040004000f00ffffffffff`; wait for a packet with prefix `040004000400`,
  ignoring other chatter.
- R3.4 Exported `DecodeBattery(packet []byte) ([]battery.Cell, error)`:
  six-byte prefix, a count, N five-byte records `[cell, const, level,
  status, const]`; cells 0x01 headset, 0x02 right, 0x04 left, 0x08 case;
  status 0x01 charging, 0x02 draining, 0x04 not on body (dropped).
  `DecodeBattery` returns the cells in the order sent, as hayami's
  `decodeAAPBattery` does; the reading built from them (hayami's
  `batteryOf`, unexported) sorts them headset, left, right, case. The
  battery's level is the minimum of the ears, ignoring the case, or the
  case's when it is the only cell. `ErrNoBatteryPacket` is silence,
  swallowed.

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
  during this spec. The entry has no vendor and no chips; `support.Lookup`
  gains a fourth rule, after the other three, under which such an entry
  covers every candidate of its driver (spec 001 R9.1 updated).

## Acceptance Criteria

- [x] `make test` and `make lint` pass with no BlueZ on the machine.
- [x] `DecodeBattery` reproduces hayami's expected cells and minimum level.
- [x] The fake channel test shows the handshake order, chatter tolerance
  and the Battery1 fallback on a refused dial.
- [x] `bluez.Devices` reports `ErrNoBlueZ` for an unreachable bus.
- [ ] `sanshoku-bench read --driver apple` with AirPods connected reports
  left, right and case; `--driver bluez` reports any other connected device
  that has a level. Output pasted below, addresses absent from it.
  **Half done**: `--driver bluez` read the Sony WH-1000XM6; no AirPods were
  connected (a pair is paired and was not connected for this run), so
  `--driver apple` found nothing and the AAP path is unverified here.
- [x] Both `Support()` tables exist and `make check-support` passes;
  README and `all` updated in the same commit.

## Risks & Assumptions

- **Battery1 generic path unverified** on real hardware in hayami; the bench
  run is the first verification and the spec records whether it worked.
- **AAP is Apple's private protocol**, measured on one pair of AirPods; a
  different generation may differ, and the driver withholds rather than
  guesses.
- **Rollback**: revert.

## Verification

Run on 2026-09-29 with BlueZ running, one Sony WH-1000XM6 connected, and a
pair of AirPods Pro paired but not connected. Addresses are masked by the
bench itself (`dev_XX_…`); none appears in any output below. A grep of the
full `scan`, `read` and `read --json` output for an address pattern found
none.

`make test` passes, and passes with `DBUS_SYSTEM_BUS_ADDRESS` pointed at a
socket that does not exist (the live tests skip; `TestAnUnreachableBusIsErrNoBlueZ`
passes). `make lint`: 0 issues. `CGO_ENABLED=0 make build` and
`make check-support` pass.

```
$ ./sanshoku-bench scan --driver bluez
DRIVER  DEVICE                  PATH                                   CAPABILITIES  TIER
bluez   WH-1000XM6 (054c:0f8a)  /org/bluez/hci0/dev_XX_XX_XX_XX_XX_XX  battery       tested
1 found

$ ./sanshoku-bench read --driver bluez
bluez  WH-1000XM6 (054c:0f8a)  /org/bluez/hci0/dev_XX_XX_XX_XX_XX_XX  [tested]
  battery: WH-1000XM6  33%  discharging  headset (0.9 ms)

$ ./sanshoku-bench read --json --driver bluez
{"driver":"bluez","name":"WH-1000XM6","vendor":"054c","product":"0f8a","bus":"bluetooth","path":"/org/bluez/hci0/dev_XX_XX_XX_XX_XX_XX","tier":"tested","capabilities":["battery"],"batteries":{"elapsed_ms":0.78,"readings":[{"name":"WH-1000XM6","level":33,"state":"discharging","kind":"headset"}]}}

$ ./sanshoku-bench scan --driver apple
DRIVER  DEVICE  PATH  CAPABILITIES  TIER
0 found

$ ./sanshoku-bench read --driver apple
(no output, exit 0: no Apple audio device connected)
```

Cross-check: `busctl --system call org.bluez / …GetManagedObjects` gave the
headset's `org.bluez.Battery1.Percentage` as 33 and its Modalias as
`usb:v054Cp0F8Ad0300`, which is the level and the 054c:0f8a the bench
reported. The Battery1 path, never verified by hayami (hayami issue #24), is
therefore Tested on this headset, as R5.3 anticipated.

The full `scan` found the Sony beside the hwmon, Logitech, SteelSeries and
NZXT devices already on the desk (17 found, exit 0).

```
$ go test -count=1 -v -run Live ./bluez ./apple
=== RUN   TestLiveBatteryOneReadingsArePlausible
--- PASS: TestLiveBatteryOneReadingsArePlausible (0.00s)
ok  	github.com/ushineko/sanshoku/bluez
=== RUN   TestLiveAirPodsReadingIsPlausible
    live_test.go:36: bluez is not answering, or no Apple audio device is connected
--- SKIP: TestLiveAirPodsReadingIsPlausible (0.00s)
ok  	github.com/ushineko/sanshoku/apple
```

**Not verified**: the accessory protocol over a real L2CAP socket. The AirPods
were not connected and were not connected for this run, because doing so
takes them from whatever they are paired with. `apple` stays Expected until a
bench run with a pair connected; that run ticks the remaining box and adds a
Tested entry for the generation, named as the product reports it.
