# 001 — library foundation

**Issue**: #1

## Status: INCOMPLETE

## Context

hayami reads peripheral batteries over hidraw, L2CAP and BlueZ and CPU
temperature from hwmon (`internal/peripherals`, `internal/cooler`, at
54d357c). hotaru drives an NZXT Kraken's telemetry and LCD over hidraw and
usbfs and reads CPU and GPU temperature from hwmon (`internal/cooler`). The
two programs share an author and no code. Three near-identical hidraw sysfs
scanners exist across them (hayami `hidraw.go`, hotaru `discover.go`, hotaru
`systemd/handles_linux.go`); two hwmon readers; two `os.OpenFile` plus
`SetReadDeadline` plus prefix-match loops. hayami's descriptor walker handles
every item; hotaru's reads only the first. hayami's hwmon table covers AMD;
hotaru's is Intel only. Each fixed a bug the other still has.

This spec creates the module, the vocabulary every driver shares, the
transports the first drivers need (hidraw, usbfs, hwmon), and the testbench
that is the module's integration test. The drivers follow in specs 002 to
005, one per cycle, each porting the source of record and running the bench.

What stays out is listed in `docs/design.md`, "What belongs here": OpenRGB,
liquidctl, headsetcontrol, solaar, nvidia-smi, `/proc`.

## Requirements

### R1. Module and tooling

- R1.1 Module `github.com/ushineko/sanshoku`, `go 1.26.0`, no `toolchain`
  line. Direct dependencies of this spec: `golang.org/x/sys` and
  `github.com/stretchr/testify`. No cgo anywhere; `make build` sets
  `CGO_ENABLED=0`.
- R1.2 Makefile targets as committed in the skeleton: `setup`, `lint`,
  `test`, `coverage`, `vuln`, `build`, `bench`, `check-no-binaries`, `clean`.
  Linter pinned to golangci-lint v2.12.2 with config
  `config/.golangci-v2.12.2.yml`.
- R1.3 `make test` passes with no device present and with the race detector.
- R1.4 GitHub Actions `build.yml` as committed: test, lint, no-binaries and
  a cgo-free build on `ubuntu-latest`; `govulncheck` on latest stable Go.
  Linux only; the module is Linux only.
- R1.5 MIT licence, `Copyright (c) 2026 ushineko`.
- R1.6 Every package has a `doc.go` whose package comment says what the
  package is for and which kernel interface it touches.

### R2. Root package `sanshoku`

The vocabulary. Nothing here opens a device.

- R2.1 `Identity{Vendor, Product uint16; Bus Bus; Name, Phys, Path string}`
  with `String()` giving `Name (vvvv:pppp)`; `Bus` is `BusUSB`, `BusBluetooth`,
  `BusHwmon` with `String()`. `Name` is the kernel's `HID_NAME` with the
  vendor word undoubled (hayami `hidraw.go` `undouble`), or the BlueZ alias,
  or the hwmon chip name.
- R2.2 `Candidate{Identity; Driver string; Open func(context.Context)
  (Device, error)}`.
- R2.3 `Device` interface: `Identity() Identity`, `Close() error`.
  Capabilities are asserted from the packages in R5.
- R2.4 `Driver` interface: `Name() string`, `Find(context.Context)
  ([]Candidate, error)`.
- R2.5 `Scan(ctx, drivers ...Driver) ([]Candidate, error)`: runs each
  `Find` in order, appends candidates, joins errors with `errors.Join`, and
  returns both. An `ErrAbsent` from a driver is not joined; absence is not an
  error. Scan does not open anything.
- R2.6 Sentinels `ErrAbsent`, `ErrGone`, `ErrUnsupported`, with the meanings
  in `docs/design.md`. `IsPermission(err) bool` reports an `EACCES` or
  `EPERM` anywhere in the chain, so a consumer can say "install the udev
  rule" instead of "absent".
- R2.7 `Capabilities(Device) []string` returns the names of the capability
  interfaces the device satisfies (`"battery"`, `"cooling"`, `"screen"`),
  for the testbench and a consumer's doctor output. The root imports the
  capability packages; they do not import the root.

### R3. `hidraw` package

Ported from hayami `internal/peripherals/hidraw.go` (the walker) with
hotaru `internal/cooler/discover.go` (`usbNode`) and `hid.go` (drain).

- R3.1 `var SysRoot = "/sys/class/hidraw"`, `var DevRoot = "/dev"`, exported
  variables so tests swap in a `t.TempDir()` tree.
- R3.2 `Node{Path, Name, Phys string; Vendor, Product uint16; Bus uint16;
  Descriptor []byte; USBPath string}`. `USBPath` is the `/dev/bus/usb/BBB/DDD`
  node found by walking `device` parents for `busnum` and `devnum` (hotaru
  `usbNode`, up to 8 levels); empty when the node is not USB.
- R3.3 `Nodes(vendor uint16, want func(Node) bool) ([]Node, error)`: parses
  `device/uevent` `HID_ID`, `HID_NAME`, `HID_PHYS`; reads
  `device/report_descriptor`; applies the predicate. A missing `SysRoot`
  returns nil, nil. `vendor == 0` means any vendor.
- R3.4 Descriptor helpers: `Walk(desc, visit func(item Item) bool)` over
  short items (size code 3 is 4 bytes); `Item{Type, Tag byte; Value uint32}`;
  `UsagePage(want uint16) func(Node) bool`; `HasReportID(page uint16, id
  byte) func(Node) bool` (hayami `speaksHIDPP`: a vendor page at or above
  0xFF00 with report ID 0x10). `PairedChild(phys) bool` for the
  hid-logitech-dj `input2:N` suffix.
- R3.5 `Open(path string) (*Handle, error)` opens `O_RDWR`. `Handle` has
  `Write([]byte) error`, `Read(ctx, buf) (int, error)` honouring the context
  deadline through `SetReadDeadline`, `Drain(depth int)` discarding up to
  `depth` queued reports with a 2 ms deadline (hotaru `drain`, `queueDepth =
  64`), `Close`. A `Read` after the node is gone returns `sanshoku.ErrGone`
  (`ENODEV`, `EIO` with the sysfs entry missing).
- R3.6 `Exchange(ctx, h, req, matches func(reply []byte) bool, size int)
  ([]byte, error)`: write, then read until `matches` or the deadline,
  skipping unrelated reports. The three drivers that used this loop
  (HID++, SteelSeries, Kraken) call it; their specs name their `matches`.
- R3.7 Feature reports: `(*Handle).SetFeature([]byte) error` and
  `GetFeature([]byte) error` through `HIDIOCSFEATURE` and `HIDIOCGFEATURE`
  composed as in hayami `razer.go:339-361` (`_IOC(R|W, 'H', 0x06|0x07, n)`,
  size capped at 14 bits). Both take a context and fail with
  `context.DeadlineExceeded` when it expires, by running the ioctl in the
  calling goroutine with the context checked before and after; the ioctl
  itself cannot be interrupted and the comment says so. hayami declared
  `RazerTimeout` and never used it; here the deadline is real at the
  call boundary.

### R4. `usbfs` package

Ported from hotaru `internal/cooler/usbfs.go`.

- R4.1 `Open(path string, iface int) (*Interface, error)` claims the
  interface (`USBDEVFS_CLAIMINTERFACE` `0x8004550F`). No kernel driver is
  detached; the Kraken's LCD interface has none bound, and a device that
  needs a detach gets it in its own spec.
- R4.2 `(*Interface).Bulk(ctx, endpoint byte, data []byte) error` chunks at
  1 MiB (`USBDEVFS_BULK` `0xC0185502`, `struct usbdevfs_bulktransfer` with
  the 64-bit padding), checks for short writes, maps the context deadline to
  the ioctl timeout field.
- R4.3 `Close` releases (`USBDEVFS_RELEASEINTERFACE` `0x80045510`).
- R4.4 Constants from hotaru with their comments. `unix.Syscall` from `x/sys`,
  not `syscall`.

### R5. Capability packages

Result types and the interfaces a device asserts to. They import nothing
from this module.

- R5.1 `battery`: `Battery{Name string; Level int; HasLevel bool; State
  State; Band Band; HasBand bool; Kind Kind; Cells []Cell}`; `State`
  (Discharging, Charging, Full); `Band` (Critical, Low, Good, Full) with
  `Segments()` and `String()`; `Kind` (Other, Mouse, Keyboard, Headset, and
  the BlueZ icon kinds hayami maps in `bluez.go` `deviceKinds`); `Cell{Cell
  CellKind; Level int; Charging bool}` with kinds Headset, Left, Right,
  Case. `Source` interface: `Batteries(context.Context) ([]Battery, error)`.
  Ported from hayami `battery.go`, `kind.go`, `hidpp10.go` (`Band`),
  `aap.go` (`CellReading`).
- R5.2 `cooling`: `Status{Coolant float64; PumpRPM, PumpDuty, FanRPM,
  FanDuty int; HasPump, HasFan bool; Taken time.Time}`. `Source` interface:
  `Status(context.Context) (Status, error)`. Ported from hotaru `status.go`
  with hayami's `HasPump`/`HasFan`.
- R5.3 `screen`: `Panel` interface: `Size() (w, h int)`, `Image(ctx,
  *gif.GIF) error`, `Readout(ctx) error` (return to the firmware's own
  display), `Appearance(ctx, brightness, degrees int) error`, `Floor(bytes
  int) time.Duration` (the minimum interval between pushes for a frame of
  that size). Ported from hotaru `screen.go` and `dashboard/push.go` `Floor`;
  `Floor` moves into the driver because it is a property of the device.

### R6. `hwmon` package

Ported from hayami `internal/cooler/hwmon.go` (the superset) with hotaru's
GPU table.

- R6.1 `var Root = "/sys/class/hwmon"`.
- R6.2 `Sensor{Chip, Label string}`; `(Sensor) Read(root string) (float64,
  error)` matches `name` then `temp*_label`, reads `tempN_input`, divides by
  1000; an empty `Label` reads `temp1_input`. Never the hwmon index.
  `ErrNoSensor` wraps `sanshoku.ErrAbsent`.
- R6.3 `CPU` ordered list: `coretemp`/`Package id 0`, `k10temp`/`Tdie`,
  `k10temp`/`Tctl`, `zenpower`/`Tdie`, with hayami's comment on why Tctl
  ranks below Tdie. `GPU`: `amdgpu`/`edge`, `amdgpu`/``, `nouveau`/`` from
  hotaru `gpu.go`. `First(root, sensors) (Sensor, float64, error)` returns
  the first that reads. nvidia-smi stays in hotaru.
- R6.4 `Driver` implementing `sanshoku.Driver` so a scan lists the hwmon
  chips present, each a `Candidate` with `BusHwmon`; the opened device
  satisfies no capability yet. This exists so the bench's `scan` shows the
  whole machine. (A `sensor.Source` capability is a later spec if a consumer
  wants it; hayami and hotaru call `First` directly.)

### R7. Testbench `cmd/sanshoku-bench`

The integration test. Cobra is not used; `flag` with subcommands is enough
for four verbs and keeps the binary dependency-free.

- R7.1 `scan`: runs `sanshoku.Scan` over `all.Drivers()`, prints one line
  per candidate: driver, identity, path, and the capabilities of the opened
  device (open, assert, close). A candidate that fails to open prints the
  error; a permission error prints the udev rule to install, from
  `docs/udev.md`'s table (embedded as a map in the bench).
- R7.2 `read`: opens every candidate and calls every read-only capability
  (`Batteries`, `Status`), printing readings with timings. `--json` prints
  one object per device with identity, capabilities, readings and errors.
  `--driver name` restricts to one driver. `--repeat N --interval D` polls,
  for watching a device sleep and wake.
- R7.3 `verify`: for each reading, cross-checks against a tool if it is on
  PATH and prints agree/disagree: `liquidctl --json --match kraken status`
  for coolant, pump and fan (tolerances from hotaru `live_test.go`);
  `solaar show` for HID++ levels; `headsetcontrol -o json` is not compared
  (headsets are out of scope) but its presence is noted. A missing tool is
  "not checked", not a failure. Exit status 1 on any disagreement.
- R7.4 `screen --yes`: the only write. Pushes a generated test GIF, waits,
  returns the panel to its readout. Refuses without `--yes`. Lands in spec
  005; this spec ships the verb printing "no screen driver yet".
- R7.5 Output never prints a Bluetooth address or serial; identities print
  vendor:product and the kernel name.
- R7.6 `make bench` runs `scan`, `read`, `verify`. The spec that lands a
  driver pastes the bench output for that driver into its "Verification"
  section, with the machine's hardware named by product, not hostname.

### R8. `all` package

- R8.1 `all.Drivers() []sanshoku.Driver` returns every driver in the module
  in the order the specs land them. This spec: `hwmon.Driver{}` only.

### R9. Documentation

- R9.1 `README.md` "What is in it" table lists every package; the changelog
  under `### Unreleased` names this spec.
- R9.2 `docs/devices.md` is the supported-device table (IDs, transport,
  driver, the machine it was measured on, the spec). This spec adds the hwmon
  rows.
- R9.3 `packaging/60-sanshoku.rules`: the union of `docs/udev.md`.

## Acceptance Criteria

- [ ] `make test` passes on a machine with no supported device present.
- [ ] `make lint` passes.
- [ ] `CGO_ENABLED=0 make build` produces `sanshoku-bench`.
- [ ] `hidraw.Nodes` against a fake sysfs tree returns the nodes matching a
  vendor and usage page and ignores others; a missing root returns nil, nil.
- [ ] `hidraw.Walk` parses a captured G502 descriptor and a captured Kraken
  descriptor (bytes committed as fixtures; they carry no identifier) and
  reports the vendor page and report IDs hayami's tests expect.
- [ ] `hidraw.Exchange` returns `context.DeadlineExceeded` on a handle that
  never answers, within the deadline. (Tested with an `os.Pipe` pair, not a
  device.)
- [ ] `sanshoku.Scan` with one fake driver returning two candidates and one
  returning `ErrAbsent` yields two candidates and a nil error; with one
  returning another error yields the candidates and that error.
- [ ] `sanshoku.Capabilities` on a fake device implementing `battery.Source`
  returns `["battery"]`.
- [ ] `hwmon.First(CPU)` against a fake tree with a renumbered chip returns
  the labelled value; against an Intel tree returns `Package id 0`.
- [ ] `sanshoku-bench scan` on the development machine lists the hwmon chips
  and exits 0; its output is pasted below.
- [ ] `docs/devices.md`, `README.md` and `packaging/60-sanshoku.rules` exist
  with the content R9 names.

## Risks & Assumptions

- **Rollback**: the module has no consumers until phase 2; a revert is a
  revert.
- **Feature-report ioctl cannot be interrupted.** R3.7 bounds the call at its
  edges. A device that hangs an ioctl hangs the goroutine; hayami has run
  with this for months without a hang. Noted, not fixed.
- **usbfs bulk timeout** is the ioctl's own field, in milliseconds, capped at
  what the kernel accepts; a context with no deadline uses 5 s.
- **`hwmon.Driver` with no capability** is a scan convenience. If it reads as
  odd in review, drop R6.4 and let the bench call `hwmon.First` directly.
- **No udev monitor.** Scan is pull-only by design; both consumers poll.

## Alternatives Considered

- A registry with `init()` registration per driver: rejected; the Go policy
  discourages `init`, and a consumer that wants two drivers should name two.
- `gousb`/libusb for the LCD: rejected; hotaru's raw usbfs is 140 lines and
  cgo-free, and the module must build with `CGO_ENABLED=0`.
- A capability enum on `Device`: rejected in favour of type assertion, which
  is what Go offers and what `io` does.

## Verification

Bench output goes here when the spec is done.
