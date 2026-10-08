# 012 — HID on Windows

**Issue**: #35

## Status: COMPLETE

## Context

Spec 011 made the module build off Linux, where every driver found nothing.
hayami now runs on Windows 11 and its peripherals section is empty there: on a
desk with a Logitech K800 on a Unifying receiver and a Razer Basilisk Ultimate
on its dongle, `hidraw` enumerates `/sys/class/hidraw` and nothing else.

Measured on Windows 11 (26200) with the plain HID class driver, no vendor
software installed, no driver of this module's own and no administrator
rights, by a probe that sent only the getters the drivers already send:

- **Logitech**: the receiver's HID++ short collection (usage page `0xFF00`,
  usage 1, 7-byte reports) answered HID++ 1.0 register `0x07` at index 1 for
  the K800 (`10 01 81 07 03 00 00`, band low); indices 2–6 answered "not
  reachable".
- **Razer**: `HidD_SetFeature` / `HidD_GetFeature` on interface 0's mouse
  collection (page 1, usage 2, a 91-byte feature report) answered the battery
  and charging getters, on a handle opened with **zero access** — the only
  access Windows grants on a mouse it owns. The Basilisk Ultimate dongle
  (`1532:0088`) read `0xE5` and not charging; the Mouse Dock (`1532:007e`)
  answered status `0x05`, not supported, as it does on Linux with no mouse on
  it.

What differs from Linux, and shapes the backend:

1. **One device path per top-level collection.** Windows splits an interface
   by collection: the receiver's short (`0x10`), long (`0x11`) and very long
   (`0x20`, `0x21`) reports are three paths. A HID++ short request is answered
   with a long report, so a driver must be able to write to one and hear the
   other.
2. **No report descriptor.** Windows gives a program only its parse of one,
   per collection (`HidP_GetCaps`, the value and button capabilities). The
   Razer mouse's 91-byte feature report has a length and no capability at
   all.
3. **No DJ split.** Linux's `hid-logitech-dj` gives the K800 a node of its own
   (`046d:2010`); Windows does not, and the Logitech driver deliberately skips
   HID++ 1.0 devices on a receiver node because on Linux the child node reads
   them.
4. **An unnumbered collection's reports carry a leading zero** on Windows in
   both directions; hidraw hands an unnumbered input report over without it.
5. **The BlueZ and Apple drivers dialled D-Bus over TCP** on Windows
   (`dial tcp 127.0.0.1:12434: connectex: ... actively refused`) on every
   scan.

Also measured, and recorded so nobody measures it again: a UGREEN BT701
Bluetooth audio transmitter (`0a12:4007`) declares a consumer-control
collection, a telephony headset collection and three Qualcomm vendor
collections (page `0xFF00`, reports 1–9 and 32). None carries a battery usage;
a GET_REPORT for every report ID from 1 to 40, feature and input, is refused;
and in a three-minute listen across the headphones being switched on and
connecting, no collection sent a report and the Windows audio endpoint stayed
active. The vendor channel is Qualcomm's closed protocol and looks like the
firmware-update path, so nothing is sent to it.

## Requirements

- R1 `hidraw` keeps its API on Windows: `Nodes`, `Node`, `Open`, `Handle`
  (`Write`, `Read`, `Drain`, `SetFeature`, `GetFeature`, `Close`), `Exchange`,
  `UsagePage`, `HasReportID`. The Linux enumeration and file handle move to
  `!windows` files unchanged in behaviour; the Windows backend is
  `*_windows.go`, through hid.dll and cfgmgr32.dll with `golang.org/x/sys`
  only. No cgo, no new module dependency.
- R2 One `Node` per interface: collections grouped by the device instance they
  hang off (`CM_Get_Parent`). `Path` is the first collection's path; `Phys`
  the parent instance (never printed: it can carry a serial or an address);
  `Name` the manufacturer and product strings joined and undoubled as the
  kernel's `HID_NAME` is; `Bus` from the enumerator.
- R3 `Node.Reports []Report{Kind, ID, Page, Usage, Len}` on both platforms:
  walked from the descriptor on Linux (with Push/Pop and extended usages), and
  from the capabilities on Windows, where a length with no capabilities is
  recorded as an unnumbered report on the collection's own page and usage.
  `UsagePage` and `HasReportID` walk the descriptor where there is one, so the
  Linux match is unchanged, and read `Reports` where there is none.
- R4 `Handle` on Windows: each collection opened read-write, or with no access
  where Windows refuses to share it. A write is routed by report ID to the
  collection that declares it and padded to its output length; an unnumbered
  collection takes the hidraw framing (a leading zero is the "no ID" byte; any
  other first byte is data and gets the zero in front). A read keeps one
  overlapped read waiting per readable collection and returns the first to
  finish; the others stay waiting. An unnumbered input report has its leading
  zero taken off. Feature reports go as overlapped IOCTLs with a deadline to
  the collection that declares them, or the first whose feature length holds
  the report. Unplugging is `sanshoku.ErrGone`: directly for
  `ERROR_DEVICE_NOT_CONNECTED`, `ERROR_NO_SUCH_DEVICE`, `ERROR_DEVICE_REMOVED`;
  for `ERROR_GEN_FAILURE` and `ERROR_OPERATION_ABORTED` only once the
  collection is no longer listed (the Linux rule for EIO).
- R5 `hidraw.SplitsReceivers` (true on Linux, false elsewhere). Where it is
  false the Logitech driver reads a HID++ 1.0 device on a receiver node at
  indices 1–6 and names it from the receiver's pairing register (long register
  `0xB5`, sub-register `0x40 + index − 1`), falling back to "Logitech", never
  the receiver's name. Linux behaviour is unchanged.
- R6 The Razer match needs no change: interface 0's collection declares a
  vendor input on page `0xFF00` (measured: usage `0x40`), so `UsagePage(0xFF00)`
  over its Reports finds it, and its feature report is routed by length.
- R7 `bluez.Devices` returns `ErrNoBlueZ` off Linux without dialling.
- R8 `support.Entry.Windows *support.Port` (tier, date, notes); docs/devices.md
  and `sanshoku-bench support` gain a Windows column; the UGREEN BT701 goes in
  the page's out-of-scope prose with what was measured.
- R9 Tests: grouping, capability fallback, bus, routing, merged reads,
  unframing, feature routing and unplugging on Windows with a fake channel
  over real Windows events; predicates over Reports on every platform; the
  descriptor walk over Reports on Linux. The sysfs-tree `Find` tests of nzxt
  and steelseries skip on Windows with the reason.
- R10 README (intro, the hidraw row, the platform paragraph, changelog),
  `.claude/CLAUDE.md` (the platform rule), the hidraw package doc, regenerated
  docs/api.md and docs/devices.md.

## Acceptance Criteria

- [x] `go test ./...` passes on Windows 11.
- [x] `go vet ./...` passes for `GOOS=windows`, `linux` and `darwin`.
- [x] The pinned golangci-lint (v2.12.2, Go 1.26.0) reports 0 issues for
  Windows and for `GOOS=linux`.
- [x] docs/api.md and docs/devices.md are regenerated from the code.
- [x] Falsified: the write-routing tests fail with routing replaced by "first
  numbered collection"; the merged-read test fails with only the first
  collection waited on. Both restored.
- [x] The bench reads the Basilisk Ultimate through its dongle on Windows,
  through the driver, with no vendor software.
- [ ] The bench reads the K800 through the driver on Windows. Not done: the
  Unifying receiver left the desk before the driver was ready (see Gaps).
- [ ] `make test` and `make lint` pass on Linux (CI): the Linux-only tests
  (the descriptor walk over Reports, the sysfs enumeration) were run once on
  Windows with their build constraint lifted; CI runs them for real.

## Risks & Assumptions

- **Linux behaviour is unchanged** by construction: the enumeration and file
  handle moved file without changing, the predicates walk the descriptor
  whenever there is one, and `SplitsReceivers` is true there. The one new
  Linux behaviour is that `Node.Reports` is filled.
- **The collection grouping assumes a USB device's collections share a parent
  instance**, which is how the HID class driver enumerates them; a collection
  whose parent cannot be read stands alone.
- **Pinned buffers.** An overlapped read's buffer and OVERLAPPED are pinned
  with `runtime.Pinner` while the kernel holds them, and close cancels and
  waits before releasing them.
- **Another program reading the same collection** gets its own copy of every
  input report (Windows keeps a queue per handle), as on Linux; a feature
  exchange can still interleave with another program's, as docs/contention.md
  says for Linux.
- **Rollback**: revert. Consumers pin a module version; v0.1.7 is the previous
  behaviour on every platform.

## Gaps found

- **The K800 through the driver is unmeasured.** The probe read register 0x07
  through the receiver; by the time the driver could, the receiver had been
  unplugged for an AULA keyboard's. Its Windows entry is Expected with that
  note, and the pairing-register name is tested against a fake only.
- `sanshoku-bench scan` exits 1 on Windows because the `bluez` and `apple`
  drivers report `ErrUnavailable`, as it does on a Linux machine without
  BlueZ. A per-platform driver list belongs to the consumer (hayami spec 035);
  the bench may want one too.
- A Razer dongle whose mouse is asleep or docked answers status `0x04`
  (timeout), which the driver reads as no reading. Seen on Windows here; it is
  the same on Linux.

## Verification

2026-10-07, Windows 11 Pro 26200, Go 1.27.0 (lint under Go 1.26.0):

- `go test ./...`: every package ok.
- `go vet ./...` for windows, linux, darwin: clean.
- golangci-lint v2.12.2: 0 issues on Windows and with `GOOS=linux`.
- `TestReportsOfReadsEachReportOnce` (Linux-only) run once on Windows with its
  constraint lifted: pass.
- Live, `sanshoku-bench read --driver razer`, no vendor software installed:

  ```
  razer  Razer Mouse Dock (1532:007e)  \\?\HID#VID_1532&PID_007E&MI_00#…  [expected]
    battery: no reading (359.1 ms)
  razer  Razer Basilisk Ultimate Dongle (1532:0088)  \\?\HID#VID_1532&PID_0088&MI_00#…  [tested]
    battery: Basilisk Ultimate Dongle  77%  discharging  mouse (141.1 ms)
  ```

  Later the same day the dongle answered status `0x04` to every transaction
  (mouse asleep or on its dock) and the bench read "no reading", which is the
  driver's answer for it.
- `sanshoku-bench scan` lists both Razer nodes, one per interface 0; the
  BlueZ and Apple drivers report "BlueZ is a Linux service" without dialling.
