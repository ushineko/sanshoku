# 020 — a partial Logitech read is no reading

**Issue**: #54

## Status: COMPLETE

## Context

On a Lightspeed receiver (046d:c547) with one G502 X PLUS paired, one poll
returned a battery named `Logitech`, level 0, `KindOther`, beside readings of
the mouse as itself before and after. hayami keys its devices by name, so it
drew a second device, remembered it for a week and gave it a slot on its
Peripherals card.

The battery feature answered and feature 0x0005 did not answer whole: the
lookup, the name or the type was lost. `identify` then fell back to
`defaultName` and `KindOther`, and `read` returned the reading under that
identity. The fallback came from hayami #58, where a stray byte became a
device called "Q". It fixed the name. It did not fix the reading: a
placeholder name is a placeholder device to any consumer that tells devices
apart by name, and the level read in the same exchange is no more reliable
than the name beside it.

## Requirements

- R1 A battery whose device does not say which it is, because the 0x0005
  lookup, the name's length, a name chunk or the device type fails or comes
  back malformed, is withheld. It is `errUnidentified`, which `Batteries`
  treats as it treats `errSilent`: no reading, no error, and the index is
  rediscovered on the next poll.
- R2 A device that answers that it has no feature 0x0005 has answered. It is
  read as `defaultName`, `KindOther`, as before. So is a name that arrives
  whole and is only spaces.
- R3 A node that goes away during identification is still `sanshoku.ErrGone`
  to `Batteries`, not `errUnidentified`, so the poll stops asking it.
- R4 The HID++ 1.0 path is unchanged: it has no 0x0005, and its name is the
  kernel's.

## Acceptance Criteria

- [x] A battery with its name's length unanswered is no reading and no error;
  the next poll that hears everything reads "G502 X PLUS", mouse
  (`TestABatteryWithoutItsNameOrTypeIsNoReading`).
- [x] The same for a lost device type (same test).
- [x] A name shorter than its declared length withholds the reading rather
  than reporting `defaultName`
  (`TestANameShorterThanTheDeviceDeclaredWithholdsTheReading`, which replaces
  `TestANameShorterThanTheDeviceDeclaredIsRefused`).
- [x] A device without feature 0x0005 still reads as `Logitech`, kind other,
  at its level (`TestADeviceWithNoNameFeatureIsReadAsTheVendor`).
- [x] Falsified: with `read` falling back to `defaultName` on an
  identification error, the first two tests fail.
- [x] Bench, Linux, the receiver above: three reads, each
  `G502 X PLUS  83%  discharging  mouse`.
- [x] `go test ./...`, vet for linux, windows and darwin, and `make lint`
  are clean.

## Risks & Assumptions

- A Logitech device that never answers 0x0005 whole, rather than answering
  that it lacks it, now has no reading at all where it used to have one named
  `Logitech`. None is known. Every HID++ 2.0 device on the bench answers
  0x0005, and a device lacking it says so, which R2 keeps.
- Rollback: revert the commit. No API changes; `errUnidentified` is
  unexported.
