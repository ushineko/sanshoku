# 017 — DualSense batteries, and a kind for game controllers

**Issue**: #47

## Status: COMPLETE

## Context

A DualSense (054c:0ce6) paired over Bluetooth to a Windows 11 desk had no
battery anywhere Windows exposes one: spec 016's Bluetooth battery property is
absent for it. The controller reports its battery in its own input report,
which is where the Linux kernel's hid-playstation and SDL read it.

There was also no `battery.Kind` for a game controller, so a consumer laying
devices out by kind (hayami #180) could not tell one from anything else.

Measured on the desk, through `hidraw` on Windows:

- In its default Bluetooth mode the controller sends only the short report
  0x01: 20 of 20 reports read, none carrying a battery. Windows pads it to 78
  bytes, the collection's input length, so it is the same length as the long
  report.
- Reading feature report 0x05 (41 bytes) switched it: 39 of the next 40
  reports were 0x31, with the status byte at offset 54 reading 0x07, level 7,
  discharging (75 % by hid-playstation's rule).

## Requirements

- R1 `battery.KindGamepad`. BlueZ's `input-gaming` icon and the Windows class
  of device's joystick and gamepad minor classes map to it.
- R2 A `sony` driver: product allow-list 054c:0ce6 (DualSense) and 054c:0df2
  (DualSense Edge), found by vendor and input report 0x01 on Generic Desktop.
  A Sony product not on the list is a candidate that refuses to open.
- R3 Over USB, the battery is the status byte of report 0x01 at offset 53;
  over Bluetooth, of report 0x31 at offset 54. The connection decides, because
  0x01 is the long report over USB and the short one over Bluetooth.
- R4 Over Bluetooth, feature report 0x05 is read once per open handle to turn
  the long report on, and again after a reading that timed out.
- R5 Level is `min(tenths*10+5, 100)`; charge state 0 discharging, 1
  charging, 2 full (100 %); fault states (0xA, 0xB, 0xF) are no reading.
- R6 Support entries, udev rules (by product through the HID ID, which covers
  Bluetooth), credits, README.

## Acceptance Criteria

- [x] The bench reads the DualSense over Bluetooth on Windows: "DualSense
  Wireless Controller 75% discharging gamepad".
- [x] Decoder tests: the measured byte, both links, the top step capped at
  100, charging and full, faults as no reading, a Bluetooth 0x01 padded to 78
  bytes is not read as a level.
- [x] Driver tests: Bluetooth asks for the switch once per handle; USB asks
  for nothing; silence is no reading and asks again; an unlisted product is
  never opened.
- [x] The kind: BlueZ `input-gaming` and the DualSense's class of device
  (0x002508) read as `KindGamepad`.
- [x] `go test ./...`, vet for linux, darwin and windows, golangci-lint for
  windows and linux, and the generated docs are clean.

## Out of scope

- The 8BitDo Ultimate Wireless controller's USB dongle (2dc8:3106 / 3109):
  research found no public protocol that carries its battery, and the Xbox 360
  wired protocol it speaks has none. SDL reads battery from some later 8BitDo
  pads in DInput mode (`SDL_hidapi_8bitdo.c`), not this dongle.
- DualShock 4: the same family, a different report; not on a desk.

## Risks & Assumptions

- Reading feature report 0x05 changes no setting, but the controller then
  sends report 0x31 until it reconnects. A program reading it as a generic
  gamepad through report 0x01, as a DirectInput game without Steam does on
  Windows, loses its input until then. Steam and the Linux kernel make the same
  request on connection, so on Linux and wherever Steam runs nothing changes.
  Decided with the user: the driver makes the request.
- On Linux the kernel already reads the battery (hid-playstation's
  power_supply). If BlueZ also offered a Battery1 for the controller, the
  `bluez` driver would list it too. Not seen on Windows (no property there);
  unmeasured on Linux.
- Additive: a new package and a new kind. Rollback: revert.

## Verification

2026-10-09, Windows 11, a DualSense over Bluetooth:
`sanshoku-bench read --driver sony` → "sony  DualSense Wireless Controller
(054c:0ce6) … [tested]  battery: DualSense Wireless Controller  75%
discharging  gamepad (110.3 ms)". The live test passed against it.
