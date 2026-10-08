# 014 — Drivers describe themselves

**Issue**: #41

## Status: COMPLETE

## Context

A consumer had to know things about each driver that only the driver knows.
hayami kept its own table of this module's drivers (`internal/panel/peripherals.go`,
`vendors()`): a display name per driver, the line for nothing found ("no
Logitech receiver", "no Razer device", "no SteelSeries device", "no AULA
receiver", "no Bluetooth device with a battery"), a `quiet` flag (a listed
device that reads nothing is reported: the Razer dock, SteelSeries base
stations, the AULA receiver), and which platforms each driver reads on (it kept
BlueZ and Apple off Windows in its own `core.Host`). For a Logitech receiver
it type-asserted `logitech.Presencer`, called `hidraw.PairedChild(c.Phys)` to
know whether a node's quiet slots were the receiver's or a child's, branched on
the vendor's name, and wrote "speaks HID++ 1.0" itself. A driver added here
meant an import, a table row and a check of those semantics there.

hayami's architecture review (its phase 3, and its `docs/architecture.md`
rule 1) asks that a device added here need no hayami change beyond `go get`.

## Requirements

- R1 `sanshoku.Description{Name, Finds, Capabilities, Platforms, Quiet}`,
  `Describer` with `Describe() Description`, and `sanshoku.Describe(Driver)`
  (false for a driver from outside the module). `Description.On(goos)` and
  `Offers(capability)` for filtering. Every driver in `all.Drivers()`
  describes itself, beside its support entries.
  - `Name` is the word a person knows the devices by; drivers reading the same
    set share it (`bluez` and `apple` are both "Bluetooth").
  - `Finds` is the noun after it: "no " + Name + " " + Finds is nothing found.
  - `Platforms` are runtime.GOOS spellings. Chosen over a `Supported() bool`
    so a consumer (or a doc generator) can ask about a system it is not
    running on, and so a test can hold it against the support table.
  - `Quiet` is a device fact: the transport lists itself whether or not the
    device behind it is awake.
- R2 `sanshoku.Presence{Nodes, Quiet, TooOld, OldProtocol}`, `Presence.Add`,
  and `sanshoku.Presencer`. `logitech.Presence` and `logitech.Presencer` become
  aliases of them, so code naming them still builds and a Logitech device
  satisfies the root interface with the method it had. The paired-child rule
  moves into the driver: a child node reports `Quiet` 0. `OldProtocol` is
  "HID++ 1.0" when `TooOld` is not empty.
- R3 A test holds each description to its driver's support entries: the same
  capabilities, Linux always, and Windows exactly when an entry stands there
  above Listed.
- R4 `docs/api.md` regenerated; `docs/devices.md` unchanged; README changelog.

## Acceptance Criteria

- [x] Every driver in `all.Drivers()` returns a Description (test).
- [x] Descriptions and support entries agree on capabilities and platforms
  (test); falsified by giving AULA Linux alone and NZXT's description cooling
  alone: both fail, and pass restored.
- [x] The battery drivers' "nothing found" lines are the five hayami used
  (test); falsified by naming Apple's driver "Apple".
- [x] A child node's Presence reports no quiet slots, and a TooOld device names
  its protocol (test); falsified by removing the rule.
- [x] `go test ./...` on Windows; `go vet` for linux, darwin and windows;
  golangci-lint v2.12.2, 0 issues on Windows and Linux.
- [x] `docs/api.md` current (`TestTheAPIReferenceIsCurrent`); `docs/devices.md`
  unchanged.

## The consumer, after this

What hayami's peripherals source becomes, with no driver package imported
beyond `all`:

```go
// vendors groups the battery drivers that read here by the name a person
// knows them by, in the module's order.
func vendors(goos string) []vendor {
	var out []vendor
	at := map[string]int{}
	for _, d := range all.Drivers() {
		desc, ok := sanshoku.Describe(d)
		if !ok || !desc.Offers("battery") || !desc.On(goos) {
			continue
		}
		if i, seen := at[desc.Name]; seen {
			out[i].drivers = append(out[i].drivers, d)
			continue
		}
		at[desc.Name] = len(out)
		out = append(out, vendor{
			name: desc.Name, absent: "no " + desc.Name + " " + desc.Finds,
			drivers: []sanshoku.Driver{d}, quiet: desc.Quiet,
		})
	}
	return out
}

// In pollVendor, after a read:
if pr, ok := dev.(sanshoku.Presencer); ok {
	out.presence = out.presence.Add(pr.Presence())
}

// And in Poll, in place of v.name == "Logitech": any vendor whose devices
// reported a receiver says so.
if !got.said && len(got.batteries) == 0 {
	if r := receiverReason(v.name, got.presence); r != nil { … }
}
for _, name := range got.presence.TooOld {
	// Text: "speaks " + got.presence.OldProtocol
}
```

`receiverReason` takes the vendor's name for "a Logitech receiver, with
nothing awake on it"; `addPresence` and the `hidraw` import go.

## Risks & Assumptions

- **Additive.** No exported identifier is removed. `logitech.Presence` is an
  alias now, not a distinct type: code that declared a method on it would
  not compile, and none in hayami or hotaru does.
- **Behaviour:** a Logitech child node's `Presence().Quiet` is 0 where it was
  the slots its discovery found silent. hayami discarded that count for child
  nodes already, so its sums do not change.
- **hwmon and NZXT** describe themselves too, though no battery consumer asks
  them: a consumer listing drivers for any capability gets every one.
- **Rollback**: revert; hayami keeps its own table until it adopts this.

## Gaps found

- `Finds` for hwmon ("sensor chip") and NZXT ("cooler") is used by no consumer
  yet; the words are chosen to read the same way as the battery drivers'.

## Verification

2026-10-08, Windows 11:

- `go test ./...`: all packages ok. `GOOS=linux`, `GOOS=darwin` and Windows
  `go vet ./...` clean. golangci-lint v2.12.2 (`GOTOOLCHAIN=go1.26.0`, the
  repo's config): 0 issues for Windows and for Linux.
- Falsified, each failing its test and passing restored: AULA Linux-only;
  Apple named "Apple"; NZXT's description cooling alone; the child-node rule
  removed. (An edit that changed NZXT's support entry and description
  together did not fail, as it should not.)
- Bench: `read --driver razer`: Basilisk Ultimate Dongle 32 %, discharging.
  The AULA F75 was asleep (no reading) and no Logitech receiver was plugged in,
  so neither was read live here.
