# sanshoku (三色)

[![Go Reference](https://pkg.go.dev/badge/github.com/ushineko/sanshoku.svg)](https://pkg.go.dev/github.com/ushineko/sanshoku)

**Version**: 0.1.9

Direct device access for Linux, and for HID devices on Windows, in Go, without
cgo: peripheral batteries over HID++ and vendor report protocols, AirPods over
Bluetooth, an NZXT Kraken's telemetry and LCD, and hwmon temperatures. It is
the maintained home of the device code that
[hayami](https://github.com/ushineko/hayami) and
[hotaru](https://github.com/ushineko/hotaru) each wrote for themselves.

Three colours, for the range of things under one roof.

> **Status**: v0.1.9. Every Linux driver but aula is bench-tested on real hardware:
> Logitech HID++ 1.0 and 2.0, Razer, SteelSeries keyboards and the Arctis Nova
> Pro Wireless, NZXT Kraken Elite telemetry and LCD, BlueZ `Battery1`, AirPods
> and hwmon. On Windows the HID drivers read through the HID class driver: the
> Razer and AULA drivers are bench-tested there. hayami imports the module.

## Contents

- [Why](#why)
  - [Another project already does this](#another-project-already-does-this)
- [What is in it](#what-is-in-it)
- [What it does not do](#what-it-does-not-do)
- [Using it](#using-it)
- [Devices](#devices)
- [Testbench](#testbench)
- [Documentation](#documentation)
- [Development](#development)
- [Licence](#licence)
- [Changelog](#changelog)

## Why

A desktop panel that shows a mouse's battery, a cooler's coolant and a
headset's charge used to need three tools from three languages behind it:
liquidctl (Python) for the cooler, headsetcontrol (C) for the headset, a
Python interpreter started per poll for each, and the panel's own hidraw
code for the rest. Two programs by the same author carried that code as
copies. This module is the one copy: native Go, no cgo, no daemon, no
subprocess, one static binary a consumer links, and one bench that proves
every driver against the hardware.

An off-the-cuff footprint comparison, measured on the desk it was written
at (Arch, `pacman -Qi`; shared packages counted once; Python is 75 MiB
whether or not anything else needs it):

| What reads the devices | On disk | Runs as | Per poll |
|---|---|---|---|
| liquidctl + Python + PyUSB, hidapi, Pillow, docopt, colorlog | ≈ 84 MiB | a Python interpreter per call | ≈ 105 ms to start it |
| solaar + Python + GTK stack | ≈ 82 MiB before GTK | a daemon and a GTK app | one `solaar show` per poll, ≈ 3.5 s |
| headsetcontrol + hidapi | ≈ 0.9 MiB | a process per call | a fork per poll |
| sanshoku, as `sanshoku-bench`, static | 6.6 MB | in the consumer's process | one hidraw exchange, 1–80 ms |

The point is not the bytes, though 84 MiB for a temperature is a lot; it is
that a panel polling four devices every fifteen seconds was forking Python
to do it, and that two programs had to agree by hand on what the devices
say. OpenRGB stays: it knows every lit device on the machine, and that
breadth is the one thing not worth rewriting.

### Another project already does this

Usually, yes, and [docs/credits.md](docs/credits.md) names each one. Every
protocol here was learned from a project that got there first. The module
exists anyway, for three reasons:

- **Go-native is the design goal, not an accident.** The consumers (hayami,
  hotaru) are Go programs that ship as one binary. A device read that
  needs a Python interpreter, a C library through cgo, or a daemon to be
  running is a build, packaging and deployment problem for each of them;
  a Go package that speaks to the kernel node is not. `CGO_ENABLED=0 go
  build` is the whole toolchain.
- **One copy, proved on hardware.** Two programs carried the same device
  code. Here it is written once, with a bench that runs every driver
  against the real device before a support entry says Tested.
- **Learning the protocols is part of the point.** Reading another
  project's source, capturing the vendor's own software with usbmon, and
  writing the decoder from the bytes is how this module's knowledge was
  built, and it is written down where the code is.

Where an existing project covers something this module cannot reasonably
do in Go, it is used instead. OpenRGB is that case for lighting in general.
It is not, deliberately, for the SteelSeries Apex Pro TKL Wireless Gen 3:

| For the Apex's lighting | What it takes | Effects |
|---|---|---|
| OpenRGB, Direct mode | the server, headless | none: per-key colour only, which is all its Apex controller offers (Direct and Onboard) |
| OpenRGB + [Effects plugin](https://gitlab.com/OpenRGBDevelopers/OpenRGBEffectsPlugin) | OpenRGB's GUI running in the desktop session, because OpenRGB 1.0 loads plugins only from its window, never under `--server`; effects configured in its UI, started and stopped by name over the SDK | about sixty, rendered by the plugin |
| sanshoku `lighting.Canvas` | the consumer's own process | whatever the consumer renders; each frame acknowledged |

The board has no firmware effect to switch to (spec 010 measured that), so
every option renders on the host. The difference is where: in a GUI
application the consumer remote-controls, or in the consumer, through a
handle it already holds for the battery. The second keeps OpenRGB headless
and the effect a product decision of the program that shows it.

## What is in it

| Package | Purpose |
|---|---|
| [`sanshoku`](https://pkg.go.dev/github.com/ushineko/sanshoku) | The vocabulary: `Identity`, `Candidate`, `Device`, `Driver`, `Scan`, the sentinel errors, `Capabilities`. |
| [`hidraw`](https://pkg.go.dev/github.com/ushineko/sanshoku/hidraw) | HID interfaces: sysfs enumeration on Linux, the HID class driver on Windows; report-descriptor walking, report exchange with deadlines, feature reports. |
| [`usbfs`](https://pkg.go.dev/github.com/ushineko/sanshoku/usbfs) | Claim an interface and write a bulk endpoint through raw usbdevfs ioctls. |
| [`hwmon`](https://pkg.go.dev/github.com/ushineko/sanshoku/hwmon) | Temperatures by chip and label; the CPU and GPU sensor tables. |
| [`l2cap`](https://pkg.go.dev/github.com/ushineko/sanshoku/l2cap) | Bluetooth L2CAP sequenced-packet sockets with deadlines. |
| [`bluez`](https://pkg.go.dev/github.com/ushineko/sanshoku/bluez) | Connected devices from the BlueZ system bus; the generic `Battery1` driver. |
| [`battery`](https://pkg.go.dev/github.com/ushineko/sanshoku/battery) | The battery reading and the `Source` capability. |
| [`cooling`](https://pkg.go.dev/github.com/ushineko/sanshoku/cooling) | The cooler reading and the `Source` capability. |
| [`screen`](https://pkg.go.dev/github.com/ushineko/sanshoku/screen) | The `Panel` capability for a device with a display. |
| [`lighting`](https://pkg.go.dev/github.com/ushineko/sanshoku/lighting) | The `Canvas` capability for a device whose lights a program streams frames to. |
| [`logitech`](https://pkg.go.dev/github.com/ushineko/sanshoku/logitech) | HID++ 1.0 and 2.0 batteries over hidraw. |
| [`razer`](https://pkg.go.dev/github.com/ushineko/sanshoku/razer) | Battery through feature reports, including a mouse behind its dock. |
| [`steelseries`](https://pkg.go.dev/github.com/ushineko/sanshoku/steelseries) | Battery over hidraw, with a product allow-list; the Apex Pro TKL Gen 3's lighting frames. |
| [`apple`](https://pkg.go.dev/github.com/ushineko/sanshoku/apple) | AirPods over the Accessory Protocol: left, right and case. |
| [`nzxt`](https://pkg.go.dev/github.com/ushineko/sanshoku/nzxt) | Kraken Elite telemetry and LCD. |
| [`aula`](https://pkg.go.dev/github.com/ushineko/sanshoku/aula) | An AULA keyboard's battery through its 2.4 GHz receiver. |
| [`support`](https://pkg.go.dev/github.com/ushineko/sanshoku/support) | The hardware support table: tested, expected, listed. |
| [`all`](https://pkg.go.dev/github.com/ushineko/sanshoku/all) | Every driver and every support entry, for a program that wants all of them. |
| [`cmd/sanshoku-bench`](https://pkg.go.dev/github.com/ushineko/sanshoku/cmd/sanshoku-bench) | The hardware testbench. |
| [`cmd/sanshoku-apidoc`](https://pkg.go.dev/github.com/ushineko/sanshoku/cmd/sanshoku-apidoc) | Writes [docs/api.md](docs/api.md), the signature index of the public packages. |
| [`internal/apidoc`](https://pkg.go.dev/github.com/ushineko/sanshoku/internal/apidoc) | The generator behind `sanshoku-apidoc` and `api_test.go`; not importable outside the module. |

## What it does not do

Lighting effects, and lighting for most devices. Every lit device is
[OpenRGB](https://openrgb.org/)'s and stays in hotaru; that breadth is the
one thing not worth reimplementing. The one exception is the SteelSeries
Apex Pro TKL Wireless Gen 3, whose only lighting path is a stream of frames
its vendor's software renders on the host (spec 010): this module carries
the stream as `lighting.Canvas`, and the effects stay the consumer's. Everything
else follows the rule in [docs/design.md](docs/design.md): if direct access
can reasonably be done without an external tool, it is. The Arctis Nova Pro
Wireless is read directly (spec 006); other headsets are a candidate for a
later spec. NVIDIA temperature stays with `nvidia-smi`
because NVML is a vendor library, not a kernel node.

## Using it

```go
ctx := context.Background()
found, err := sanshoku.Scan(ctx, all.Drivers()...)
if err != nil {
	fmt.Println(err) // a driver failed; what the others found is still here
}
for _, c := range found {
	dev, err := c.Open(ctx)
	if err != nil {
		continue // sanshoku.IsPermission(err): the udev rule is missing
	}
	if src, ok := dev.(battery.Source); ok {
		batteries, _ := src.Batteries(ctx)
		for _, b := range batteries {
			if b.HasLevel {
				fmt.Printf("%s: %d%%\n", b.Name, b.Level)
			}
		}
	}
	_ = dev.Close()
}
```

A consumer names the drivers it wants, or takes `all.Drivers()`. A device
that has gone returns `sanshoku.ErrGone`; scan again. A permission error
means the udev rule in [docs/udev.md](docs/udev.md) is missing.

## Devices

[docs/devices.md](docs/devices.md) is generated from `all.Support()` and
has three tiers: **tested** on named hardware with the bench output in the
spec, **expected** to work because the protocol is generic and the code
path exists (run the bench on yours and report), and **listed** by ID but
not implemented. A program can ask `support` the same question at runtime;
the bench prints the tier beside every device it finds. The Apex Pro TKL
Wireless Gen 3 is the one device with a `lighting` capability.

## Testbench

`make bench` builds `sanshoku-bench` and runs `scan`, `read` and `verify`
against whatever is on the desk. All three are read-only. `verify` compares
each reading with the same device's through `liquidctl`, `solaar` and
`headsetcontrol`, whichever are on PATH.

`sanshoku-bench support` prints the support table, and `support --markdown`
prints [docs/devices.md](docs/devices.md), which is how `make generate`
writes it.

`sanshoku-bench screen --yes` is the one write: it pushes a test card to a
device's display, waits the panel's floor, and returns the display to its
readout. `screen --yes --hold D` keeps the card up for D instead (at least
the floor), so a person can look at it.

`sanshoku-bench light --yes` streams three patterns (steady, breathe, wave)
to every lighting canvas for `--hold` each, releases it, and prints frames
sent, acknowledged and timed out with the acknowledgement latency.
`--rate D` sets the frame interval, `--pattern`, `--color RRGGBB` and
`--only IDS` narrow the run, and `light --keys` lists the keys a frame may
address.

## Documentation

- [docs/design.md](docs/design.md): the rules the packages follow.
- [docs/devices.md](docs/devices.md): what is supported and how it was
  verified.
- [docs/udev.md](docs/udev.md): the rules a consumer ships.
- [docs/contention.md](docs/contention.md): what sharing the Kraken's and
  the Apex's nodes with OpenRGB and a second program measured.
- [docs/credits.md](docs/credits.md): the projects each protocol was learned
  from, and their licences.
- [docs/api.md](docs/api.md): every exported identifier with its signature
  and first sentence, generated from the doc comments.
- `specs/`: one spec per cycle of work.

## Development

```
make setup              # install the pinned linter
make test               # unit tests and the README and API canaries; opens no device
make coverage           # the tests, with a coverage report in the browser
make lint
make build              # the testbench, CGO_ENABLED=0
make bench              # the testbench against the hardware, read-only
make generate           # docs/devices.md and docs/api.md
make check-support      # fail if docs/devices.md is stale
make check-api          # fail if docs/api.md is stale
make check-no-binaries  # fail if a binary is committed
make vuln               # govulncheck
```

No cgo. Runtime dependencies are `golang.org/x/sys` and
`github.com/godbus/dbus/v5`. Linux is where every driver runs. On Windows the
HID drivers -- Logitech, Razer, SteelSeries and the Kraken's telemetry -- run
through the HID class driver, with no vendor software and no driver of their
own (spec 012); the Windows column of [docs/devices.md](docs/devices.md) says
where each device stands there. BlueZ, AirPods, hwmon and the Kraken's screen
need Linux interfaces: there they report `sanshoku.ErrUnavailable` or an error
wrapping `errors.ErrUnsupported`, and find nothing. The module builds and its
tests pass on Windows, Linux and macOS.

## Licence

MIT. See [LICENSE](LICENSE). The protocols were learned from other people's
open-source work first; [docs/credits.md](docs/credits.md) says whose.

## Changelog

### 0.1.9 (2026-10-07)

- **New API**: drivers describe themselves (spec 014, #41).
  `sanshoku.Description` (`Name`, `Finds`, `Capabilities`, `Platforms`,
  `Quiet`), `Describer`, `Describe(Driver)`, `Description.On` and `Offers`;
  every driver in the module implements it, and a test holds each description
  to its support entries. A consumer can list `all.Drivers()` and word what it
  finds and what it does not with no table of its own.

- **New API**: `sanshoku.Presence`, `Presence.Add` and `sanshoku.Presencer`, a
  receiver's nodes, quiet slots and unreadable devices, moved from the
  logitech driver; `logitech.Presence` and `logitech.Presencer` are aliases of
  them. **Change**: a Logitech child node now reports no quiet slots (the
  receiver's node counts them), and `OldProtocol` names what a `TooOld`
  device speaks ("HID++ 1.0").

- **Fix**: `IsPermission` recognises Windows' `ERROR_ACCESS_DENIED` (it matches
  `fs.ErrPermission`, not `EACCES` or `EPERM`), so a device another program
  holds unshared reads as not permitted instead of silent (#39).

### 0.1.8 (2026-10-07)

- `aula`: the AULA F75's battery through its 2.4 GHz receiver (spec 013):
  report 0x13, command 0x4A, the level and the power state in one reply. On
  its cable the receiver pins the level at 100, which is read as charging
  with no level. An unanswered question is asked once more, because a
  keyboard just switched to the receiver misses the first. Tested on Windows;
  Expected on Linux until a bench there reads it. The udev rules gain vendor
  3554.

- HID devices are read on Windows (spec 012). `hidraw` gains a Windows
  backend: one `Node` per USB interface, its top-level collections put back
  together; a write goes to the collection that declares its report ID, a read
  takes the first report from any of them, and feature reports go overlapped,
  with a deadline, to the collection that has them, even one opened with no
  access. `Node.Reports` lists what a node declares on both platforms, and
  `UsagePage` and `HasReportID` read it where there is no descriptor. Read on
  a desk with no vendor software: a Basilisk Ultimate through its dongle.

- Logitech: where the system gives a device paired to a receiver no node of
  its own (`hidraw.SplitsReceivers`, false off Linux), a HID++ 1.0 device is
  read on the receiver's node and named from its pairing register. Linux is
  unchanged.

- `bluez.Devices`, and with it the `bluez` and `apple` drivers, reports
  `ErrNoBlueZ` off Linux without dialling: on Windows godbus tried a TCP
  session bus on every scan.

- `support.Entry.Windows` says where an entry stands on Windows, and
  docs/devices.md has a Windows column. A Bluetooth audio transmitter that
  runs its own Bluetooth (UGREEN BT701) is recorded as out of scope, with what
  was measured.

### 0.1.7 (2026-10-01)

- The module builds off Linux (spec 011). `hidraw` feature reports, `l2cap`
  and `usbfs` have a stand-in for other platforms that returns an error
  wrapping `errors.ErrUnsupported`; `l2cap.ParseAddress` is portable. hayami
  could not be built for Windows at all before this, although every package it
  reads through sanshoku only needs to report "nothing found" there.

- hidraw nodes and usbfs paths are joined with `path`, not `path/filepath`: a
  device path is a Linux path whatever the host, and on Windows it read
  `\dev\hidraw12`.
### 0.1.6 (2026-10-01)

- **Fix**: the `steelseries` driver asks a keyboard or mouse the battery
  form it last answered first, up to three times, before trying the other.
  Through the Apex's receiver, with lighting frames streaming from another
  program, one wireless battery reply in five to one in two was lost, so a
  poll came back empty; twenty reads now give twenty readings. A read
  through the receiver also no longer spends 300 ms on the wired form it
  never answers (#31).

### 0.1.5 (2026-10-01)

- **New API**: package `lighting` with the `Canvas` capability (`Keys`,
  `Frame`, `Release`, `Floor`) and `ErrNoCanvas`; `Capabilities` reports
  `"lighting"`. The `steelseries` driver's Apex Pro TKL Wireless Gen 3
  (1038:1644 and 1038:1646) satisfies it by streaming the 0x61 direct frames
  SteelSeries GG was captured sending: 85 keys, a floor of 16 ms, each
  frame acknowledged. Its `Release` reboots the keyboard (0x41), the one
  way found to give the lighting back to the firmware, so the device is
  `ErrGone` afterwards. `sanshoku-bench light --yes` and `light --keys`.
  Spec 010.

- README: a "Why" with the footprint of the tools this module replaces,
  measured on the desk.

- `docs/credits.md`: the projects each protocol was learned from (liquidctl,
  HeadsetControl, Solaar, OpenRazer, rivalcfg, LibrePods, the kernel) with
  their licences, and what each taught this module.

### 0.1.4 (2026-09-30)

- **Fix**: the `steelseries` driver drains a held handle before every ask.
  hidraw copies every input report to every open handle, other programs'
  replies included, so a handle held across polls read the oldest queued
  report and froze at the state the headset had when the handle was opened
  (#24).

### 0.1.3 (2026-09-30)

- The Bluetooth live tests skip when BlueZ is unavailable, as they did when
  that was absence; CI on a runner without BlueZ failed instead.

- AirPods Pro over the accessory protocol are Tested: left and right read on
  the desk, which closes spec 004.

- **Visible to consumers**: battery names from the `logitech`, `razer` and
  `steelseries` drivers no longer start with the vendor.
  `battery.Product(vendor, name)` drops a leading vendor word (and a
  corporate suffix after it), so the Arctis reads "Arctis Nova Pro
  Wireless" and a K800 "K800"; a HID++ 2.0 name such as "G502 X PLUS" is
  unchanged. A consumer that keys anything by `Battery.Name` (hayami's
  last-reading memory) sees each such device under its new name once.
  `Identity.Name`, the support table and error messages are unchanged
  (spec 009, issue #22).

### 0.1.2 (2026-09-30)

- `hidraw` undoubles a vendor that carries a corporate suffix: "NZXT, Inc.
  NZXT Kraken Elite V2" reads as "NZXT Kraken Elite V2" (hotaru spec 059).

### 0.1.1 (2026-09-29)

- `sanshoku.ErrUnavailable`: a driver that could not look because its
  transport is missing. `bluez.ErrNoBlueZ` wraps it instead of `ErrAbsent`,
  so `Scan` reports it and hayami can say "no Bluetooth adapter" (hayami
  spec 020 found `Scan` was hiding it).

- Spec 008, a README and API reference that cannot drift (issue #16):
  `readme_test.go` fails when a package or command has no row in the
  "What is in it" table, a documented `make` target is missing from the
  Development block, a page in `docs/` is not linked, or the **Version**
  line is not the newest changelog heading. The "Using it" block is now
  the body of `Example` in `example_test.go`, so it compiles.
  `cmd/sanshoku-apidoc` writes [docs/api.md](docs/api.md), a signature
  index of the public packages from their doc comments; `make generate`
  writes it, `make check-api` and CI fail on a stale page, and `api_test.go`
  fails on the same diff and on any exported identifier without a doc
  comment. The Testbench section now covers `support --markdown` and
  `screen --yes --hold D`.

### 0.1.0 (2026-09-29)

- Spec 007, a paired child node knows its own index: new
  `hidraw.PairedIndex` returns the `:N` of a paired child's `HID_PHYS`, and
  `PairedChild` is its `ok` (it no longer accepts an index of 0 or above 6).
  The `logitech` driver probes a child node at its own index only, so a
  silent index behind a Unifying receiver is asked once rather than on all
  seven probes (issue #11).
- `sanshoku-bench read` prints "off" rather than a state beside a battery
  reading that has neither a level nor a band.

- Spec 006, Arctis Nova Pro Wireless battery, direct: the `steelseries`
  driver reads the headset through its base station (1038:12e5, and 12e0 by
  protocol) with HeadsetControl's `06 b0` report, matched on its echo, and
  sends it nothing else. New exported `steelseries.DecodeNovaPro`. A headset
  that is switched off is a reading with no level, not an error. The entry
  is Tested; `sanshoku-bench verify` compares the level and status with
  `headsetcontrol -o json`.
- Bench runs on two more machines promote the Razer Mouse Dock Pro, the
  Basilisk Ultimate dongle, the SteelSeries Apex Pro TKL Wireless Gen 3 and
  the Logitech K800 via Unifying to Tested; spec 003 is complete.
- `make lint` keeps its cache under the checkout, so git worktrees stop
  reporting findings against each other's deleted files.

- Spec 004, Bluetooth batteries: the `l2cap` transport, the `bluez`
  transport and driver, and the `apple` driver, ported from hayami's
  `internal/peripherals`. `l2cap.Dial` connects a sequenced-packet socket
  non-blocking with the address in written order (x/sys reverses it), awaits
  `EINPROGRESS` with `poll` and `SO_ERROR`, and retries `EINTR` with the
  deadline recomputed; `Conn` has `Send`, `Receive(ctx)` and `Close`, and
  `ParseAddress` reads a printed address. `bluez.Devices` lists connected
  devices from the system bus's ObjectManager, with the vendor and product
  from the Modalias, and reports an unreachable bus as `ErrNoBlueZ`, which
  wraps `ErrAbsent`. `bluez.Driver` reads `Battery1` for every connected
  device with a level except Apple audio; `apple.Driver` reads left, right
  and case over the Accessory Protocol on PSM 0x1001 and falls back to
  `Battery1`. Exports `bluez.Device`, `bluez.Devices`, `bluez.ErrNoBlueZ`,
  `apple.DecodeBattery`, `apple.ErrNoBatteryPacket`, both `Driver`s and
  `Support`s. A Bluetooth candidate's address is in `Identity.Phys` and in
  no error message; the bench masks it in the D-Bus object path it prints.
  `support.Lookup` gains a fourth rule: an entry with no vendor and no chips
  covers every candidate of its driver. `all.Drivers()` includes both; the
  `Battery1` path is Tested on a Sony WH-1000XM6 (hayami never verified it),
  and AirPods are Expected. Adds `github.com/godbus/dbus/v5` v5.2.2.
- `sanshoku-bench screen --hold D` keeps the test card on the panel for D (at
  least its floor), so a person can look at it.

- Spec 005, NZXT Kraken: the `nzxt` driver, ported from hotaru's
  `internal/cooler`. Coolant, pump and fan over hidraw (`74 01` → `75 01`,
  drained before every question, twelve reports per search, three attempts),
  with an allow-list of one product, the Kraken Elite (1e71:3012), and a
  status probe at `Open` that reports a node which does not answer as
  `ErrAbsent`. Reads coalesce within `Driver.Freshness` (250 ms); errors are
  never cached. The 640×640 LCD over the usbfs bulk endpoint, claimed on the
  first panel call: GIF only, fitted to the panel, placed with liquidctl's
  bucket algorithm, double-buffered, and returned to the firmware readout on
  `Close`. Exports `Driver`, `Support`, `Known`, `Model`, `Size` and
  `DecodeStatus`. `screen` gains `ErrNoPanel`. `sanshoku-bench screen --yes`
  pushes a test card, waits the panel's floor, and restores the readout.
  `all.Drivers()` includes it; the Kraken Elite is Tested.
  `docs/contention.md` records the three bench runs.
- `sanshoku-bench` treats a device a driver recognised and refused
  (`ErrUnsupported`) as a state, "unsupported", not a failure: `read` exits
  0 on it and `scan` shows it in the capabilities column. The Arctis Nova Pro
  Wireless has a Listed entry so the table names it.

- Spec 003, Razer and SteelSeries batteries: the `razer` and `steelseries`
  drivers, ported from hayami. `razer` reads a battery through feature
  reports on the node declaring usage page 0xFF00 or 0xFF01, including a
  mouse behind a Mouse Dock Pro through its RF relay: the checksum is
  verified, the transaction ID is searched in OpenRazer's order and
  remembered, and busy, timeout and not-supported are silence. `steelseries`
  reads the 0x92 command, then 0xD2, on usage page 0xFFC0, and only for
  products in its allow-list; any other product (the Arctis Nova Pro among
  them) and the listed legacy Rival family are candidates whose `Open`
  returns `ErrUnsupported` with the device's name, never written to. This
  replaces hayami's `Unsupported()` side channel. Exports `Driver`, `Support`
  and `razer.Decode`, `steelseries.DecodeModern`. `all.Drivers()` includes
  both; every entry is Expected or Listed until a bench run reads the Mouse
  Dock Pro and the Apex.
- Spec 002, Logitech HID++ batteries: the `logitech` driver, ported from
  hayami. One `Device` per HID++ hidraw node, held open; HID++ 2.0 features
  0x1004 and 0x1000 with the name and kind from 0x0005, HID++ 1.0 registers
  0x0D and 0x07 on a paired device's own node, both error forms, a software
  ID that is never solaar's, five attempts on silence. Exports `Driver`,
  `Support`, `DecodeUnifiedBattery`, `DecodeBatteryStatus`, `Presence` and
  `Presencer`. `all.Drivers()` includes it; the G502 X PLUS via Lightspeed is
  Tested, the K800 via Unifying and the two protocol families are Expected.
- `hidraw`, from the first driver to use it: **breaking**, `Exchange` takes a
  `ReportDevice` (`Write`, `Read` with a context), which `*Handle` satisfies,
  instead of `*Handle`. An exchange that runs out of time, on its own
  two-second bound or the caller's deadline, returns an error wrapping the
  new `ErrSilent` and `context.DeadlineExceeded` both; a cancelled context is
  the caller's error. `BusUSB` and `BusBluetooth` name `Node.Bus`'s values.
  `sanshoku-bench read` prints a HID++ node's Presence beside a withheld
  reading.
- Spec 001, the library foundation: the root vocabulary (`Identity`,
  `Candidate`, `Device`, `Driver`, `Scan`, `ErrAbsent`, `ErrGone`,
  `ErrUnsupported`, `IsPermission`, `Capabilities`); `hidraw` (sysfs
  enumeration by vendor and descriptor, the item walker, report exchange with
  deadlines, feature-report ioctls); `usbfs` (claim and bulk write);
  `hwmon` (sensors by chip and label, the CPU and GPU tables, a scan-only
  driver); the `battery`, `cooling` and `screen` capability types; the
  `support` table and `all`; `sanshoku-bench` with `scan`, `read`, `verify`,
  `support` and `screen`. `docs/devices.md` is now generated and CI checks it.
- Specs 001 to 005 and the module skeleton: vocabulary, transports, five
  drivers, the testbench. No implementation yet.
