# Backlog

This document lists the work that is known and not done. Each item names where
it came from.

The repository has no open issue, no pull request and no TODO comment, so
every item here comes from a README, a commit or the setup in D22. A decision
is not a backlog item, and it goes in [PLAN.md](PLAN.md). When an item is
done, delete it, and record in `PLAN.md` anything that was decided on the way.

## Archives

### Lower the glibc floor of the Linux archives

All three Linux archives need glibc 2.38 or newer, because the `aws-lc` C
sources use `__isoc23_sscanf` and `__isoc23_strtol`. A link against an older
glibc fails, which excludes Debian 12, Ubuntu 22.04, RHEL 9 and Amazon Linux
2023. The fix is to build the Linux targets in an older base image and rebuild
them. The floor then becomes the glibc of that image.

Source: "Linux needs glibc 2.38 or newer" in the README, added in `6093403`.
Open question 3 in `PLAN.md` asks Ken whether to do it.

### Ship a DLL on Windows

A DLL exports only the C interface, so the duplicate symbols of D9 cannot
happen. It is also smaller: on Linux the shared library is 30.0 MiB against
129.9 MiB for the static archive. blitz-c already builds a `cdylib`. The cost
is a `blitz.dll` beside each binary.

Source: "The real long-term fix" in the README. Open question 4 in `PLAN.md`
asks Ken.

### Prepare for the split of the linux-armv7 archive

The linux-armv7 archive is 98.1 MiB, 1.9 MiB under the file limit of GitHub.
The next update of blitz-c will probably push it over, and `build-blitz.sh`
will then split it (D6). No CI job links linux-armv7 (D20), so nothing will
show whether the pieces link.

Source: the message of `6888437`.

### Run windows-arm64 and freebsd-amd64

Both archives link into a Go test binary with `zig cc`, and nobody has run
either. `windows-arm64` can run on the `windows-11-arm` runner of GitHub. The
FreeBSD archive links against `execinfo`, `kvm`, `memstat`, `procstat` and
`devstat`, which zig does not ship, so a real FreeBSD host is the only check of
those flags.

Source: D24.

### Keep llvm-objcopy from failing silently

`slim_archive` now slims member by member when `llvm-objcopy` refuses an archive.
It still prints "slimmed" for any other outcome, and it trusts the test to find a
section that is left. Make the script scan the archive for the sections of D29
and stop when one is left, so that a bad archive does not reach the test.

Source: D29.

### Teach update-pins.sh in blitz-c about the git AnyRender source

`scripts/update-pins.sh` in blitz-c rewrites `rev = "..."` lines and assumes that
blitz uses AnyRender from crates.io, with a `[patch.crates-io]` entry. Since blitz
`c27b4ca` the source is the `main` branch of the git repository, and the commit
is pinned in `Cargo.lock` (D31). The script also needs the `vello` revision from
the lock file of blitz. Until it does, an update is a manual job: pin blitz, read
the AnyRender and vello revisions from the lock file of that blitz, and
`cargo update -p anyrender --precise <commit>`.

Source: D31.

### File the two Blitz defects, and drop the markdown workarounds

Nobody has filed either defect on DioxusLabs/blitz (D32). Each issue needs the
version of blitz (`c27b4ca`) and a reproduction.

1. An outside ordered list marker has no trailing space. With
   `<ol><li>Ordered item</li></ol>` the render shows "1.Ordered item", and with
   `<ul>` it shows a bullet and a gap. A browser leaves a gap after the marker.
   `li::marker { content: ... }` has no effect.
2. A tab in `white-space: pre` text draws the missing-glyph box, in
   `<pre>a<newline><tab>b</pre>`. `tab-size` and `white-space: pre-wrap` have no
   effect, and spaces draw correctly. The tab should advance to the next tab
   stop, 8 columns by default.

When Blitz fixes one, remove the matching workaround from `src/markdown.rs` in
blitz-c, which is the `ol > li` rule or `expand_tabs_in_pre`, and the tests of D32.

Source: D32, and the report of the iv session on 2026-11-12.

### Drop the vello patch and the reqwest hold

blitz-c patches four vello crates to a development branch and stays on reqwest
0.13.1 (D31). Remove the patch when blitz stops patching glifo. Move reqwest when
blitz-c has a plan for its root certificates.

Source: D31.

## Releases

### Check the require block before a release

The `replace` block in the root `go.mod` hides the `require` block from every
build here, so CI cannot find a stale version. v0.3.0 shipped against the
v0.1.0 archives because nobody updated the block, and `8a5767d` fixed it. A
step before each tag can compare the `require` versions with the newest
`libblitz/<target>/v*` tags.

Source: `8a5767d`, and "Releasing" in `libblitz/README.md` (D18).

## CI

### Install the Go of go.mod in CI

`.github/workflows/ci.yml` installs Go 1.22 in both jobs that run tests, and
`go.mod` says `go 1.27` (D17). The go command downloads the newer toolchain on
its own, so CI passes, but the version in the workflow is not the version
that runs. `actions/setup-go` can read `go-version-file: go.mod` instead.

This hid the fault of D29. CI links the macOS archive on `macos-latest`, and
Go 1.27.4 refused it for every consumer on that Go while CI passed. CI needs a
job on the newest Go, as well as the oldest that `go.mod` allows.

Source: found in the setup of D22. Open question 6 in `PLAN.md` asks Ken.

### Add CI jobs for the four new targets

CI links four of the ten targets (D20). The new ones need a job each: an Alpine
container for the two musl targets, the `windows-11-arm` runner for
`windows-arm64`, and a way to run FreeBSD that does not start a virtual machine
in this repository's work. `make check-generated` already covers the generated
files of all ten.

On the `windows-11-arm` runner, use clang from llvm-mingw as the C compiler. The
magic repository passed its tests there with llvm-mingw 20261006, the aarch64
ucrt zip. The native arm64 zig 0.16.0 crashed there with exit code 0xc0000005 in
cgo, so do not use zig as the compiler on that runner. GitHub has no hosted
FreeBSD runner.

Source: D23, and a report from the magic session on 2026-10-14. Open question 2
in `PLAN.md` asks Ken.

### Decide whether CI runs gofmt and a linter

CI runs `go vet` and not gofmt (D19), and the repository has no
`.golangci.yml`.

Source: `4a51621`, and the setup of D22. Open questions 1 and 5 in `PLAN.md`
ask Ken.

## Code

### Fix the comments in build-blitz.sh that name blitz.go

Five comments in `build-blitz.sh` say that `blitz.go` holds the `#cgo`
directives or needs the `-l` flags. That stopped in `ab91e63`, when the flags
moved to the generated `lib.go` of each platform module (D5). The
`GO_PLATFORM` table says that it checks those directives, and nothing reads
it.

Source: found in the setup of D22.

### Wrap the errors that pass through without context

`WriteURLPNG`, `WriteMarkdownPNG`, `WriteHTMLPNG` and `WritePDF` return the
error of `os.WriteFile` as it is. `rgbStream` returns the errors of
`compress/zlib` as they are. The go-pedantry skill wraps each one with `%w` and
the name of the step, such as `writing %s: %w`.

Source: a go-pedantry review of `blitz.go` and `pdf.go` in the setup of D22.
