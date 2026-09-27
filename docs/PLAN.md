# Plan

This document holds the plan for blitz, then every decision that shapes it,
then the open questions for Ken. blitz is a small library, so its decisions
stay in this file and are not split into one file each (dbmeta D111).

The table lists every decision with its status. A decision with the status
"Proposed" is not decided. It records what the code does now, and Ken decides
it.

| Decision | Title | Status |
| --- | --- | --- |
| [D1](#d1-blitz-wraps-the-c-interface-of-xoblitz-c-through-cgo-decided) | blitz wraps the C interface of xo/blitz-c through cgo | Decided |
| [D2](#d2-the-archives-are-committed-to-the-repository-decided) | The archives are committed to the repository | Decided |
| [D3](#d3-every-target-is-built-with-cross-under-podman-on-a-linux-host-decided) | Every target is built with cross under podman on a Linux host | Decided |
| [D4](#d4-blitzgo-names-the-archives-and-the-link-flags-of-each-platform-by-hand-amended-by-d5) | blitz.go names the archives and the link flags of each platform by hand | Amended by D5 |
| [D5](#d5-each-platform-is-its-own-go-module-and-build-blitzsh-generates-its-link-flags-decided-amends-d4) | Each platform is its own Go module, and build-blitz.sh generates its link flags | Decided. Amends D4 |
| [D6](#d6-an-archive-over-100-mib-is-split-into-archives-that-link-as-a-group-decided) | An archive over 100 MiB is split into archives that link as a group | Decided |
| [D7](#d7-an-archive-uses-the-release-profile-and-loses-its-debug-information-but-not-its-symbols-decided) | An archive uses the release profile, and loses its debug information but not its symbols | Decided |
| [D8](#d8-the-windows-archive-is-built-for-x86_64-pc-windows-gnu-decided) | The Windows archive is built for x86_64-pc-windows-gnu | Decided |
| [D9](#d9-the-windows-archive-keeps-its-symbols-external-decided) | The Windows archive keeps its symbols external | Decided |
| [D10](#d10-versiontxt-names-the-blitz-c-revision-and-all-six-targets-are-rebuilt-in-one-commit-decided) | version.txt names the blitz-c revision, and all six targets are rebuilt in one commit | Decided |
| [D11](#d11-the-directory-names-under-libblitz-are-module-paths-and-do-not-change-decided) | The directory names under libblitz are module paths and do not change | Decided |
| [D12](#d12-every-exported-operation-is-safe-for-concurrent-use-decided) | Every exported operation is safe for concurrent use | Decided |
| [D13](#d13-the-tests-run-with-the-race-detector-and-a-render-under-2048-bytes-fails-decided) | The tests run with the race detector, and a render under 2048 bytes fails | Decided |
| [D14](#d14-a-test-that-uses-the-network-runs-only-with-the-network-flag-decided) | A test that uses the network runs only with the network flag | Decided |
| [D15](#d15-the-markdown-stylesheet-has-its-own-method-and-is-not-a-field-of-options-decided) | The markdown stylesheet has its own method and is not a field of Options | Decided |
| [D16](#d16-the-print-media-type-changes-only-the-styles-and-a-pdf-holds-the-render-as-an-image-decided) | The print media type changes only the styles, and a PDF holds the render as an image | Decided |
| [D17](#d17-the-module-requires-go-127-decided) | The module requires Go 1.27 | Decided |
| [D18](#d18-a-release-tags-the-platform-modules-first-and-the-root-requires-them-by-hand-decided) | A release tags the platform modules first, and the root requires them by hand | Decided |
| [D19](#d19-ci-does-not-run-gofmt-proposed) | CI does not run gofmt | Proposed |
| [D20](#d20-ci-links-four-of-the-six-targets-proposed) | CI links four of the six targets | Proposed |
| [D21](#d21-the-agent-skills-are-committed-under-agents-and-claude-amended-by-d22) | The agent skills are committed under .agents and .claude | Amended by D22 |
| [D22](#d22-blitz-is-set-up-for-coding-agents-as-every-xo-repository-is-decided-amends-d21) | blitz is set up for coding agents as every xo repository is | Decided. Amends D21 |

## Purpose

blitz lets a Go program render HTML, markdown and web pages to images without
a browser, a window or a GPU. It also turns a render into a PDF file. It wraps
[xo/blitz-c](https://github.com/xo/blitz-c), a C interface to the Blitz
renderer from Dioxus Labs. A consumer runs `go get github.com/xo/blitz` and
needs no other step, because the static archives are in the repository.

## Current state

This section describes the repository on 2026-09-27, at `8a5767d`.

The root module is at v0.3.1. It requires the six platform modules at v0.1.2.
Those archives were built from blitz-c `4910b87`, which pins
dioxuslabs/blitz `7100a3b`. `version.txt` records the blitz-c revision.

The package has these parts:

- `blitz.go` holds `New`, `Context` and its render methods, `Options`,
  `DefaultOptions`, `MarkdownToHTML`, `Version` and `DefaultMarkdownStylesheet`.
- `pdf.go` holds `EncodePDF` and `WritePDF`, which write a PDF without a
  dependency.
- `cmd/blitz-render` is an example command that renders one file or URL.
- `build-blitz.sh` builds the archives and generates the Go files of each
  platform module.

The six targets are linux-amd64, linux-arm64, linux-armv7, macos-amd64,
macos-arm64 and windows-amd64. CI links and tests four of them (D20).

The repository has no open issue, no pull request and no TODO comment.
`docs/BACKLOG.md` holds the work that is known and not done.

## Direction

No direction is recorded beyond the backlog. The README names two fixes that
it expects: build the Linux archives in an older base image, and ship a DLL on
Windows. Neither is decided. Both are in [BACKLOG.md](BACKLOG.md) and in the
open questions below.

## Decisions

Each decision gives what was decided, where the history shows it, and the
reason. The reason comes from a commit message, a code comment or a README. If
none of these gives a reason, the decision says "Reason not recorded".

D1 to D21 were written on 2026-09-27 from the history, from `af2dcb8`
(2026-09-11) to `8a5767d` (2026-09-26). Ken made every commit in that range. A
decision is "Decided" when a commit of his made the choice on purpose. A
decision is "Proposed" when the history shows the result but not whether he
chose it.

A decision is never edited to change its conclusion. A later decision that
changes it says "Amends D4" in its heading, and the earlier one says "Amended
by D5" in its own. A bare number such as D3 means a decision in this file. A
decision of another repository names that repository, such as dbmeta D110.

### D1. blitz wraps the C interface of xo/blitz-c through cgo. Decided.

Since `af2dcb8`, the package calls the C functions that `libblitz/blitz.h`
declares. The header is a copy of the one in blitz-c. The package does not
bind the Rust code directly, and it has no API of its own beyond what the C
interface gives.

### D2. The archives are committed to the repository. Decided.

A consumer of `go get` receives what is in the repository and runs no build
step. If the archives are not committed, cgo has nothing to link. The
alternatives fail: the module proxy does not resolve Git LFS pointers, `go
build` cannot download a release asset, and a consumer does not run `go
generate`. `libblitz/README.md` gives the full reason. `.gitattributes` marks
each archive `binary -diff -merge`, so that git never converts, diffs or merges
one.

### D3. Every target is built with cross under podman on a Linux host. Decided.

`build-blitz.sh` builds every target with
[cross](https://github.com/cross-rs/cross), and there is no native build path.
One toolchain for every target means that an archive cannot differ by the
machine that built it. macOS and Windows cannot build the archives, which is a
second reason to commit them (D2). The Apple targets use cross-toolchain images
that each developer builds locally, because the Apple SDK is not
redistributable. `Cross.toml` installs the target libraries that each build
needs.

### D4. blitz.go names the archives and the link flags of each platform by hand. Amended by D5.

From `af2dcb8` to `ab91e63`, `blitz.go` held one `#cgo LDFLAGS` line per
platform. It named the archive pieces and the system libraries of each target.
When those lines and the archives disagreed, `build-blitz.sh` only warned.

### D5. Each platform is its own Go module, and build-blitz.sh generates its link flags. Decided. Amends D4.

`ab91e63` moved each target into a module of its own, such as
`github.com/xo/blitz/libblitz/linux-amd64`. The root imports it from a
build-tagged `link_GOOS_GOARCH.go`.

The six targets come to about 648 MiB. The go command refuses a module whose
zip or extracted content is over 500 MiB, so one module cannot hold them. With
one module per target, a build downloads only its own target, about 33 MiB.
A directory that holds a `go.mod` is left out of the zip of its parent, so the
root module stays at about 0.3 MiB.

`build-blitz.sh` now writes `lib.go`, `doc.go` and `go.mod` of each module and
the root `link_*.go`. The link flags come from the `SYSTEM_LIBS` table and from
the number of pieces, so there is one source and nothing to keep in step.
`make check-generated` fails in CI when a generated file is stale.

### D6. An archive over 100 MiB is split into archives that link as a group. Decided.

GitHub refuses a file over 100 MiB, and the linux-amd64 and linux-arm64
archives are larger. A consumer cannot run a step that joins the chunks of one
file. So `build-blitz.sh` splits the archive into `libblitz0.a`,
`libblitz1.a` and so on. Each piece holds a subset of the object files. The
pieces refer to each other, so GNU ld links them inside `--start-group` and
`--end-group`. Apple ld64 needs no group. The split uses `llvm-ar`, which
writes a symbol table for ELF, Mach-O and COFF alike.

### D7. An archive uses the release profile, and loses its debug information but not its symbols. Decided.

`build-blitz.sh` removes the LLVM bitcode and the debug information from each
archive with `llvm-objcopy --strip-debug`. That saves about 13% on linux-amd64.
It never removes symbols, so it never uses `--strip-unneeded`. The blitz-c
`production` profile is not used. It sets `lto = true`, which makes the archive
larger, 195.7 MiB against 129.9 MiB. `libblitz/README.md` holds the
measurement.

### D8. The Windows archive is built for x86_64-pc-windows-gnu. Decided.

cgo on Windows links with mingw gcc, and an archive from the MSVC target does
not combine with it cleanly. The gnu target also names its output
`libblitz.a`, so `-lblitz` is the same flag on every platform.

### D9. The Windows archive keeps its symbols external. Decided.

A binary that links blitz and a second Rust static library on Windows can get
duplicate symbols from the two copies of the Rust standard library. The
consumer adds `-Wl,--allow-multiple-definition` to its own flags. The README
explains why that is safe.

Hiding the symbols in the archive was tried and measured, and every variant
failed. `rust_eh_personality` is defined in one object and used by 552 others,
so no symbol can be hidden at the level of the archive, and mingw cannot do the
partial link that fixes it. "Windows: linking a second Rust static library"
in the README holds the full record. A DLL removes the problem, and it is in
the backlog.

### D10. version.txt names the blitz-c revision, and all six targets are rebuilt in one commit. Decided.

`version.txt` is the only record of what the archives were built from, so it
changes in the same commit as the archives. Each archive is tens of megabytes
and git keeps every version. A partial update adds to the size of the
repository and does not give a usable release. The README and
`libblitz/README.md` both state the rule, and `6888437` followed it.

### D11. The directory names under libblitz are module paths and do not change. Decided.

The names use the form `linux-amd64` and not the Go form `linux_amd64`. They
are the keys of the `TARGETS` table in `build-blitz.sh` and the module paths
that a consumer resolves. A rename breaks every consumer that resolved the old
path, so the names stay.

### D12. Every exported operation is safe for concurrent use. Decided.

The library reports an error through a slot that belongs to one OS thread. The
Go scheduler can move a goroutine to another thread between the call and the
read of the error. So every entry point holds `runtime.LockOSThread` across
both. `Close` takes the write lock of a `sync.RWMutex` and each render takes
the read lock, so `Close` waits for a render to finish before it frees the
context. `TestConcurrentErrorsAreNotCrossed` and `TestCloseDuringRender` hold
the two rules.

Concurrent renders wait in turn, because the library renders one at a time.
Stylo keeps its style state for the whole process. A caller that needs
parallel renders runs more than one process.

### D13. The tests run with the race detector, and a render under 2048 bytes fails. Decided.

CI runs `go test -race`, and `make test` does too. Most of the concurrency
tests prove nothing without the race detector. A render with no fonts is a
valid PNG that is almost empty, and the library reports success. So
`checkPNG` fails on a PNG under 2048 bytes, and CI fails on a render of the
example command under the same size.

### D14. A test that uses the network runs only with the network flag. Decided.

The tests are hermetic unless `go test` gets `-network`. The README gives the
reason: a test that fails on a train is a test that people learn to ignore. The
CI job that renders google.com has `continue-on-error`, because the site can
limit requests from a data center.

### D15. The markdown stylesheet has its own method and is not a field of Options. Decided.

A markdown render has three states: the built-in stylesheet, no stylesheet,
and a stylesheet of the caller. `RenderMarkdown` uses the built-in one.
`RenderMarkdownStyled` uses the one it gets, and an empty string gives no
stylesheet. A string field in `Options` cannot tell "not set" from "empty",
which is why the stylesheet is a method. `DefaultMarkdownStylesheet` returns
the built-in one, so that a caller can extend it.

### D16. The print media type changes only the styles, and a PDF holds the render as an image. Decided.

`ae77651` added `MediaType` and `pdf.go`. Blitz has no fragmentation, so
`@page` and `page-break-*` have no effect and a render is one continuous image.
`EncodePDF` embeds that image as a compressed RGB stream, which keeps the
package free of a dependency. With `Paginate`, it cuts the image into pages by
arithmetic, and a cut can go through a line of text.

### D17. The module requires Go 1.27. Decided.

The root `go.mod` has said `go 1.27` since `af2dcb8`. Code and tests can use
the standard library of Go 1.27, such as `slices.Sorted` and `maps.Keys`. The
platform modules say `go 1.21`, because they hold no Go code beyond a `#cgo`
line. The CI workflow installs Go 1.22, and the backlog holds that mismatch.

### D18. A release tags the platform modules first, and the root requires them by hand. Decided.

The `require` block of the root `go.mod` must name platform versions that
exist. So a release tags each platform module, such as
`libblitz/linux-amd64/v0.1.2`, pushes the tags, updates the `require` block,
and then tags the root. `libblitz/README.md` gives the steps.

The `replace` block points at the working tree, so the build here works before
the tags exist. It has no effect on a consumer. It also means that CI cannot
find a stale version in the `require` block. That cost a release: v0.3.0 shipped against the
v0.1.0 archives, and `8a5767d` fixed it in v0.3.1.

### D19. CI does not run gofmt. Proposed.

The first workflow had a gofmt step, and `4a51621` removed it four minutes
after `d1169d5` fixed other CI faults. Reason not recorded. The CI matrix has
a Windows runner, and a checkout with CRLF line endings makes gofmt report
every file. That is a guess and the history does not confirm it. `make lint`
still runs `gofmt -l -d .`. D22 adds `* text=auto eol=lf`, so a checkout now
gets LF endings on every runner. Ken decides whether CI runs gofmt again.

### D20. CI links four of the six targets. Proposed.

The CI matrix links and tests linux-amd64, linux-arm64, macos-arm64 and
windows-amd64. It has not linked linux-armv7 or macos-amd64 since `d1169d5`.
That commit removed the job that built the Linux targets with cross and the
step that checked the architecture of each archive. The comment that replaced
them says that the test matrix settles whether the archives link, which holds
for four targets only. The message of the commit, "Fix ci/cd issues", does not
say whether the smaller coverage was chosen. Ken decides.

### D21. The agent skills are committed under .agents and .claude. Amended by D22.

`ba03e43` added the `simple-english` and `go-pedantry` skills, with
`skills-lock.json`. The files were in `.agents/skills`, and each
`.claude/skills/<name>` was a symbolic link to them. A Windows checkout writes
a symbolic link as a small text file, and Claude Code then loads no skill and
reports nothing.

### D22. blitz is set up for coding agents as every xo repository is. Decided. Amends D21.

Ken decided on 2026-09-27 that every repository in the xo namespace is set up
for coding agents the same way (dbmeta D110). blitz is a small library, so its
decisions stay in this file (dbmeta D111). This change did the following:

- Each skill is an ordinary folder in `.agents/skills` and in `.claude/skills`,
  installed with `--copy`. `TestSkillsAreCopies` fails on a symbolic link, on a
  missing copy, on two copies that differ, and on a skill folder that
  `skills-lock.json` does not name.
- `AGENTS.md` holds the rules for a coding agent and opens with the three
  standing rules. `CLAUDE.md` holds one line, `@AGENTS.md`, and
  `TestClaudeImportsAgents` checks it.
- The root holds four documents: `README.md`, `AGENTS.md`, `CLAUDE.md` and
  `CONTRIBUTING.md`. Every other document goes in `docs/`, and
  `TestTheRootHoldsFourDocuments` checks it.
- This file holds the plan, the decisions and the open questions.
  `docs/BACKLOG.md` holds the work that is known and not done.
- `TestTheDecisionIndexIsComplete` checks that each decision has a row in the
  table at the top, with the same title and status.
  `TestAnAmendmentPointsBothWays` checks that an amendment is named from both
  sides. `TestEveryDecisionReferenceExists` checks that a bare number names a
  decision in this file.
- `.gitignore` ignores `.claude/settings.local.json`, which holds the Claude
  Code permissions of one person. `.gitattributes` holds
  `* text=auto eol=lf`.

## Open questions for Ken

1. Does CI run gofmt again? D19 records why it does not run now, and the
   reason is not recorded.
2. Does CI link linux-armv7 and macos-amd64? D20 records that it stopped in
   `d1169d5`. The linux-armv7 archive is 98.1 MiB, so the next update of
   blitz-c will probably split it, and no CI job links the pieces.
3. Is glibc 2.38 the floor for Linux, or do the Linux archives move to an older
   base image? The README says that the floor is almost certainly not
   intended.
4. Does Windows move from the static archive to a DLL? A DLL removes the
   duplicate symbols of D9 and is smaller. The cost is a `blitz.dll` beside
   each binary.
5. Does blitz get a `.golangci.yml`? The repository has none, and CI runs only
   `go vet`.
6. Which Go does CI install? The workflow names 1.22 and `go.mod` names 1.27
   (D17).
