# 005 — NZXT Kraken telemetry and LCD

**Issue**: #5

## Status: INCOMPLETE

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
  `Model.Screen` is a size, not a string.
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
  one returns that reading. Errors are never cached.

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
- R4.3 `Readout(ctx)` returns to the firmware's liquid display. `Close`
  calls it with a 2 s timeout, then releases the interface, then closes the
  hidraw handle.
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

- R6.1 `docs/devices.md`: Kraken Elite 1e71:3012, firmware 1.2.0, measured;
  the two udev rules. Naming: "Kraken Elite" in code and docs (hotaru used
  two names).
- R6.2 README, changelog, `all.Drivers()`.
- R6.3 `docs/contention.md`: what the three bench runs showed about
  sharing the node with OpenRGB and with a second consumer of this module,
  as the record hayami's phase 2 spec cites.

## Acceptance Criteria

- [ ] `make test` and `make lint` pass.
- [ ] The fake shows a reading survives one stolen reply and a queue of
  chatter, and a `FF FF` reply is an error that is not cached.
- [ ] Placement tests pass against hotaru's cases.
- [ ] `sanshoku-bench read --driver nzxt` reports coolant, pump and fan;
  `verify` agrees with liquidctl. Output pasted below for all three runs:
  alone, with OpenRGB up, with hotaru's service up.
- [ ] `sanshoku-bench screen --yes` shows the test image and the panel
  returns to its readout afterwards, observed by eye.
- [ ] `docs/devices.md`, `docs/contention.md`, README and `all` updated in
  the same commit.

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

Bench output goes here when the spec is done.
