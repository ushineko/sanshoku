# sanshoku (三色)

[![Go Reference](https://pkg.go.dev/badge/github.com/ushineko/sanshoku.svg)](https://pkg.go.dev/github.com/ushineko/sanshoku)

**Version**: 0.0.0

Direct device access for Linux, in Go, without cgo: peripheral batteries over
HID++ and vendor report protocols, AirPods over Bluetooth, an NZXT Kraken's
telemetry and LCD, and hwmon temperatures. It is the maintained home of the
device code that [hayami](https://github.com/ushineko/hayami) and
[hotaru](https://github.com/ushineko/hotaru) each wrote for themselves.

Three colours, for the range of things under one roof.

> **Status**: specified, not yet built. Specs 001 to 005 in `specs/` are the
> plan; nothing below the vocabulary exists yet.

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
| [`all`](https://pkg.go.dev/github.com/ushineko/sanshoku/all) | Every driver, for a program that wants all of them. |
| [`cmd/sanshoku-bench`](https://pkg.go.dev/github.com/ushineko/sanshoku/cmd/sanshoku-bench) | The hardware testbench. |

## What it does not do

Lighting. Every lit device is [OpenRGB](https://openrgb.org/)'s and stays
in hotaru. Headsets are `headsetcontrol`'s. NVIDIA temperature is
`nvidia-smi`'s. The rule is in [docs/design.md](docs/design.md): a protocol
is written here only where no maintained tool serves.

## Using it

```go
found, err := sanshoku.Scan(ctx, all.Drivers()...)
for _, c := range found {
    dev, err := c.Open(ctx)
    if err != nil { continue }
    if src, ok := dev.(battery.Source); ok {
        batteries, _ := src.Batteries(ctx)
        // ...
    }
    dev.Close()
}
```

A consumer names the drivers it wants, or takes `all.Drivers()`. A device
that has gone returns `sanshoku.ErrGone`; scan again. A permission error
means the udev rule in [docs/udev.md](docs/udev.md) is missing.

## Devices

The table is [docs/devices.md](docs/devices.md), one row per device measured
or per protocol spoken.

## Testbench

`make bench` builds `sanshoku-bench` and runs `scan`, `read` and `verify`
against whatever is on the desk. All three are read-only. `verify`
cross-checks against `liquidctl` and `solaar` when they are on PATH.
`sanshoku-bench screen --yes` is the one write, and asks.

## Documentation

- [docs/design.md](docs/design.md): the rules the packages follow.
- [docs/devices.md](docs/devices.md): what is supported and how it was
  verified.
- [docs/udev.md](docs/udev.md): the rules a consumer ships.
- `specs/`: one spec per cycle of work.

## Development

```
make setup      # install the pinned linter
make test       # unit tests; opens no device
make lint
make build      # the testbench, CGO_ENABLED=0
make bench      # the testbench against the hardware, read-only
make vuln       # govulncheck
```

Linux only. No cgo. Runtime dependencies are `golang.org/x/sys` and
`github.com/godbus/dbus/v5`.

## Licence

MIT. See [LICENSE](LICENSE).

## Changelog

### Unreleased

- Specs 001 to 005 and the module skeleton: vocabulary, transports, five
  drivers, the testbench. No implementation yet.
