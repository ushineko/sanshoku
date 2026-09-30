# sanshoku design

This page holds the rules that the packages in this module follow. They come
from the comments and specs in hayami's `internal/peripherals` and
`internal/cooler` and hotaru's `internal/cooler`, which carried these rules as
two copies that agreed by habit. Where the two programs disagreed, this page
records the choice and the reason.

The rules are mandatory for code in this module and recommended defaults for
a program that uses it. If a program does not follow a rule, it writes the
reason in a comment at that point.

## Contents

- [Vocabulary](#vocabulary)
- [What belongs here](#what-belongs-here)
- [The shape of a driver](#the-shape-of-a-driver)
- [Discovery](#discovery)
- [Transports](#transports)
- [Contention](#contention)
- [Errors](#errors)
- [Concurrency](#concurrency)
- [Freshness](#freshness)
- [Permissions](#permissions)
- [Testing](#testing)
- [Numbers](#numbers)
- [Public repository](#public-repository)

## Vocabulary

- **Transport**: a kernel interface the module speaks through. Four exist:
  hidraw (`/dev/hidrawN`), usbfs (`/dev/bus/usb/BBB/DDD`), L2CAP sockets and
  the BlueZ D-Bus API. sysfs hwmon is a fifth, read-only.
- **Driver**: a package that knows one vendor's protocol on one transport. It
  finds candidates and opens them. `logitech`, `razer`, `steelseries`, `apple`,
  `bluez` and `nzxt` are drivers.
- **Candidate**: a device a driver found but has not opened. It carries an
  `Identity` and knows how to open itself.
- **Device**: an open handle. It has an identity, a `Close`, and whatever
  capabilities it implements.
- **Capability**: a small interface a device may satisfy: `battery.Source`,
  `cooling.Source`, `screen.Panel`. A consumer asks for a capability with a
  type assertion. There is no capability enum.
- **Identity**: vendor, product, bus, kernel name, physical path and the
  node path. It is what a program shows the user and what the testbench keys
  its report on.
- **Reading**: a value with a time. `battery.Battery`, `cooling.Status` and
  `hwmon` temperatures are readings.

## What belongs here

This module holds the direct device access that hayami and hotaru wrote
themselves, and nothing else.

- **In**: a protocol the module speaks to a kernel device node or socket.
  Logitech HID++ over hidraw, Razer feature reports, SteelSeries rivalcfg
  reports, Apple AAP over L2CAP, BlueZ Battery1 over D-Bus, NZXT Kraken over
  hidraw and usbfs, hwmon by chip and label.
- **Out**: anything a maintained tool already covers. Lighting is OpenRGB's
  and stays in hotaru's `internal/openrgb`. liquidctl, headsetcontrol,
  solaar and nvidia-smi are subprocesses in the consumer, not transports
  here; the testbench may call them to cross-check a reading, which is the
  only place they appear. `/proc/stat`, `/proc/meminfo`, `/proc/net/dev` and
  DRM busy counters are not devices.
- **The rule** (hotaru spec 012, "where this reasoning stops"): write a
  protocol only where no integration point exists. A driver added here needs
  the sentence in its spec that says which tool was checked and why it did
  not serve.

## The shape of a driver

A driver package exports three things and hides the rest.

```go
type Driver struct{ ... }                       // options, all zero-usable
func (Driver) Name() string                      // "logitech"
func (Driver) Find(ctx) ([]sanshoku.Candidate, error)
```

A candidate's `Open` returns a `sanshoku.Device`. The device type is
unexported; the consumer sees `sanshoku.Device` plus the capability
interfaces it asserts. Nothing else is exported from a driver package except
the decoders the spec names as pure functions (a decoder takes bytes and
returns a reading, so it can be table-tested without a fake).

A driver never enumerates the whole machine itself. It asks the transport
package (`hidraw.Nodes`, `bluez.Devices`) with a vendor and a predicate and
gets back nodes. The transport packages own the sysfs and D-Bus knowledge;
the drivers own the protocols.

A driver matches a device by **vendor and report-descriptor usage page**, or
by an explicit product allow-list, and never by hidraw node number. The
SteelSeries Apex changes product ID between wired and wireless; the Kraken's
node number changes at every boot.

A driver that writes to a device it has not positively identified is a bug.
SteelSeries and NZXT hold product allow-lists for this reason: the Arctis
Nova Pro exposes the same usage page as the Apex, and a Corsair PSU answered
the Kraken's status probe. A product not in the table is reported as
unsupported by name, never written to.

## Discovery

`sanshoku.Scan(ctx, drivers...)` runs every driver's `Find` and returns the
candidates in driver order. It is a pull; there is no udev monitor and no
hotplug event. A consumer scans when it wants to know what is there: hayami
on every poll, hotaru at attach with a backoff. Driver errors do not stop the
scan; they are joined and returned beside the candidates that were found.

`sanshoku/all.Drivers()` is the list of every driver in the module, for a
program that wants all of them. There is no registry and no `init`. A
program that wants two drivers names two.

A device that has gone away returns `sanshoku.ErrGone` from its methods, and
the consumer rescans. hotaru had no reconnect after the cooler was unplugged
once its owner was running; this is the fix.

## Transports

- **hidraw**: open `O_RDWR`, deadlines through `SetReadDeadline`, reports
  matched by prefix, unrelated traffic skipped. Feature reports go through
  `HIDIOCSFEATURE` and `HIDIOCGFEATURE` for devices that declare no output
  report (Razer). Every read has a deadline; a transport method with no
  timeout is a bug (hayami's Razer path had none).
- **usbfs**: claim, bulk write, release, through raw ioctls. No libusb, no
  cgo. Only the Kraken's LCD uses it.
- **L2CAP**: `SOCK_SEQPACKET`, connect with poll and `SO_ERROR`, `EINTR`
  retried with the deadline recomputed. The Bluetooth address is passed in
  written order; `x/sys` reverses it.
- **BlueZ**: the system bus `ObjectManager`. No `upower` and no BlueZ
  experimental flag.
- **hwmon**: chip name and label, never the hwmon index, which the kernel
  renumbers.

All five are Linux. The module is Linux-only and says so; there are no stub
files for other platforms.

## Contention

Other programs hold the same nodes. OpenRGB holds the Kraken's hidraw and
takes replies meant for hotaru; liquidctl and hayami's HID++ share receivers
with solaar and with the desktop's own battery applet. Each driver records
its measured defence:

- HID++ uses a software ID that is not solaar's, so replies to solaar are
  recognisable as not ours.
- The Kraken drains queued broadcasts before every ask and retries an
  exchange three times because a reply can be stolen.
- Razer treats "busy" as silence, not failure.

A driver does not lock a device against other processes. Cross-process
locking is the consumer's decision and a later spec's problem.

## Errors

Three sentinels at the root, wrapped with `%w` everywhere:

- `ErrAbsent`: nothing to find. Not an error a program shows in red.
- `ErrGone`: the device was there and is not now. Rescan.
- `ErrUnsupported`: the device is known and this driver will not speak to it.

A driver returns partial results beside its error, joined with
`errors.Join`. Silence (a device asleep) is not an error; it is a reading
withheld, and the driver's spec says how many attempts it makes and why.

An error message never contains a Bluetooth address, a serial number or a
name the user gave a device. The repository and the bug tracker are public.

## Concurrency

A `Device` is safe for concurrent use from one process: each driver holds a
mutex around its transport. A `Device` is one handle; two `Open`s of the same
candidate are two handles and the caller keeps them apart.

No goroutines in driver packages. Reads are synchronous and the caller owns
the ticker. `context.Context` bounds every operation.

## Freshness

A driver may coalesce reads: the Kraken answers a `Status` call from a reading
less than 250 ms old rather than asking the device again, because three
consumers in hotaru (the API, the dashboard, the tray) polled it in the same
second. Errors are never cached. The freshness window is a driver option
with a documented default, not a package constant.

## Permissions

The module runs as the logged-in user. It needs the seat ACL that
`systemd-logind` applies to nodes tagged `uaccess`. `docs/udev.md` carries
the rules per vendor; a consumer ships them in its package. A missing rule
looks like `ErrAbsent` with an "opening /dev/hidrawN: permission denied"
underneath, and the testbench says so in words.

## Testing

Three layers, in the order they are trusted:

1. **The testbench** (`cmd/sanshoku-bench`) against real hardware. This is
   the integration test and the correctness oracle. `scan`, `read` and
   `verify` are read-only and run under `make bench`; writes are behind
   `screen --yes`. `verify` compares against liquidctl, solaar and
   headsetcontrol when they are on PATH, with the tolerances the specs name.
2. **Live `_test.go` files**, one per driver, that skip on `ErrAbsent`. They
   call the same code the bench does and exist so `go test` on the desk
   catches a regression without running the bench by hand.
3. **Unit tests** for the API and the decoders. The root API has one fake
   driver. Decoders are table-tested from captured bytes. A driver has one
   fake transport, carrying only the measured behaviours its spec lists
   (queued broadcasts, a stolen reply, silence, a fault code). No fake models
   a device it was not measured against.

Unit tests never open a device. `make test` passes on a machine with nothing
plugged in.

## Numbers

Every constant in a driver is a measurement from one machine, and the comment
says which behaviour set it: the Razer settle time (below 50 ms the device
returns the previous answer), the HID++ retry count (a mouse idle for 6 s
needed four attempts), the Kraken's push floor per frame size. A constant
without its measurement is a guess and is labelled one.

## Public repository

No captured traffic that carries a serial number, a Bluetooth address or a
device name the user set. Test fixtures use `AA:BB:CC:DD:EE:FF` and vendor
names. No personal paths or hostnames. Vendor and product IDs are public
facts and are fine.
