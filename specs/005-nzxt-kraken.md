# 005 — NZXT Kraken telemetry and LCD

**Issue**: #5

## Status: COMPLETE

## Context

hotaru drives an NZXT Kraken Elite (1e71:3012, firmware 1.2.0) without
liquidctl (`internal/cooler/*`, spec 012): coolant temperature, pump and fan
speed and duty over hidraw, and the 640×640 LCD over a usbfs bulk endpoint,
with the bucket-memory placement algorithm ported from liquidctl. It holds a
product allow-list because a Corsair PSU answered the status probe. It
drains queued broadcasts before every ask because the device sends a `75 02`
status about once a second unasked, and it retries an exchange three times
because OpenRGB holds the same node and takes replies. Its `Owner` coalesces
reads within 250 ms because three consumers polled in the same second, and
returns the panel to the firmware readout on close.

hayami reads the same cooler through a `liquidctl --json status` subprocess,
inherited from the Python monitor it replaced, and its spec 006 declined to
import hotaru's package because it "speaks HID to the device it also writes
to". The rule now (`docs/design.md`, "What belongs here") is direct access
wherever it is reasonable, so hayami adopts this driver in phase 2 and drops
liquidctl. That makes two processes on one node (hotaru's service and
hayami's panel) the normal case, and this spec measures it rather than
leaving it to the consumer.

This spec ports the driver as `nzxt`, on spec 001's `hidraw` and `usbfs`.
Behaviour is preserved. Three shape changes, each required by
`docs/design.md`: the owner's freshness becomes a driver option; `Floor`
moves from hotaru's dashboard package into the panel because it is a
property of the device; a node that vanishes returns `ErrGone` so the
consumer can rescan (hotaru had no reconnect once its owner ran).

## Requirements

### R1. Driver and discovery

- R1.1 `nzxt.Driver{Freshness time.Duration; ProbeTimeout time.Duration}`
  (defaults 250 ms and 500 ms). `Name()` is `"nzxt"`.
- R1.2 `Find`: `hidraw.Nodes(0x1E71, any)` filtered by the allow-list
  `Known = map[uint16]Model{0x3012: {Name: "Kraken Elite", Screen: Size{640,
  640}}}`; vendor-defined usage pages sorted first, as hotaru does. A
  product not in `Known` is listed and `Open` returns `ErrUnsupported`.
  `Model.Screen` is a size, not a string: `nzxt.Size{W, H int}`, exported
  with `Model`. "Sorted first" is by the descriptor's first usage-page item,
  as hotaru reads it.
- R1.3 `Open` probes with `Status` under `ProbeTimeout` and fails with
  `ErrAbsent` if the node does not answer `75 01` (the PSU case). Returns a
  `Device` satisfying `cooling.Source` and, if `Model.Screen` is set and
  `Node.USBPath` is known, `screen.Panel`. The usbfs interface is claimed
  lazily on the first panel call, and a failure to claim is
  `screen.ErrNoPanel` (wrapping `ErrAbsent`), so telemetry works without
  the udev rule for the USB node.

### R2. HID transport use

- R2.1 64-byte reports, no report ID prepended; a reply's prefix is
  `cmd[0]+1, cmd[1]`; `hidraw.Exchange` with that predicate; 12 report
  reads per attempt (liquidctl's number), 3 exchange attempts (stolen
  replies), `Drain(64)` before every ask, default wait 2 s.
- R2.2 Result code at byte 14: 0x01 ok; 0x04 refused; 0x09 slot would not
  clear.

### R3. Telemetry

- R3.1 `Status`: `74 01` → `75 01`; bytes 15/16 coolant whole and tenths,
  17–18 pump RPM LE, 19 pump duty, 23–24 fan RPM LE, 25 fan duty. `FF FF` at
  15–16 is a firmware fault (liquidctl #172), returned as an error, never
  cached. Exported `DecodeStatus(reply []byte) (cooling.Status, error)`.
- R3.2 Freshness: a `Status` call within `Freshness` of the last successful
  one returns that reading. Errors are never cached. The probe `Open` makes
  is a successful `Status`, so a read straight after `Open` is served from
  it. A node that returns `ErrGone` is not asked again within the three
  attempts.

### R4. Panel

Ported from `screen.go`, `fit.go`, `owner.go` with the protocol table from
hotaru spec 012.

- R4.1 `Size()` from the model. `Image(ctx, *gif.GIF)`: fit to the panel by
  nearest-neighbour in palette space (a wrong-size GIF displays blank),
  place in a vacant bucket (16 buckets, 1024-byte packets, 24 320 packets of
  memory, the placement algorithm from liquidctl), send over bulk endpoint
  0x02 on interface 0 with the 12-byte header `12 FA 01 E8 AB CD EF 98 76 54
  32 10` and 8 info bytes (`[0]=0x01` GIF, `[4:8]` LE32 length), show it,
  then delete the previous bucket. Never write the bucket on screen.
- R4.2 Command sequence, with replies: `30 04 slot` → `31 04` slot info;
  `32 02 slot` → `33 02` delete; `32 01 slot slot+1 addr(2) size(2) 01` →
  `33 01` reserve; `36 03` open exchange; `36 01 slot` begin; bulk; `36 02`
  end; `38 01 04 slot` → `39 01` show; `38 01 02` → `39 01` readout;
  `30 02 01 brightness 00 00 01 deg/90` with no reply for `Appearance`.
- R4.3 `Readout(ctx)` returns to the firmware's liquid display. `Close`,
  when the panel was claimed, calls it with a 2 s timeout, then releases the
  interface; then it closes the hidraw handle. A device nobody drew on is
  closed without a write (hotaru's `Owner.Close`). `Readout` and
  `Appearance`, like `Image`, claim the panel if it is not yet claimed.
- R4.4 `Floor(bytes)`: 1 s for frames ≤ 6 KB, 2 s for ≤ 24 KB, else 3 s,
  measured on this device; the comment says another cooler will have its
  own.
- R4.5 Static images are dropped by the firmware after 5–10 s; GIF only.
  The RGB565 framebuffer path stays out (it flickers; hotaru prototype).
- R4.6 Fan and pump duty writes stay out (the firmware discards them).

### R5. Tests

- R5.1 `DecodeStatus` table including the `FF FF` fault.
- R5.2 One fake transport, ported from hotaru `fake.go`: `Chatter`
  (unsolicited `75 02`), `Queued` backlog, `Silent`, `Faulty`, `LoseFirst`
  (a stolen reply), recorded `Told`. These are the measured behaviours and
  the reason the retry and drain exist; they stay.
- R5.3 Placement (`place`, `vacant`, `free`) table-tested against a fake
  slot table, from hotaru `screen_test.go`.
- R5.4 `live_test.go`: skips on `ErrAbsent`; agrees with `liquidctl --match
  kraken status` within hotaru's tolerances when liquidctl is on PATH; the
  6 s idle-queue test under `-short` skips; any screen write is opt-in via
  `SANSHOKU_LIVE_SCREEN=1`.
- R5.5 Bench `screen --yes` (spec 001 R7.4) lands here: push a generated
  GIF, wait `Floor`, return to readout.

### R6. Documentation

- R6.1 `docs/devices.md` is regenerated from the entry in R6.4. Naming:
  "Kraken Elite" in code and docs (hotaru used two names).
- R6.2 README, changelog, `all.Drivers()`.
- R6.4 `nzxt.Support()`: `Tested` for the Kraken Elite 1e71:3012 at the
  firmware the bench saw, capabilities cooling and screen. No `Expected`
  entries: hotaru measured one product and the allow-list is one product;
  other Kraken models get an entry when someone runs the bench on one.
- R6.3 `docs/contention.md`: what the three bench runs showed about
  sharing the node with OpenRGB and with a second consumer of this module,
  as the record hayami's phase 2 spec cites.

## Acceptance Criteria

- [x] `make test` and `make lint` pass.
- [x] The fake shows a reading survives one stolen reply and a queue of
  chatter, and a `FF FF` reply is an error that is not cached.
- [x] Placement tests pass against hotaru's cases.
- [x] `sanshoku-bench read --driver nzxt` reports coolant, pump and fan;
  `verify` agrees with liquidctl. Output pasted below for all three runs:
  alone, with OpenRGB up, with hotaru's service up.
- [x] `sanshoku-bench screen --yes` shows the test image and the panel
  returns to its readout afterwards, observed by eye. Observed 2026-09-29
  with `--hold 10s` (the flag added for this check) and hotaru stopped: the
  test card appeared on the pump head's LCD, held, and the readout returned.
  Every step reported success and liquidctl read the cooler normally
  afterwards (below).
- [x] `nzxt.Support()` exists and `make check-support` passes;
  `docs/contention.md`, README and `all` updated in the same commit.

## Risks & Assumptions

- **This driver writes to the device** (the LCD). It is the one place the
  module is not read-only; every write is behind an explicit call and the
  bench needs `--yes`.
- **OpenRGB and hotaru's service hold the node** in normal use. The bench
  is run three ways and the spec records each: alone, with OpenRGB up, and
  with hotaru's service up (which is what hayami's phase 2 looks like).
  hidraw gives every open descriptor its own copy of each input report, so
  the expected failure mode is interleaving and queue overflow, which the
  drain and the three-attempt retry exist for. If the bench shows
  something else, that is a finding for `docs/contention.md` and possibly
  a sixth spec, not a reason to keep liquidctl.
- **One cooler per machine** is assumed, as in hotaru.
- **Rollback**: revert; hotaru keeps its copy until phase 2.

## Verification

Kraken Elite, 1e71:3012, USB bcdDevice 01.02 (firmware 1.2.0), on hidraw6,
2026-09-29. liquidctl on PATH. The `read` timing prints 0.0 ms because the
reading is served from the `Open` probe taken a moment earlier (R3.2); the
exchange itself takes about 1.7 ms (docs/contention.md).

Checks:

```
$ make test          # every package ok; nzxt includes the live tests, 7.3 s
ok  	github.com/ushineko/sanshoku/nzxt	7.326s
$ make lint
0 issues.
$ CGO_ENABLED=0 make build
CGO_ENABLED=0 go build -trimpath -o sanshoku-bench ./cmd/sanshoku-bench
$ make check-support    # after make generate
(no diff)
$ ./sanshoku-bench screen
screen writes to a device's display; pass --yes to allow it     (exit 2)
```

### Run (a): hotaru's service and OpenRGB up

hidraw6 held by `openrgb` and `hotaru`; the USB node held by `hotaru`.

```
$ ./sanshoku-bench read --driver nzxt
nzxt  NZXT, Inc. NZXT Kraken Elite V2 (1e71:3012)  /dev/hidraw6  [tested]
  cooling: coolant 41.2 °C  pump 2746 rpm 92%  fan 1500 rpm 64% (0.0 ms)
exit 0
$ ./sanshoku-bench verify --driver nzxt
NZXT, Inc. NZXT Kraken Elite V2 coolant: agree (ours 41.2, liquidctl 41.2)
NZXT, Inc. NZXT Kraken Elite V2 pump: agree (ours 2746, liquidctl 2746)
NZXT, Inc. NZXT Kraken Elite V2 fan: agree (ours 1500, liquidctl 1587)
solaar: on PATH; no HID++ level to compare
headsetcontrol: on PATH; headsets are out of scope, not compared
exit 0
$ ./sanshoku-bench read --driver nzxt --json --repeat 50 --interval 200ms
50 reads, 50 without error
```

### Run (b): hotaru stopped, OpenRGB up

hidraw6 held by `openrgb`.

```
$ ./sanshoku-bench read --driver nzxt
nzxt  NZXT, Inc. NZXT Kraken Elite V2 (1e71:3012)  /dev/hidraw6  [tested]
  cooling: coolant 41.2 °C  pump 2749 rpm 92%  fan 1587 rpm 64% (0.0 ms)
exit 0
$ ./sanshoku-bench verify --driver nzxt
NZXT, Inc. NZXT Kraken Elite V2 coolant: agree (ours 41.2, liquidctl 41.2)
NZXT, Inc. NZXT Kraken Elite V2 pump: agree (ours 2749, liquidctl 2749)
NZXT, Inc. NZXT Kraken Elite V2 fan: agree (ours 1587, liquidctl 1587)
solaar: on PATH; no HID++ level to compare
headsetcontrol: on PATH; headsets are out of scope, not compared
exit 0
```

### Run (c): alone

hotaru stopped and `openrgb-server` stopped (a user unit; systemd ended it
with SIGKILL at its stop timeout). Nothing else held hidraw6.

```
$ ./sanshoku-bench read --driver nzxt
nzxt  NZXT, Inc. NZXT Kraken Elite V2 (1e71:3012)  /dev/hidraw6  [tested]
  cooling: coolant 41.2 °C  pump 2749 rpm 92%  fan 1500 rpm 64% (0.0 ms)
exit 0
$ ./sanshoku-bench verify --driver nzxt
NZXT, Inc. NZXT Kraken Elite V2 coolant: agree (ours 41.2, liquidctl 41.2)
NZXT, Inc. NZXT Kraken Elite V2 pump: agree (ours 2749, liquidctl 2733)
NZXT, Inc. NZXT Kraken Elite V2 fan: agree (ours 1500, liquidctl 1587)
solaar: on PATH; no HID++ level to compare
headsetcontrol: on PATH; headsets are out of scope, not compared
exit 0
```

Both services were started again afterwards and were active; hotaru
reattached to hidraw6 and reclaimed the USB node.

A 300-question measurement loop in each run (docs/contention.md) had no
question unanswered within twelve reports in any configuration.

### screen --yes: hotaru stopped, OpenRGB up

```
$ ./sanshoku-bench screen --yes
nzxt  NZXT, Inc. NZXT Kraken Elite V2 (1e71:3012)  panel 640x640, test card 6766 bytes, floor 2s
  image: ok (92.9 ms)
  waited 2s
  readout: ok (3.7 ms)
exit 0
$ liquidctl --match kraken status
NZXT Kraken 2024 Elite RGB
├── Liquid temperature    41.2  °C
├── Pump speed            2735  rpm
├── Pump duty               92  %
├── Fan speed             1507  rpm
└── Fan duty                64  %
exit 0
$ ./sanshoku-bench read --driver nzxt
nzxt  NZXT, Inc. NZXT Kraken Elite V2 (1e71:3012)  /dev/hidraw6  [tested]
  cooling: coolant 41.2 °C  pump 2735 rpm 92%  fan 1507 rpm 64% (0.0 ms)
exit 0
```

The by-eye check was done by the author at the desk with `screen --yes --hold 10s`
(see the acceptance criterion above).


### Where the port differs from hotaru

- Names: "Kraken Elite" for the model (hotaru: "NZXT Kraken Elite V2" in
  `Known`); the bench prints the kernel's name for the node.
- A node with no USB device above it is a telemetry-only candidate rather
  than skipped (R1.3).
- The status probe fills the freshness cache (R3.2); an `ErrGone` ends the
  three attempts early.
- `Image` takes a `*gif.GIF` (the `screen.Panel` signature) rather than
  bytes, so the fit encodes the GIF once; a GIF built in memory with no
  `Config` size is sized from its first frame.
