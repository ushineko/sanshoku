# 011 — builds off Linux

**Issue**: #33

## Status: COMPLETE

## Context

hayami imports this module and is being brought up on Windows. It could not
be built there at all: `hidraw/feature.go`, `l2cap/l2cap.go` and
`usbfs/usbfs.go` call `golang.org/x/sys/unix` with no build constraint, and on
Windows that package has none of `Syscall`, `SYS_IOCTL`, `Errno` or the
Bluetooth socket constants. Every package that reaches one of the three
failed to compile, which is most of the module.

Nothing in hayami needs a device driver to *work* on Windows for the panel to
run — the drivers only need to find nothing. Reading devices on Windows is a
separate question with its own spec, if it comes.

The same Windows checkout surfaced two smaller faults: a `/dev` path joined
with `path/filepath` reads `\dev\hidraw12` on Windows, and the byte-for-byte
tests (the API reference, the README example) failed on a CRLF checkout,
which is Git for Windows' default.

## Requirements

- R1 The three files carry `//go:build linux`. Each has a `!linux`
  counterpart declaring the same exported identifiers, each returning an
  error that wraps `errors.ErrUnsupported`: `hidraw.(*Handle).SetFeature`,
  `GetFeature`; `l2cap.Conn`, `Dial`, `(*Conn).Send`, `Receive`, `Close`;
  `usbfs.Interface`, `Open`, `(*Interface).Bulk`, `Close`.
- R2 `l2cap.ParseAddress` is pure and moves to a file with no constraint.
- R3 `internal/apidoc` selects files as a linux/amd64 build would, so
  `docs/api.md` documents the Linux surface whatever host generates it and is
  unchanged by this spec.
- R4 hidraw node paths and usbfs paths are joined with `path`.
- R5 `.gitattributes` pins LF for every text file.
- R6 Tests that depend on Linux behaviour are constrained, not weakened:
  the pipe-backed exchange test is `//go:build linux` (Windows pipes take no
  read deadline); the NZXT serialisation test uses a freshness of zero rather
  than a nanosecond, which reads as fresh under Windows' clock resolution.
- R7 CI gains a job that runs `go test ./...` on `windows-latest` and vets
  with `GOOS=darwin`.
- R8 README: the platform paragraph says the module builds elsewhere and
  finds nothing there; changelog under `### Unreleased`.

## Acceptance Criteria

- [x] `GOOS=linux`, `GOOS=windows` and `GOOS=darwin go vet ./...` pass.
- [x] `go test ./...` passes on Windows (an LF checkout).
- [x] `make test`, `make lint`, `make check-api`, `make check-support` pass
  on Linux (CI).
- [x] `docs/api.md` is unchanged.
- [x] hayami builds for Windows against this branch.

## Risks & Assumptions

- **No behaviour change on Linux.** The Linux files are unchanged apart from
  the constraint and `ParseAddress` moving out; `path.Join` and
  `filepath.Join` agree on Linux.
- **Assumption**: the library rule "Linux only" is about what works, not
  what compiles. This spec keeps the first and changes the second.
- **Renormalising line endings** touches no committed content: the
  repository already stores LF.
- **Rollback**: revert.

## Verification

2026-09-30, Windows 11 Pro 26200, Go 1.26.0, `CGO_ENABLED=0`:

- `go vet ./...` with `GOOS` linux, windows and darwin: clean.
- `go test -count=1 ./...`: every package ok; the pipe exchange test is
  constrained out, as R6 says.
- `go run ./cmd/sanshoku-apidoc` matches `docs/api.md` byte for byte, and
  `sanshoku-bench support --markdown` matches `docs/devices.md`.
- hayami (its spec 021) builds both programs for Windows against this
  branch and its suite passes; on that machine every driver reports finding
  nothing, and the panel draws.
- `govulncheck`: two standard-library findings from the local Go 1.26.0
  (fixed in 1.26.1 and 1.26.3), none in this module's code.

Linux: PR #34 CI passed `go test -race`, `make lint`, `make build`,
`make check-support` and `make check-api`, and the new Windows and macOS job.
