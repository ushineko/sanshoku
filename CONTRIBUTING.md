<!-- Generated from the shared contributing policy. Edit the template and the project fragments, then re-render; do not hand-edit this file. -->

# Contributing to sanshoku

Thanks for your interest. This is a personal project, and external pull requests
are welcome. This guide states the policy a PR is held to, and what the
maintainers will do with it.

## TL;DR

- Only elected maintainers merge. See [MAINTAINERS.md](MAINTAINERS.md).
- AI-written and hand-written PRs are both accepted, and both get the same
  review. Either may be rejected if it does not meet the standards below.
- Every PR needs tests **and**, for anything touching hardware or device
  support, an end-to-end reading from a real system — pasted into the PR.
  A PR without that collateral may be rejected.
- Keep the diff scoped: one logical change, no drive-by reformatting.
- No `Co-Authored-By` trailers and no AI-attribution footers in commits or the
  PR description.

## Who merges

Merge rights belong to the maintainers listed in
[MAINTAINERS.md](MAINTAINERS.md), and to nobody else. No contributor — human or
agent — merges their own PR, and no PR lands without a maintainer's approving
review.

Maintainers are elected, not self-appointed. The process, the current roster,
and how to be considered are all in [MAINTAINERS.md](MAINTAINERS.md).

A maintainer may merge a PR as-is, modify it before merging, hold it pending
changes, or close it. Closing is not a judgement of the contributor; it most
often means the change does not fit the project's direction, and that is the
maintainer's call to make.

## AI-written contributions

Code written with an AI assistant is fine. It is reviewed exactly like
hand-written code, held to the same standards, and modified by maintainers where
needed.

One rule makes that workable: **you must understand what you submitted.** If you
cannot explain what the change does, how it behaves at the edges, and why it
fits the existing design, the PR will be closed. Review questions go to the
person who opened the PR, not back to a model.

Specifically, a PR is likely to be rejected if it:

- adds a plausible-looking abstraction the project did not ask for,
- restates existing behaviour in new words without changing it,
- carries generated commentary, diagnosis dumps, or implementation-plan prose
  in the diff or the PR body,
- reformats or "improves" code outside the change,
- or claims a test or an e2e reading that was not actually run.

Do not add `Co-Authored-By` trailers or "Generated with …" footers to commits or
to the PR description. A PR containing them will be asked to amend.

## Tests and e2e collateral

Two things are required, and they are separate requirements.

**1. Tests.** Any behaviour change adds or updates tests. Tests encode
behavioural contracts — what the code does — not internals. A test that breaks
on a pure refactor with no behaviour change is testing the wrong thing. Paste
the test run summary into the PR.

**2. An e2e reading from a real system.**

sanshoku is a device-access library. Its entire value is that the bytes it
sends and parses match what real hardware does, and a mocked HID transport
will happily agree with a wrong implementation. Any change to a device driver,
a report layout, or a transport must come with a reading from the real device.

Paste, at minimum:

- the device, its USB `VID:PID` (`lsusb`) or Bluetooth address, and the
  transport (`/dev/hidraw`, L2CAP, hwmon);
- the firmware or product revision if the device reports one;
- the command or short program you ran and its real output — the parsed value
  *and*, for a protocol change, the raw report bytes;
- for battery readings: the value alongside what the vendor tool or the
  device's own indicator shows, so the two can be compared. A plateaued
  percentage is often the real firmware value, not a bug;
- for telemetry: two readings far enough apart to show the value moving.

A reading against a synthetic or recorded transport is a test, not an e2e
reading. Say which device you tested and which supported devices you could not.

Report what you actually observed. Describe the hardware, the OS and the
software versions involved, the command you ran, and its real output. "Works on
my machine" is not a reading. If a reading cannot be taken — no access to the
device, platform not available to you — say so plainly in the PR and say what
*was* verified; a maintainer will decide whether to take the reading or hold the
PR. Claiming a reading that was not taken is the one thing that will get a
contributor's future PRs declined on sight.

## Scope

Direct device access for Linux, in Go, without cgo: peripheral batteries over
HID++ and vendor report protocols, AirPods over Bluetooth, NZXT Kraken
telemetry and LCD, and hwmon temperatures. It is the maintained home of the
device code that [hayami](https://github.com/ushineko/hayami) and
[hotaru](https://github.com/ushineko/hotaru) each once carried themselves.

In scope: new devices, protocol corrections, transports, and the public API
that the consuming projects depend on. Out of scope: application-level UI,
daemons, and anything that belongs in a consumer rather than in the library.

This is a library with downstream consumers. Public API changes are a direction
change: start a Discussion. `docs/api.md` and `docs/devices.md` are generated —
run `make generate`, never hand-edit them.

Changes that alter the project's direction — the interaction model, the
architecture, persistence formats, or the public interface — start as a GitHub
Discussion, not as a PR. A PR that changes direction without prior alignment
will likely be closed regardless of its quality.

## Development setup

```bash
git clone git@github.com:ushineko/sanshoku.git
cd sanshoku
make setup        # installs the pinned golangci-lint
make test         # no hardware required
make lint
make generate     # regenerates docs/api.md and docs/devices.md
```

Go 1.26 or newer, no cgo. `make test` runs without hardware. Exercising a real
device needs read/write access to its `/dev/hidraw` node or, for Bluetooth, an
L2CAP-capable adapter.

## What gets checked on your PR

| Required from you | Not required from you |
| --- | --- |
| `make test` passes | Spec files in `specs/` |
| `make lint` is clean | Validation reports |
| `make check-api` and `make check-support` pass (run `make generate`) | Version bump or release tag |
| `make check-no-binaries` passes | |
| Tests for the behaviour you changed | |
| An e2e device reading, pasted in the PR | |
| No secrets in code, logs, or pasted output | |
| No `Co-Authored-By` / AI-attribution trailers | |

The maintainer's own workflow (spec files under `specs/`, validation reports,
release tagging) is internal cadence. **External contributors are not expected
to write specs or validation reports, bump versions, or tag releases.** Bring a
clean, tested, in-scope change with its e2e reading and the maintainers handle
the bookkeeping on merge.

## Security and dependencies

- No hardcoded secrets or credentials, and none in logs or error messages.
- No `eval`/`exec` of dynamic input. Spawn subprocesses with explicit argument
  lists, never by interpolating into a shell string.
- No new network calls without prior discussion.
- Prefer the standard library. Open a Discussion before adding a dependency;
  a new third-party module in a PR is a decision for the maintainers, not a
  detail of the change.

## Commit and PR conventions

- Conventional-style subjects: `feat(...)`, `fix(...)`, `refactor(...)`,
  `docs(...)`, `test(...)`.
- One logical change per PR. Split unrelated work.
- No secrets or credentials, in code, in logs, in error messages, or in pasted
  e2e output. Redact serial numbers and hostnames if you would rather not
  publish them.
- Reference a related issue in the commit body with `refs #<number>`.
- Describe what changed, why, and how it was verified.

## Questions

Open a GitHub Discussion. Issues are for reproducible bug reports and
maintainer-created work items.

---

This policy is shared across the project author's public repositories; the
canonical copy lives in a private sysadmin repository and is rendered into each
project. Project-specific sections (scope, setup, checks, e2e) differ per repo;
the governance sections do not.
