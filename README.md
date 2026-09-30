# sanshoku (三色)

[![Go Reference](https://pkg.go.dev/badge/github.com/ushineko/sanshoku.svg)](https://pkg.go.dev/github.com/ushineko/sanshoku)

**Version**: 0.1.0

Direct device access for Linux, in Go, without cgo: peripheral batteries over
HID++ and vendor report protocols, AirPods over Bluetooth, an NZXT Kraken's
telemetry and LCD, and hwmon temperatures. It is the maintained home of the
device code that [hayami](https://github.com/ushineko/hayami) and
[hotaru](https://github.com/ushineko/hotaru) each wrote for themselves.

Three colours, for the range of things under one roof.

> **Status**: v0.1.0. Every driver is built and bench-tested on real
> hardware: Logitech HID++ 1.0 and 2.0, Razer, SteelSeries keyboards and the
> Arctis Nova Pro Wireless, NZXT Kraken Elite telemetry and LCD, BlueZ
> `Battery1`, and hwmon. AirPods over the Accessory Protocol are written and
> not yet benched. No program imports the module yet; hayami is first.

## Contents

- [What is in it](#what-is-in-it)
- [What it does not do](#what-it-does-not-do)
- [Using it](#using-it)
- [Devices](#devices)
- [Testbench](#testbench)
- [Documentation](#documentation)
- [Development](#development)
- [Licence](#licence)
- [Changelog](#changelog)

## What is in it

| Package | Purpose |
|---|---|
| [`sanshoku`](https://pkg.go.dev/github.com/ushineko/sanshoku) | The vocabulary: `Identity`, `Candidate`, `Device`, `Driver`, `Scan`, the sentinel errors, `Capabilities`. |
| [`hidraw`](https://pkg.go.dev/github.com/ushineko/sanshoku/hidraw) | sysfs enumeration, report-descriptor walking, report exchange with deadlines, feature-report ioctls. |
| [`usbfs`](https://pkg.go.dev/github.com/ushineko/sanshoku/usbfs) | Claim an interface and write a bulk endpoint through raw usbdevfs ioctls. |
| [`hwmon`](https://pkg.go.dev/github.com/ushineko/sanshoku/hwmon) | Temperatures by chip and label; the CPU and GPU sensor tables. |
| [`l2cap`](https://pkg.go.dev/github.com/ushineko/sanshoku/l2cap) | Bluetooth L2CAP sequenced-packet sockets with deadlines. |
| [`bluez`](https://pkg.go.dev/github.com/ushineko/sanshoku/bluez) | Connected devices from the BlueZ system bus; the generic `Battery1` driver. |
| [`battery`](https://pkg.go.dev/github.com/ushineko/sanshoku/battery) | The battery reading and the `Source` capability. |
| [`cooling`](https://pkg.go.dev/github.com/ushineko/sanshoku/cooling) | The cooler reading and the `Source` capability. |
| [`screen`](https://pkg.go.dev/github.com/ushineko/sanshoku/screen) | The `Panel` capability for a device with a display. |
| [`logitech`](https://pkg.go.dev/github.com/ushineko/sanshoku/logitech) | HID++ 1.0 and 2.0 batteries over hidraw. |
| [`razer`](https://pkg.go.dev/github.com/ushineko/sanshoku/razer) | Battery through feature reports, including a mouse behind its dock. |
| [`steelseries`](https://pkg.go.dev/github.com/ushineko/sanshoku/steelseries) | Battery over hidraw, with a product allow-list. |
| [`apple`](https://pkg.go.dev/github.com/ushineko/sanshoku/apple) | AirPods over the Accessory Protocol: left, right and case. |
| [`nzxt`](https://pkg.go.dev/github.com/ushineko/sanshoku/nzxt) | Kraken Elite telemetry and LCD. |
| [`support`](https://pkg.go.dev/github.com/ushineko/sanshoku/support) | The hardware support table: tested, expected, listed. |
| [`all`](https://pkg.go.dev/github.com/ushineko/sanshoku/all) | Every driver and every support entry, for a program that wants all of them. |
| [`cmd/sanshoku-bench`](https://pkg.go.dev/github.com/ushineko/sanshoku/cmd/sanshoku-bench) | The hardware testbench. |
| [`cmd/sanshoku-apidoc`](https://pkg.go.dev/github.com/ushineko/sanshoku/cmd/sanshoku-apidoc) | Writes [docs/api.md](docs/api.md), the signature index of the public packages. |
| [`internal/apidoc`](https://pkg.go.dev/github.com/ushineko/sanshoku/internal/apidoc) | The generator behind `sanshoku-apidoc` and `api_test.go`; not importable outside the module. |

## What it does not do

Lighting. Every lit device is [OpenRGB](https://openrgb.org/)'s and stays
in hotaru; that breadth is the one thing not worth reimplementing. Everything
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
the bench prints the tier beside every device it finds.

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

## Documentation

- [docs/design.md](docs/design.md): the rules the packages follow.
- [docs/devices.md](docs/devices.md): what is supported and how it was
  verified.
- [docs/udev.md](docs/udev.md): the rules a consumer ships.
- [docs/contention.md](docs/contention.md): what sharing the Kraken's nodes
  with OpenRGB and a second program measured.
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

Linux only. No cgo. Runtime dependencies are `golang.org/x/sys` and
`github.com/godbus/dbus/v5`.

## Licence

MIT. See [LICENSE](LICENSE).

## Changelog

### Unreleased

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
