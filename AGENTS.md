# blitz

`blitz` is the Go binding for [Blitz](https://github.com/dioxuslabs/blitz), a
native HTML and CSS renderer. It renders HTML, markdown and web pages to
images without a browser, and it turns a render into a PDF file. It calls the
C interface of [xo/blitz-c](https://github.com/xo/blitz-c) through cgo, and it
links prebuilt static archives that are committed to this repository.

## Standing rules

These hold in every `xo` repository, for every coding agent (dbmeta D110).

1. Stage changes for review. Commit and push only when Ken says so.
2. Load the simple-english skill before you write any text that a person
   reads: project documentation, a code comment, an error message or a commit
   message.
3. In a Go project, load the go-pedantry skill before you write or review Go
   code. A rule in this file wins where the two conflict.

`CLAUDE.md` holds one line that imports this file, so that Claude Code and
every other agent read the same rules. Edit this file, not that one.

## Which document to read

`docs/PLAN.md` holds the plan, every decision and the open questions. Read the
status of a decision, because some amend an earlier one, and two are only
proposed. Do not decide an open question on your own. Ask Ken.

| If you are | Read |
| --- | --- |
| using the package or the example command | `README.md` |
| asking why something is the way it is | the decisions in `docs/PLAN.md` |
| rebuilding the archives or updating blitz-c | "Rebuilding the archives" and "Updating" in `README.md`, then `libblitz/README.md`, D3, D6, D7 and D10 |
| changing the link flags of a platform | the `SYSTEM_LIBS` table in `build-blitz.sh`, then `make modules`. See D5 |
| tagging a release | "Releasing" in `libblitz/README.md` and D18 |
| changing a call into the C library | "Concurrency" in `README.md`, D12, and "Go conventions" below |
| adding a field to `Options` | "Go conventions" below, and `ae77651` as the example |
| a link failure on Windows or on an old Linux | "Platform notes" in `README.md`, D8 and D9 |
| looking for work that is known and not done | `docs/BACKLOG.md` |
| writing a document, a code comment, an error message or a commit message | the simple-english skill. Load it first |
| writing or reviewing Go code | the go-pedantry skill. Load it first |
| adding or updating an agent skill | "Agent skills" in `CONTRIBUTING.md`, and D22 |

`CONTRIBUTING.md` is the same guide for a person, and it is shorter.

A bare decision number, such as D3, means a decision in `docs/PLAN.md`. A
decision of another repository names that repository, such as dbmeta D110.
`TestEveryDecisionReferenceExists` fails on a bare number that `docs/PLAN.md`
does not hold.

## Layout

- `blitz.go` is the package: `New`, `Context` and its render methods,
  `Options`, `Error` and the status codes. The cgo preamble includes
  `libblitz/blitz.h`.
- `pdf.go` writes a PDF from a render, with no dependency.
- `link_GOOS_GOARCH.go` is one blank import per platform, gated by a build
  constraint. `build-blitz.sh` generates each one. Do not edit them.
- `libblitz/blitz.h` is the header, copied from blitz-c.
- `libblitz/<target>/` is one Go module per target, such as
  `github.com/xo/blitz/libblitz/linux-amd64`. It holds the archives,
  `native-static-libs.txt`, and the generated `go.mod`, `doc.go` and `lib.go`.
  The directory names are module paths and do not change (D11).
- `build-blitz.sh` builds the archives with cross and generates the Go files.
  `Cross.toml` holds the images and the target libraries that cross uses.
- `version.txt` names the blitz-c revision that the archives came from (D10).
- `cmd/blitz-render/` is an example command.
- `testdata/` holds `sample.md` and `sample.html`. The tests write their
  renders to `testdata/output/`, which git ignores.
- `docs/` holds `PLAN.md` and `BACKLOG.md`.

The root `go.mod` requires the six platform modules, and a `replace` block
points each one at its directory here. The `replace` block works only in this
repository. The `require` versions change by hand at a release (D18).

## Build and test

```bash
gofmt -l .
go vet ./...
go test -race ./...
```

`gofmt -l .` must print nothing. Run the tests with `-race`, because most of
the concurrency tests prove nothing without it (D13). The first build links
more than 100 MiB of archives, so it is slow.

The Makefile wraps the same commands:

- `make test` runs `go test -race -v ./...`.
- `make test-net` adds `-network`, which renders google.com (D14).
- `make test-short` skips the slow concurrency tests.
- `make lint` runs `go vet` and `gofmt -l -d .`.
- `make check-generated` fails when a generated file is stale. CI runs it.
- `make modules` regenerates the Go files of the platform modules. It needs no
  container.

The repository has no `.golangci.yml`, and CI does not run golangci-lint.

A render needs fonts. On Linux, install `fontconfig` and one font family, such
as `fonts-dejavu-core`. Without fonts, the library returns an almost empty PNG
and reports success, and `checkPNG` then fails the test.

Do not run `build-blitz.sh` or `make libs` unless the task is to rebuild the
archives. It starts a container for each target and takes a long time. If you
rebuild, rebuild all six targets in one commit with `version.txt` (D10).

## Go conventions

These are the conventions that the code follows now.

Every call into the C library that can fail runs inside `Context.call`, or
holds `runtime.LockOSThread` until it reads the error. The library keeps the
error in a slot that belongs to one OS thread (D12). `call` also takes the
read lock, returns `ErrClosed` after `Close`, and calls `runtime.KeepAlive` so
that the finalizer cannot free the context during a render. A render that also
encodes, such as `renderToPNG`, does both steps inside one `call`.

Free C memory in the function that allocates it. Each `C.CString` gets a
`defer C.free`. Each `BlitzImage` and `BlitzBuffer` gets `blitz_image_free` or
`blitz_buffer_free`. Copy the data into Go memory before you free it, as
`toImage` and `C.GoBytes` do.

A null C string means "use the default" and an empty string is a value.
`optionalCString` keeps the two apart, and the markdown stylesheet depends on
it (D15).

The library reports a failure as `*Error`, with `Op`, `Code` and `Message`.
`Op` is the name of the C function without `blitz_`, such as `render_url`. A
caller branches on `Code` with the `Code*` constants. `ErrClosed` is a
sentinel error, and a caller compares it with `errors.Is`. Every error that
the package makes starts with `blitz:`. An error from `os` or `compress/zlib`
passes through without context now, and the backlog holds that.

`Options` has no meaningful zero value, and a caller starts from
`DefaultOptions`. A new option needs a field in `Options`, a line in
`DefaultOptions` and a line in `toC`. `ae77651` added `MediaType` that way.

Receivers are `c` for `*Context`, `o` for `Options` and `e` for `*Error`. Every
exported name has a doc comment. A comment says why the code does something,
and the comments in `blitz.go` give the reason for each rule above.

The root module depends on nothing outside the standard library except its own
platform modules. `pdf.go` writes the PDF by hand to keep it that way (D16). A
new dependency changes that, so ask Ken first.

The tests are in package `blitz`. They share one `Context` through `ctx(t)`,
because a `Context` starts worker threads and sharing one finds data races.
`testOptions` turns the network off. `checkPNG` checks the PNG header, a size
of at least 2048 bytes and a decodable image. A test that uses the network
skips unless `*network` is set, and a slow test skips under `-short`. The
tests of the repository layout are in package `blitz_test`, in
`skills_test.go` and `docs_test.go`.

## Writing documentation

A new document goes in `docs/`. Only `README.md`, `AGENTS.md`, `CLAUDE.md` and
`CONTRIBUTING.md` go in the root, and `TestTheRootHoldsFourDocuments` fails on
any other. `libblitz/README.md` stays beside the archives that it describes.
Add a new document to the table above and to the list in `README.md`.

A decision goes in `docs/PLAN.md`, with the next number, and nowhere else. Its
heading is `### D<n>. <title>. <status>.`, where the status is `Decided`,
`Proposed`, `Amends D<n>` or `Amended by D<n>`, and a heading can join two with
a full stop, as in `Decided. Amends D4.` Add its row to the table at the top.
`TestTheDecisionIndexIsComplete` prints the row when it is missing. If a
decision changes an earlier one, write it in both headings, and
`TestAnAmendmentPointsBothWays` checks it. If you do not know whether Ken
decided something, write "Proposed" and ask him.

Work that is known and not done goes in `docs/BACKLOG.md`, with where it came
from. When an item is done, delete it, and record in `docs/PLAN.md` anything
that was decided.
