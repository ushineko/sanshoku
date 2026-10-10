# 016 — Bluetooth batteries on Windows

**Issue**: #46

## Status: IN PROGRESS

## Context

On Windows the `bluez` driver found nothing by design (spec 012): BlueZ is a
Linux service. A Bose QC35 paired directly to a Windows 11 desk showed its
battery in Windows' own Bluetooth settings and was absent from hayami's
Peripherals card.

Windows already holds the level. The Bluetooth stack writes what it knows about
a paired device into the properties of the device nodes it creates: one node
for the device (`BTHENUM\DEV_<address>\...`) and one per profile it offers. On
the QC35 the battery is on the Hands-Free AG profile node, as a byte, under the
undocumented key `{104EA319-6EE2-4701-BD47-8DDBF425BBE5} 2` that Windows'
settings page reads. It was 80, and 90 an hour later; Windows refreshes it.
The device node carries the name, the class of device and whether a link is up;
all the nodes of one device share a container ID.

## Requirements

- R1 `bluez.Devices` on Windows lists connected Bluetooth devices from the
  device properties, through setupapi, with no cgo and no subprocess. Off Linux
  and Windows it is `ErrNoBlueZ` as before.
- R2 One `Device` per connected device node, its level taken from whichever
  node of the same container carries the battery property. A device whose own
  node is not connected is not listed, because Windows keeps a paired device's
  nodes and last level while it is switched off.
- R3 The kind comes from the Bluetooth class of device: worn audio (headset,
  hands-free, headphones) is `KindHeadset`; a peripheral's keyboard bit
  `KindKeyboard`, its pointing bit `KindMouse`; anything else `KindOther`.
- R4 No radio is `ErrNoBlueZ`, which wraps `sanshoku.ErrUnavailable`, as a
  machine with no Bluetooth is on Linux.
- R5 `Apple` is false on Windows: the accessory protocol is L2CAP, which is
  read on Linux only, so Apple audio is not withheld from this driver there.
- R6 The `bluez` driver describes itself as reading on Linux and Windows; its
  support entry gains a Windows standing from a bench reading.
- R7 The bench masks the address in a Windows instance ID, which carries it
  twice and unseparated.

## Acceptance Criteria

- [x] The bench reads the QC35 on Windows at the level Windows' settings show:
  "qc35 90% discharging headset".
- [x] Unit tests (no build tag, so they run on Linux CI) hold the join: the
  battery on a profile node is the device's, a device not connected is not
  listed, containers keep devices apart, a level outside 0-100 is no level, and
  the class-of-device decoding gives the kinds above.
- [x] The bench prints the instance ID with both copies of the address masked;
  a test holds it.
- [x] hayami built against this branch reads the QC35 as a live headset.
- [ ] Switching the QC35 off takes it off the list (the connected property
  reads false on its device node).
- [x] `go test ./...`, vet for linux, darwin and windows, golangci-lint for
  windows and linux, and the generated docs are clean.

## Out of scope

- The DualSense on the same desk: Windows gives it no battery property. Issue
  #47.
- A kind for game controllers. Issue #47.
- LE devices: the `BTHLE` and `BTHLEDEVICE` enumerators are read and joined the
  same way, but no LE device with a battery was on the desk, so whether Windows
  puts their level under the same key is unmeasured.

## Risks & Assumptions

- The battery key is undocumented. It is what Windows' Settings and Device
  Manager show, and its absence is no level rather than an error, so a Windows
  release that moved it would make devices lose their level, not fail.
- `{83DA6326-97A6-4088-9453-A1923F573B29} 15` is taken as "connected" from
  reading true on two connected devices' nodes and false on their profile
  nodes; the switched-off reading is the open criterion above.
- Additive on Linux: the D-Bus path is unchanged. Rollback: revert.

## Verification

2026-10-09, Windows 11, a Bose QC35 and a DualSense paired over Bluetooth:
`sanshoku-bench read --driver bluez` → "bluez  qc35 (009e:400c)
BTHENUM\DEV_XXXXXXXXXXXX\…&BLUETOOTHDEVICE_XXXXXXXXXXXX  [tested]  battery:
qc35  90%  discharging  headset". The DualSense was listed by `bluez.Devices`
with no level (054c:0ce6) and so is not a candidate.
