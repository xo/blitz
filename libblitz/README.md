# libblitz

Prebuilt static archives, committed to the repository.

```
libblitz/
  blitz.h                             header, copied from the blitz-c repo
  linux-amd64/libblitz0.a             split: over the 100 MiB file limit
  linux-amd64/libblitz1.a
  linux-amd64/native-static-libs.txt
  linux-arm64/libblitz.a
  linux-armv7/libblitz.a
  macos-amd64/libblitz.a
  macos-arm64/libblitz.a
  windows-amd64/libblitz.a
```

Everything here is produced by `../build-blitz.sh`. Only `build.log` is
gitignored.

## Archives over 100 MiB are split

GitHub refuses any single file larger than 100 MiB, and `libblitz.a` for the
64-bit Linux targets is past that. Since a consumer never runs anything from
this repository, it cannot reassemble a file that was chunked — so the archive
is divided into several *archives* instead, `libblitz0.a`, `libblitz1.a`, …,
each holding a subset of the object files. `build-blitz.sh` does this
automatically for any target whose archive would be over the limit, and leaves
smaller targets as a single `libblitz.a`.

The pieces have to be linked as a group, because they refer to each other's
symbols in both directions and GNU ld would otherwise give up after a single
left-to-right pass:

```
-Wl,--start-group -lblitz0 -lblitz1 -Wl,--end-group
```

Apple's `ld64` has no `--start-group` and needs none — it resolves the whole set
of archives together — so the macOS targets just list the pieces. Splitting uses
`llvm-ar` rather than GNU `ar`: it writes symbol tables for ELF, Mach-O and COFF
archives alike, so one code path covers all six targets from the Linux build
host.

`build-blitz.sh` prints the exact `#cgo` line each target needs and warns when
`blitz.go` disagrees, so a change in the number of pieces cannot quietly break
downstream links.

## Why these are committed

`go get github.com/xo/blitz` has no build step. A consumer fetching the module
receives exactly what is in the repository and never runs `build-blitz.sh`, so
if the archives aren't committed, cgo has nothing to link against and the
package fails to build for everyone downstream.

That rules out the usual alternatives:

- **Git LFS** would break it. The Go module proxy fetches repositories without
  resolving LFS pointers, so consumers would get small text files where the
  archives should be.
- **Release assets** would break it. There's no hook to download them during
  `go build`.
- **A `go:generate` step** would break it. Consumers don't run `go generate`.

So the archives go in the tree. They're marked `binary -diff -merge` in
`.gitattributes` so git never tries to diff or merge them, and
`linguist-vendored` so they don't skew the repository's language stats.

## Keeping the repository from bloating

Each archive is tens of megabytes and git keeps every historical version
forever. A few habits help:

- Rebuild and commit **all six targets together**, in one commit, when bumping
  `blitz-c`. Partial updates multiply the stored blobs without making any
  release usable.
- Don't commit intermediate or debugging builds. `./build-blitz.sh <target>`
  overwrites that target's archive in place, so check `git status` before
  committing and revert any archive you weren't intending to update.
- `version.txt` in the repository root records which `blitz-c` tag or revision
  the current archives came from. Update it in the same commit.

## Directory names are interface

The names here are referenced by hand in the `#cgo` directives in `blitz.go`,
and by the `TARGETS` map in `build-blitz.sh`. Renaming one means editing all
three.

## native-static-libs.txt

Records the system libraries that archive expects to be linked against,
captured during the build via
`CARGO_TARGET_<TRIPLE>_RUSTFLAGS=--print=native-static-libs`. If a build fails
with undefined symbols, diff this against the `LDFLAGS` for the matching
platform in `blitz.go`.
