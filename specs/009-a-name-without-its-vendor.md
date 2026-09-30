# 009 — a name without its vendor

**Issue**: #22

## Status: COMPLETE

## Context

The hidraw drivers name a battery from the kernel's `HID_NAME`, which the
vendor writes as its own name and the product's: "SteelSeries Arctis Nova
Pro Wireless", "Logitech K800", "Razer Basilisk Ultimate Dongle". hayami
draws that name in a cell half a card wide and shows "SteelSeries Arctis…"
(hayami #83), the vendor kept and the product cut. The driver knows the
vendor; a person reading the name beside a battery level does not need it
twice.

`Identity.Name` is a different thing: it is what the kernel calls the node,
and the bench, a consumer's doctor and the support table key on it. It does
not change.

## Requirements

- R1 `battery.Product(vendor, name string) string` in the `battery` package
  (which imports nothing from the module): drops a leading word of `name`
  that equals `vendor` case-insensitively, and a following corporate suffix
  word as `hidraw`'s undouble does ("NZXT, Inc." is not a battery vendor
  today, but the rule is the same). "SteelSeries Arctis Nova Pro Wireless"
  → "Arctis Nova Pro Wireless"; "Logitech K800" → "K800"; "Razer Basilisk
  Ultimate Dongle" → "Basilisk Ultimate Dongle"; "G502 X PLUS" unchanged; a
  name that is only the vendor word is returned as is.
- R2 The `logitech`, `razer` and `steelseries` drivers pass every
  `battery.Battery.Name` they build from a kernel name through it, with
  their vendor word ("Logitech", "Razer", "SteelSeries"). A HID++ 2.0 name
  read from the device (feature 0x0005) is already the product's and goes
  through it too, which is a no-op. The `apple` and `bluez` drivers do
  not: an alias is the user's.
- R3 `Identity.Name`, the support entries and every error message are
  unchanged.
- R4 The bench's `read` output shows the battery name as the driver gives
  it (it already prints `j.Name`), so "Arctis Nova Pro Wireless" beside the
  identity "SteelSeries Arctis Nova Pro Wireless (1038:12e5)".
- R5 README changelog under `### Unreleased`, marked as a change consumers
  will see (hayami keys a memory of last readings by name).

## Acceptance Criteria

- [x] `make test`, `make lint`, `make check-api`, `make check-support` pass.
- [x] `battery.Product` table covers the five cases in R1 and a corporate
  suffix.
- [x] The existing driver fakes show a K800, a Razer dock and the Arctis
  named without their vendor, and the G502 unchanged.
- [x] `sanshoku-bench read` on the desk shows "Arctis Nova Pro Wireless"
  and "G502 X PLUS" as battery names with the identities intact. Output
  pasted below.

## Risks & Assumptions

- **hayami's `seen` map is keyed by name**, so the first poll after the
  bump forgets the Arctis once. Harmless; noted in hayami's changelog.
- **Rollback**: revert.

## Verification

2026-09-30, on the desk (Arctis Nova Pro Wireless base station and G502 X
PLUS on a Lightspeed receiver), from `CGO_ENABLED=0 make build`:

```
$ ./sanshoku-bench read --driver steelseries
steelseries  SteelSeries Arctis Nova Pro Wireless (1038:12e5)  /dev/hidraw14  [tested]
  battery: Arctis Nova Pro Wireless  no level  off  headset (9.3 ms)

$ ./sanshoku-bench read --driver logitech
logitech  Logitech USB Receiver (046d:c547)  /dev/hidraw12  [tested]
  battery: G502 X PLUS  86%  discharging  mouse (726.6 ms)
```

The identity lines keep the kernel names; the battery lines carry the
product. The headset was off, so the base station reported no level.

Checks, each exiting 0:

```
$ make test          # every package ok, battery included
$ make lint
0 issues.
$ make check-api     # after make generate added battery.Product to docs/api.md
$ make check-support # no diff
$ CGO_ENABLED=0 make build
CGO_ENABLED=0 go build -trimpath -o sanshoku-bench ./cmd/sanshoku-bench
```

`battery/product_test.go` covers the five R1 cases, a corporate suffix
("NZXT, Inc. Kraken Elite V2" → "Kraken Elite V2"), a vendor in another
case, and a name that is only vendor and suffix (returned as is). The
fakes: `logitech` K800 → "K800", G502 X PLUS unchanged; `razer` Mouse Dock
Pro → "Mouse Dock Pro"; `steelseries` Arctis → "Arctis Nova Pro Wireless"
and the Apex → "Apex Pro TKL Wireless Gen 3". The legacy-Rival refusal
still names the device by its full kernel name.
