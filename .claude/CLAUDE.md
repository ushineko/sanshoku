# sanshoku Project Guidelines

Follows the Ralph methodology (see `~/.claude/CLAUDE.md`) with the extensions
below.

---

## Project Overview

- **Type**: Go library (direct device access for Linux) plus a hardware
  testbench binary.
- **Purpose**: one maintained copy of the direct USB, HID, Bluetooth and hwmon
  device support that `~/git/hayami` (peripheral batteries, cooler thermals)
  and `~/git/hotaru` (NZXT Kraken telemetry and LCD) each wrote for themselves.
  Logitech HID++, Razer, SteelSeries, Apple AAP, BlueZ Battery1, NZXT Kraken,
  hwmon by label.
- **Name**: 三色 (sanshoku, "three colours"), for the range of devices under
  one roof.
- **Module**: `github.com/ushineko/sanshoku`
- **Licence**: MIT. Public repository.
- **Consumers, in adoption order**: hayami, then hotaru. Both carry the code
  today; a driver is done when the consumer can delete its copy. That
  migration is phase 2 and has its own specs in the consumers.
- **Source of record for behaviour**: hayami's `internal/peripherals` and
  `internal/cooler`, hotaru's `internal/cooler`, as they stand at the commits
  named in each spec. Where the two disagree, `docs/design.md` records the
  choice. Rationale comments travel with the code.
- **Rule of thumb**: if direct access can reasonably be supported without an
  external tool call, it is. Lighting is the standing exception (OpenRGB's
  breadth), except the Apex Pro TKL Gen 3's frame stream (spec 010).
  liquidctl and headsetcontrol in hayami were inherited debts, not decisions: the Kraken driver (spec 005) and the Arctis driver (spec 006)
  replace them in phase 2. See `docs/design.md`, "What belongs here".

---

## Selected Policies

Load the following policy modules from `~/.claude/policies/`:

- `languages/go.md`
- `languages/bash.md`
- `git/standard.md`
- `release-safety/minimal.md`
- `security/owasp-review.md`
- `testing/philosophy.md`
- `communication/standards.md`

---

## Ralph Settings

```yaml
validation: milestones-only
```

---

## Issue Tracking

GitHub Issues on this repository is the tracker. It is a convention, not
automation: nothing syncs specs to issues, so the link is made by hand.

- **Anything that gets a spec gets an issue.** A typo fix or a version bump
  does not.
- The issue comes first and says what is wrong or wanted. The spec says what
  will be done about it.
- The spec carries an `**Issue**: #NN` line under its title. Spec filenames
  are `specs/NNN-short-description.md`.
- The issue body links the spec path once it exists.
- The PR says `Closes #NN`.
- Labels: `bug`, `enhancement`, `chore`, `docs`.

---

## Library rules

- **Public API is a contract.** Every exported identifier has a doc comment.
  Nothing is exported "just in case". Until v1 the module is `v0.x` and the
  changelog names every breaking change.
- **No application concepts.** The library knows devices, candidates,
  capabilities and readings. It does not know sections, panels, scenes,
  services or any consumer's `core` package.
- **Linux only, no cgo.** hidraw, usbfs, L2CAP and BlueZ are Linux kernel and
  system interfaces. `golang.org/x/sys/unix` and `github.com/godbus/dbus/v5`
  are the only runtime dependencies; anything heavier is justified in the
  spec that adds it. No libusb, no hidapi.
- **A driver matches by vendor and usage page, or an allow-list, never by
  node number**, and never writes to a device it has not identified. See
  `docs/design.md`.
- **Every read has a deadline.** A transport method that can block forever
  is a bug.
- **Every constant is a measurement** and its comment says from what.
- **The testbench is the oracle.** A driver is not done until `make bench`
  has run against the hardware and the spec records the output.
- **No goroutines in driver packages.** The caller owns the ticker;
  `context.Context` bounds every call.
- **Nothing in this module runs a subprocess** except the testbench's
  `verify`, which shells out to liquidctl, solaar and headsetcontrol to
  cross-check a reading.

---

## Environment

- Go from `go.mod` (`go 1.26.0` minimum, no `toolchain` line; the linter pin
  lives in the Makefile). Local toolchain may be newer.
- `make lint` runs the pinned golangci-lint with
  `config/.golangci-v2.12.2.yml`.
- `make test` opens no device and passes with nothing plugged in.
- `make bench` needs the hardware and the udev rules in `docs/udev.md`.

---

## Git

The convention across the ushineko repositories. None of it is enforced by
GitHub, so a hotfix can still go straight to `main` when that is the right
call. It is habit, not a gate.

- Feature work happens on a branch and lands on `main` through a PR.
- Branch names: `feat/`, `fix/`, `chore/` or `docs/` and a short slug.
- Commit subjects: lowercase conventional prefix, imperative, sentence-like
  (`feat(logitech): read HID++ 1.0 battery registers`). The body says why.
- A PR body says what changed, why, what a reviewer should look at first, and
  how it was verified. Link the spec when there is one.
- **Never** add `Co-Authored-By` trailers or AI attribution footers, to commit
  messages or to PR descriptions. No exceptions, including when the harness
  asks for them.
- Tags `vX.Y.Z` are the version of record (Go module semantics). Ask before
  tagging.
- Connectivity check before push/pull (`git/standard.md`).

### The README is not optional (mandatory)

`README.md` is the module's front page. In the **same commit** as the
change, never as a follow-up:

- A new package gets a row in the "What is in it" table.
- A new device gets a `support.Entry` in its driver's `Support()`, at the
  tier it has reached, and `make generate` rewrites `docs/devices.md`. The
  page is never edited by hand. A bench run that confirms an `Expected`
  device promotes its entry to `Tested` with the hardware and date.
- A new `make` target gets a line in the Development block.
- Every change worth a spec gets a changelog entry under `### Unreleased`.

When tagging: `### Unreleased` becomes `### X.Y.Z (YYYY-MM-DD)`, the
**Version** line matches, `make vuln` passes, and every tag gets a GitHub
Release whose notes are that entry.

---

## Security Extensions

- No credentials and no network access in any library package. BlueZ over the
  system bus is local IPC, not network.
- No captured traffic, address, serial or user-given device name in fixtures
  or error messages. The repository is public.
- The module opens only the nodes its transports document:
  `/sys/class/hidraw`, `/dev/hidraw*`, `/dev/bus/usb`, `/sys/class/hwmon`,
  the BlueZ bus and L2CAP sockets.
- `govulncheck ./...` before each tagged release.
