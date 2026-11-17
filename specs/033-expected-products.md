# 033 — Every product that shares a lighting driver's protocol

**Issue**: #90

## Status: COMPLETE

## Context

Each lighting driver opened one product: asus `0b05:18f3`, corsair the MM700,
keychron the K4 HE, msi one card, nzxt the Kraken Elite. The protocols are
wider, and the maintainer asked on 2026-11-17 that every product known to
share one be on its driver's list as expected, untested, with what is known of
its keys and zones, before hotaru's next release.

The source is the open record: OpenRGB's controller tables at commit
`5bcfa9ae` (protocol facts only, credited; no code), liquidctl for the Kraken
2024 Plus's panel, and NZXT's product pages for the RGB Core frames. A
research pass wrote each fact with its file and line; it is kept outside the
repository with the maintainer.

## Requirements

- R1 **asus** opens `1939`, `19af`, `1aa6`, `1bed` and `1b29` beside `18f3`.
  `1b29` is the ROG Strix LC III's pump, named for itself. On Windows, `19af`
  is read on its usage `A1` collection. Firmware whose version starts
  `AULA3-AR32` numbers effect channels headers first and the board after them.
  The Aura Addressable controllers stay refused.
- R2 **corsair** opens the MM700 3XL (`1bc9`), whose V2 entry is the MM700's.
- R3 **keychron** opens any keyboard on VIA's raw page whose features answer
  with the Keychron RGB bit; one off the tested list is named by its own USB
  name. The per-key effect number and the own effects are the K4 HE's.
- R4 **msi** opens the 47 cards OpenRGB pairs with this controller at `0x68`,
  on all four PCI IDs, each with its effect table: six without Stack and
  Rhythm, the RTX 50 cards with the coloured Rhythm as mode `0B`.
- R5 **nzxt** knows the Kraken 2024 Plus (`3014`): one lighting channel, its
  fans as accessory `2A` (three of eight) taken as attached where it lists
  none, a 240 by 240 panel.
- R6 Each is a support entry at **Expected**; the tested entries are
  unchanged. The Corsair V2 mice and keyboards (#92) and the Logitech `0x8081`
  keyboards (#93) are their own issues.
- R7 Docs: each driver's page, the credits, README.

## Acceptance Criteria

- [x] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries`, and `go vet` for windows, linux and darwin
      pass.
- [x] Tests: AR32 numbering, with and without the board's lights; the LC III's
      name; `19af`'s collection on Windows, and a product with one collection
      unfiltered; an Aura Addressable product refused; any Keychron board
      opened and named; an MSI card off the list refused, the families'
      effects, 47 cards; the Kraken 2024 Plus taking its fans as three of
      eight, and an Elite with nothing attached taking nothing.
- [x] The desk's tested devices are found, named and opened as before.
- [x] Docs per R7.

## Risks & Assumptions

- **Expected is not tested.** Every new entry rests on the open record; a bench
  run on one promotes it.
- **Keychron's per-key effect.** A firmware with another set of effects
  numbers its per-key effect differently, and claiming 23 there shows one of
  its own effects instead; nothing is harmed.
- **MSI writes over I2C.** Only cards OpenRGB itself pairs with this controller
  and address are listed, each on all four IDs.
- **Rollback:** revert.

## Verification

### Tests

`make test`, `make lint` (0 issues), `make check-api`, `make check-support`,
`make check-no-binaries`, and `go vet` for windows, linux and darwin pass.

- `asus`: channels from a table of one board light and three headers number
  `0 1 2 3`, and `3 0 1 2` on `AULA3-AR32-0207`; with no board light, `0 1 2`.
  `1b29` is "ASUS ROG Strix LC III" over the DMI name. `19af` without a
  descriptor is taken on usage `A1` and not `A0`. `1867` is refused unopened.
- `keychron`: `0e41`, "Keychron Keychron V1 Max", is "Keychron V1 Max" and
  opens.
- `msi`: subsystem `5199` is not found; `familyNoStack` lacks Stack and
  Rhythm; `familyRTX50`'s Rhythm is `0B` and the shared table's stays `0C`;
  `2b85:5300` is RTX 50; 47 cards.
- `nzxt`: a Plus with no accessories has one zone, Fan, 24 lights, parts
  `8 8 8`; an Elite with none has no zones.

### The desk (2026-11-17, read-only, hotaru running)

`sanshoku-bench scan` from this branch found the Logitech receiver, the Kraken
Elite, the MM700, the Z790's controller (`18f3`), the K4 HE and the RTX 4090,
each named and Tested as before. The MM700's second node (`hidraw9`) does not
answer, as it does not on `main`.
