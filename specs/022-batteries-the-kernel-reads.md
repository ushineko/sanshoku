# 022 — Batteries the kernel reads

**Issue**: #62

## Status: COMPLETE

## Context

An 8BitDo controller in its Bluetooth (Switch) mode connects as a Nintendo
Switch Pro Controller: HID ID `0005:057E:2009` (bus 0005, Bluetooth), BlueZ
name "Pro Controller". No driver finds its battery.

Measured on the desk (Linux 6.18, BlueZ over the system bus):

- BlueZ lists the device as connected and offers no `org.bluez.Battery1`.
  BlueZ adds that interface for a Bluetooth LE device with the Battery Service
  and for a headset over the hands-free and AVRCP profiles; a classic
  Bluetooth HID gamepad gets neither. The `bluez` driver (spec 004) lists only
  devices with a Battery1 level, so it passes this one over.
- The kernel reads it. `hid-nintendo` publishes
  `/sys/class/power_supply/nintendo_switch_controller_battery_0005:057E:2009.<n>`
  with `type=Battery`, `scope=Device`, `present=1`, `status=Discharging`,
  `capacity_level=High`, and no `capacity` file.
- The node's `device` link is the HID device
  (`/sys/devices/virtual/misc/uhid/0005:057E:2009.<n>`), whose `driver` is
  `nintendo` and whose `uevent` carries `HID_ID`, `HID_NAME=Pro Controller`
  and `HID_UNIQ`, the controller's Bluetooth address in lower case. BlueZ's
  `Address` for the same device is the same address in upper case.

Nothing in the module reads `/sys/class/power_supply`. The "generic Bluetooth
battery" has meant whatever BlueZ has. Kernel drivers publish device batteries
for more than this controller (xpadneo for Xbox pads, `hid-generic` for any
HID device that reports a battery strength), and some of them read devices
this module already reads another way: `hid-playstation` (the `sony` driver,
spec 017), `logitech-hidpp-device` (`logitech`), `hid-generic` on a Bluetooth
LE device (`bluez`, through Battery1).

So the driver reads every device battery the kernel publishes, and a scan
drops the kernel's reading of a device another driver already reads. That
takes a key two drivers can compare, which the module does not have:
`Identity.Phys` is the device's Bluetooth address for `bluez` and `apple`,
but `HID_PHYS` for the hidraw drivers, which for a Bluetooth device is the
host adapter's address.

Measured for the key, on the desk:

- One USB device is several kernel HID devices: three interfaces on a
  Logitech receiver, two each on a SteelSeries Arctis Nova Pro Wireless, a
  Keychron K4 HE and a Corsair MM700. The kernel's battery node can hang off
  one interface while a driver opens another, so the HID device is too fine a
  key.
- `HID_UNIQ` over USB is the serial string, and it is empty on the Logitech
  receiver, the Arctis and the Keychron. It is not a key over USB.
- Over Bluetooth, `HID_UNIQ` and BlueZ's `Address` name the same device,
  compared without case.

## Requirements

### The key

- R1 A device key: a string field on `Candidate` that is kept for matching
  and never displayed, like `Phys`. Two kinds, by bus:
  - Bluetooth: `bt:` and the device address in lower case. For a kernel HID
    device on bus 0005, the address is `HID_UNIQ`; for `bluez` and `apple`,
    BlueZ's `Address`.
  - USB: `usb:` and the sysfs name of the USB device (the port chain, as
    `1-13.3.4.3.4`), the ancestor every interface of that device shares, and
    every device behind a receiver plugged into it.
  - Anything else, or a HID device whose key cannot be read: no key.
- R2 The key is compared only within one `Scan`. It is not an identity across
  scans: the port chain changes when a device moves ports, and nothing
  remembers it.
- R3 One function computes the key of a kernel HID device from sysfs, used by
  `hidraw` (so every hidraw-based driver's candidates carry it) and by the new
  driver. `bluez` and `apple` set theirs from the address they already hold.
- R4 The key never appears in an error message, the testbench's output, a
  fixture (fixtures use `AA:BB:CC:DD:EE:FF` and invented port chains) or a
  log line.

### The driver

- R5 A `powersupply` package with a driver named `powersupply`. Linux only: on
  every other platform `Find` returns `sanshoku.ErrAbsent` and opens nothing.
- R6 `Find` returns one candidate per node under `/sys/class/power_supply`
  with `type=Battery`, `scope=Device` and `present=1` whose `device` link is a
  HID device. Identity from the HID device's `uevent`: vendor and product from
  `HID_ID`, name from `HID_NAME`, bus from `HID_ID`'s bus field; `Path` is the
  HID device's sysfs directory, not the node's, because `hid-playstation`
  names its node `ps-controller-battery-<address>`. The candidate carries the
  HID device's key (R1).
- R7 The reading: `capacity` (0–100) as `Level` when the node has it;
  otherwise `capacity_level` as a `Band`, never converted into a percentage:
  `Full`→Full, `High`→Good, `Normal`→Good, `Low`→Low, `Critical`→Critical
  (Q1, decided).
  `status` `Charging`→Charging, `Full`→Full, anything else→Discharging.
  `capacity_level=Unknown`, or neither file, is no reading.
- R8 A read finds the node again by key, not by name, so a controller that
  reconnects under a new HID instance number is read without a rescan. A node
  that has gone is no reading, not an error.
- R9 Kind: `KindGamepad` when the HID device's kernel driver is one of
  `nintendo`, `xpadneo`, `playstation`, `sony`; `KindOther` otherwise,
  including `microsoft`, which binds Microsoft's keyboards and mice as well as
  Xbox pads.
- R10 Sysfs files only, read only, each read bounded by the context; no
  goroutine, no subprocess, nothing written.

### The scan

- R11 A driver may declare itself a fallback (an optional interface on
  `Driver`). `Scan` drops a fallback candidate whose key equals the key of a
  candidate from a driver that is not a fallback. A candidate with no key is
  never dropped and never drops anything. `powersupply` is the one fallback.
- R12 Dropping is per scan and whole-candidate: under a Logitech receiver,
  every kernel battery is dropped while `logitech` lists that receiver, even
  for a paired device `logitech` then fails to read. The specific driver's
  reading wins (proposed to the user, not objected to).

### The rest

- R13 `all.Drivers` and `all.Support` include it; a `support.Entry` for the
  Switch Pro Controller (`057e:2009`) at Tested once benched, and a
  driver-wide Expected entry; `make generate`; README "What is in it" row and
  changelog line (the new `Candidate` field and `Driver` interface named as
  additive API); CLAUDE.md's list of opened nodes gains
  `/sys/class/power_supply`.

## Acceptance Criteria

- [x] The approach probe (below) runs on the desk before the driver is
  written, and its output is recorded in Verification.
- [x] Key tests against a sysfs fixture under `t.TempDir()`: two interfaces of
  one USB device have one key; a device behind a hub has its full port chain;
  a Bluetooth HID device's key equals the key `bluez` builds from the same
  address in upper case; a HID device on another bus has no key.
- [x] Scan tests: a fallback candidate sharing a key with a specific one is
  dropped; one with a different key is kept; one with no key is kept; two
  specific candidates sharing a key are both kept; a scan with only the
  fallback driver drops nothing.
- [x] Decoder tests: a level-only node reads as a band, each level as R7; a
  node with `capacity` reads as a level and no band; each `status` as R7;
  `Unknown` and neither file are no reading.
- [x] Find tests: `scope=System` and `type=Mains` nodes and a node whose
  device is not HID are not candidates; no node is `ErrAbsent`; a node that
  goes away between Find and a read is no reading, and one back under a new
  instance number is read again (R8).
- [x] No error message or bench line carries a key or `HID_UNIQ`.
- [x] On the desk: `sanshoku-bench read` lists the Pro Controller under
  `powersupply` with a band, a state and `gamepad`; with a DualSense on
  Bluetooth as well, the DualSense appears under `sony` only.
- [x] On Windows and darwin `Find` returns `ErrAbsent`; vet passes for linux,
  darwin and windows.
- [x] hayami, at its next sanshoku bump, shows the controller in its battery
  section, on a screenshot (checked there, recorded here).
- [x] `go test ./...`, golangci-lint for linux and windows, and the generated
  docs are clean.

## Decisions

- **Q1 `Normal`→Good.** hid-nintendo publishes the controller's five steps
  one to one (`joycon_parse_battery_status`): empty→`Critical`, low→`Low`,
  medium→`Normal`, high→`High`, full→`Full`, and `Band` has four. `Normal`
  goes to Good so Low keeps meaning "charge it soon". Decided with the user;
  the alternative, `Normal`→Low, warned a step earlier.

## Approach probe

Before the driver, a throwaway test (outside the repository) runs the real
`sanshoku.Scan(all.Drivers())`, computes R1's key for every candidate and for
every `scope=Device` battery node, and prints which nodes would be kept and
which dropped, with keys hashed. It confirms on hardware that:

- the Pro Controller's node has a Bluetooth key, that key equals the one from
  BlueZ's address, and no specific candidate shares it (kept);
- every hidraw candidate gets a key, and the interfaces of one USB device
  agree;
- where a node duplicates a specific driver's device (a DualSense on
  Bluetooth, if connected), the keys match (dropped).

## Out of scope

- Remembering devices across scans, or a stable device identity: R2.
- Devices that report no battery to the kernel either: the 8BitDo's USB
  dongle (spec 017, Out of scope) is still one.
- Windows. The Switch Pro Controller there is a HID collection with the
  battery in its own input report; reading it is a hidraw driver and a
  separate spec.
- Merging a fallback reading into a specific one (the kernel's level where
  `logitech` failed): R12.

## Risks & Assumptions

- A Bluetooth LE device with a random address is assumed to reach the kernel
  under the address BlueZ shows on D-Bus, since BlueZ creates the uhid device
  from its own device object. Unmeasured: no such device has a kernel battery
  node on the desk. If it fails, the device is listed twice, once by `bluez`
  and once by `powersupply`, not lost.
- A device that reconnects in the middle of a scan can be listed twice for
  that scan.
- `Candidate` gains a field and `sanshoku` an optional interface: additive at
  v0.x, named in the changelog. Consumers that build `Candidate` literals are
  unaffected by a new field.
- A new node read, `/sys/class/power_supply`, and more reads of `/sys/class/
  hidraw`'s parents: read only. The security list in CLAUDE.md is updated in
  the same commit.
- Rollback: revert, or drop the driver from `all.Drivers`; with no fallback
  driver, `Scan` drops nothing.

## E2E Test Plan

- Linux desk, the 8BitDo paired over Bluetooth in Switch mode:
  `sanshoku-bench read` shows it under `powersupply` with a band. Disconnect
  and read again: absent, no error.
- With a DualSense on Bluetooth as well: listed under `sony` only.
- With the Logitech receiver plugged in and `hid-logitech-hidpp` bound: no
  `powersupply` line for its devices.
- hayami built against the branch: the battery section shows the controller
  as a four-segment band, on a screenshot.

### Prod visibility

Not applicable: a library read on the user's own desk; the bench output is
the observation.

## Verification

### Approach probe, 2026-10-28, Linux desk

A throwaway test ran `sanshoku.Scan(all.Drivers())` and computed R1's key
for every candidate, every `scope=Device` battery node and every kernel HID
device (keys shown hashed):

- The one kernel battery node, `nintendo` / "Pro Controller", has key `bt:…`,
  equal to the key built from BlueZ's `Address` for the connected
  "Pro Controller" (the kernel's address is lower case, BlueZ's upper). No
  specific candidate shares it: **kept**.
- hidraw candidates `logitech` (receiver), `steelseries` (Arctis Nova Pro
  Wireless) and `nzxt` (Kraken Elite V2) each get a `usb:` key.
- All 17 kernel HID devices on the desk reduce to 12 keys, one per physical
  device: the Logitech receiver's three interfaces, and the two each of the
  Arctis, the Keychron K4 HE and the Corsair MM700, share their device's key.
- `hwmon` also lists the controller's node as a hwmon chip
  (`nintendo_switch_controller_battery_…`): power_supply registers one. Not a
  battery candidate, so not a duplicate; noted for consumers that show chips.

Run again with a DualSense paired over Bluetooth: `hid-playstation` published
a second node, its key equal to the `sony` candidate's `bt:` key (and to
BlueZ's address for it): **dropped, read by sony**. The node's name carries
the controller's address, which is why R6's `Path` is the HID device's.

Not exercised: a device behind a receiver, since `hid-logitech-dj` had split
no paired devices off the receiver. It stays an acceptance criterion
(E2E Test Plan).

### Implementation, 2026-10-28, Linux desk

`sanshoku-bench read --driver powersupply`, the fallback alone, so nothing
is dropped:

```
powersupply  Pro Controller (057e:2009)  /sys/devices/virtual/misc/uhid/0005:057E:2009.0019  [tested]
  battery: Pro Controller  Good  discharging  gamepad (0.1 ms)
powersupply  DualSense Wireless Controller (054c:0ce6)  /sys/devices/virtual/misc/uhid/0005:054C:0CE6.001A  [expected]
  battery: DualSense Wireless Controller  75%  discharging  gamepad (0.2 ms)
```

`sanshoku-bench read`, every driver: the DualSense is listed by `sony` only,
the Pro Controller by `powersupply`:

```
logitech  Logitech USB Receiver (046d:c547)  /dev/hidraw13  [tested]
steelseries  SteelSeries Arctis Nova Pro Wireless (1038:12e5)  /dev/hidraw15  [tested]
nzxt  NZXT Kraken Elite V2 (1e71:3012)  /dev/hidraw7  [tested]
sony  DualSense Wireless Controller (054c:0ce6)  /dev/hidraw17  [expected]
powersupply  Pro Controller (057e:2009)  /sys/devices/virtual/misc/uhid/0005:057E:2009.0019  [tested]
```

The kernel's DualSense level (75 %) is the one hid-playstation computes from
the same status byte the `sony` driver reads.

Where each criterion is met:

- Keys: `internal/hiddev` (`Key`, `BluetoothKey`), tests in
  `internal/hiddev/hiddev_test.go`; every driver's candidate sets `Key`.
- Scan: `withoutSuperseded` in `sanshoku.go`, tests in
  `scan_fallback_test.go`.
- Driver, decoder, Find, reconnect, errors: `powersupply/driver.go`, tests in
  `powersupply/driver_test.go`; `TestLiveKernelBatteriesArePlausible` ran
  (not skipped) against both controllers.
- Off Linux: `Driver.Find` returns `ErrAbsent` on `runtime.GOOS != "linux"`;
  `go vet` passes for linux, darwin and windows.
- `go test ./...`, `make lint` for linux and windows, `make generate` clean.
- Each test was falsified once (Normal read as Low, a read by node path
  only, Scan dropping nothing, a key that kept its case) and failed.

Lint: gosec's G703 began flagging the two sysfs reads in `hidraw.usbNode`,
unchanged code, once `hidraw` imported `internal/hiddev`; they read a sysfs
directory above a node under `SysRoot` and are marked `nolint` with that
reason.

The receiver case of the E2E plan (a device behind a Logitech receiver) did
not occur on the desk; fixtures cover it (`hiddev_test.go`).

### hayami, 2026-10-28

hayami bumped to 0.1.17 (ushineko/hayami#229). hayami scans one driver at a
time, so `Scan`'s drop never sees `sony` and `powersupply` together; it
applies the same rule itself, asking fallback drivers last and leaving out a
fallback candidate whose key a direct driver listed in the poll. Before that
change the DualSense was one cell only because its Bluetooth name is the same
from both drivers.

- `hayami-tui readings`, DualSense and 8BitDo on Bluetooth: one DualSense,
  75 %; the Pro Controller as band Good, 3 segments, gamepad.
- The panel, on a screenshot with the DualSense disconnected (hayami gives
  one slot per kind, and the DualSense, connected last, held the gamepad
  slot): the Pro Controller in the gamepad slot, three of four segments,
  "Good". Not committed: the shot shows a personal device name.
