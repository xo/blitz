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

### Find out why slimming does nothing for windows-arm64

`slim_archive` removes the LLVM bitcode and the debug information. It saved 13%
on linux-amd64 and left windows-arm64 at 134 MiB, the same size as before. Check
whether the archive has either one, or whether `llvm-objcopy` skips the COFF
members. If the archive is under 100 MiB after that, it needs no split.

Source: the build of `windows-arm64` on 2026-10-14.

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

Source: found in the setup of D22. Open question 6 in `PLAN.md` asks Ken.

### Add CI jobs for the four new targets

CI links four of the ten targets (D20). The new ones need a job each: an Alpine
container for the two musl targets, the `windows-11-arm` runner for
`windows-arm64`, and a way to run FreeBSD that does not start a virtual machine
in this repository's work. `make check-generated` already covers the generated
files of all ten.

Source: D23. Open question 2 in `PLAN.md` asks Ken.

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
