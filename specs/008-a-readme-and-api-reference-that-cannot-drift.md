# 008 — a README and API reference that cannot drift

**Issue**: #16

## Status: COMPLETE

## Context

After seven specs the README's package table, documentation list and
changelog are complete, every exported identifier in every public package
has a doc comment, and nothing checks any of it. `docs/devices.md` is the
one page that cannot drift, because `make check-support` regenerates it;
the rest was checked by hand once, at v0.1.0.

fynedesygn's README drifted twenty-four releases before a test read it
(its spec 018), and its spec 022 generates a signature index of the public
surface for the same reason: a hand-written API guide goes stale the way
the README did, and a PR that widens a `v0.x` module's public surface
should show that where a reviewer looks. This spec brings both canaries
here, and fixes the three README gaps the audit found: the Testbench
section does not mention `support` or `screen --hold`, the Development
block lists eight of the Makefile's thirteen targets, and the usage
example is prose that nothing compiles.

## Requirements

### R1. `readme_test.go` at the module root

Each test names what is missing and the fix is to write the line, never to
relax the test.

- R1.1 Every package `go list ./...` reports, except `package main`
  commands, has a row in the "What is in it" table (`[`name`]`), and
  every command has one too.
- R1.2 Every `make` target with a `##` description has a `make <target>`
  line in the Development block, except a documented plumbing list
  (`help`, `install-lint`, `clean`).
- R1.3 Every `docs/*.md` is linked from the Documentation list.
- R1.4 The `**Version**` line equals the newest `### X.Y.Z (date)`
  changelog heading, and an `### Unreleased` heading, when present, is
  above it.
- R1.5 The usage example is a Go Example (`example_test.go` in package
  `sanshoku_test`, importing `all` and `battery`) so it compiles, and the
  README's code block is that example's body verbatim, checked by the test.

### R2. `docs/api.md`, generated

- R2.1 `cmd/sanshoku-apidoc` writes a signature index: one section per
  public package in import-path order, the package synopsis, then
  constants, variables, types (with methods and the type's constructors
  under it) and functions, each with its signature and the first sentence
  of its doc comment. Package `main` is excluded. `go/doc` and
  `go/parser`, no network, no cgo.
- R2.2 `make generate` writes it beside `docs/devices.md`; `make
  check-api` regenerates and fails on a diff, and CI runs it next to
  `check-support`.
- R2.3 `api_test.go` at the root regenerates in memory and compares with
  the committed file, naming the first differing line.
- R2.4 `api_test.go` also fails on an exported identifier in a public
  package without a doc comment, naming package and identifier. Interface
  method implementations count; the audit found none today.
- R2.5 The README's Documentation list links `docs/api.md`; the "What is
  in it" table gets the command's row.

### R3. README repairs

- R3.1 Testbench: `support` (and `--markdown`), `screen --yes --hold D`,
  and that `verify` compares against liquidctl, solaar and headsetcontrol.
- R3.2 Development: `coverage`, `check-no-binaries`, `generate`,
  `check-support`, `check-api`.
- R3.3 Changelog entry under `### Unreleased`.

## Acceptance Criteria

- [x] `make test` and `make lint` pass; `make check-api` and
  `make check-support` pass; CI runs both.
- [x] Removing a package row from the README makes `readme_test.go` fail,
  naming the package (verified by hand and reverted, noted below).
- [x] Changing a doc comment's first sentence makes `api_test.go` fail
  until `make generate` is run (verified by hand and reverted).
- [x] Deleting one exported identifier's doc comment makes `api_test.go`
  fail naming it (verified by hand and reverted).
- [x] `docs/api.md` is committed, linked, and lists every public package.
- [x] The README's example block equals the Example's body.

## Risks & Assumptions

- **A signature index, not a transcript.** pkg.go.dev renders the prose;
  the index exists so a widened surface shows in a diff.
- **Rollback**: revert.

## Implementation notes

- The generator lives in `internal/apidoc` so that `cmd/sanshoku-apidoc`
  and `api_test.go` share it without widening any public package (a test
  cannot import package main). `internal/apidoc` is a package `go list`
  reports, so by R1.1 it has a README row; it is not public, so
  `docs/api.md` does not index it and R2.4 does not apply to it.
- The index follows go/doc's grouping: a type's typed constants and
  variables, its constructors and its methods are listed under it. Each
  constant and variable is one line (`const BandCritical Band`), a type's
  declaration is printed as gofmt would with every comment stripped, and
  unexported fields show as go/doc's "contains filtered or unexported
  fields" marker. Struct fields and interface methods are part of the
  printed declaration and are not separately required to carry a doc
  comment.
- A constant or variable counts as documented when it, or its group, has a
  comment, including a trailing line comment.
- The README's "Using it" block is the Example body with one tab of
  indentation removed, tabs kept. The Example has no `// Output:`, so
  `go test` compiles it and never runs it.

## Verification

2026-09-29, in the worktree, Go 1.27.1 (and `GOTOOLCHAIN=go1.26.0 go test .`
for the minimum).

- `make test`: pass. `make lint`: 0 issues. `CGO_ENABLED=0 make build`:
  pass. `make generate` twice: `docs/api.md` byte-identical
  (sha1 unchanged), `docs/devices.md` unchanged. `make check-support`,
  `make check-api`, `make check-no-binaries`: pass. CI's Test job runs
  `go test -race ./...` (so both canaries) and gains a `make check-api`
  step after `make check-support`.
- `docs/api.md`: 16 sections, `sanshoku` then `all` through `usbfs` in
  import-path order; no `cmd/` or `internal/` package.
- Before the README repairs, the new canaries failed on
  `cmd/sanshoku-apidoc` having no row, `make coverage` missing from the
  Development block, and the old prose example not being Example's body.

Deliberate breakage, each reverted afterwards:

1. Deleted the `hwmon` row from "What is in it":
   `TestEveryPackageIsInTheReadme` failed with
   `package hwmon has no row in the What is in it table`. Deleting the
   `docs/api.md` line from the Documentation list at the same time also
   failed `TestEveryDocumentIsLinkedFromTheReadme` with
   `docs/api.md is not linked from the Documentation list`.
2. Changed `Scan`'s first sentence to "Scan asks each driver in turn and
   returns every candidate found.": `TestTheAPIReferenceIsCurrent` failed
   with `docs/api.md is stale at line 54: run make generate`, printing the
   committed and generated lines. After `make generate` it passed; the
   source and the page were then restored and `make check-api` passed.
3. Deleted the doc comment on `battery.Band.Segments`:
   `TestEveryExportedIdentifierIsDocumented` failed with
   `battery.Band.Segments is exported and has no doc comment`, and the
   staleness test failed at line 162 (the sentence gone from the index).
