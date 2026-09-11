# libblitz

Prebuilt static archives, committed to the repository, one Go module per
platform.

```
libblitz/
  blitz.h                             header, copied from the blitz-c repo
  linux-amd64/go.mod                  module github.com/xo/blitz/libblitz/linux-amd64
  linux-amd64/doc.go                  generated
  linux-amd64/lib.go                  generated: the #cgo LDFLAGS for this target
  linux-amd64/libblitz0.a             split: over the 100 MiB file limit
  linux-amd64/libblitz1.a
  linux-amd64/native-static-libs.txt
  linux-arm64/...
  linux-armv7/...
  macos-amd64/libblitz.a
  macos-arm64/libblitz.a
  windows-amd64/libblitz.a
```

Everything here is produced by `../build-blitz.sh`. Only `build.log` is
gitignored.

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

## Why each platform is its own module

The six targets come to ~648 MiB. The go command refuses a module whose zip
*or extracted content* is over 500 MiB (`golang.org/x/mod/zip.MaxZipFile`), so
shipping them in the root module does not work at all — it is a hard failure,
not a slow download.

Even under the cap it would be the wrong shape: one module means every consumer
pays for all six targets to link one.

Splitting fixes both. Each target gets its own `go.mod`, and the root module
imports it from a build-tagged `link_GOOS_GOARCH.go`. The go command downloads
a module's zip only when a package it provides is actually in the build, so a
linux/amd64 build fetches the linux-amd64 module and nothing else. Directories
holding a `go.mod` are excluded from the parent module's zip, so the root module
stays small (~0.3 MiB) rather than carrying the archives twice.

Measured on a cold module cache against a committed `go.mod`/`go.sum`:

| | download |
|---|---|
| root module | 0.3 MiB |
| one platform module | ~29-33 MiB |
| **per consumer build** | **~33 MiB** |
| all six, as one module | ~194 MiB — and over the cap, so impossible |

The one rough edge: `go get` and `go mod tidy` walk every `GOOS`/`GOARCH`, so
the first of either in a consumer's tree fetches all six to record `go.sum`
hashes. Ordinary builds afterwards fetch one.

### Releasing

Nested modules are tagged with a path prefix, and the root module's `require`
block has to name versions that already exist. So the order is:

1. Tag each platform module: `libblitz/linux-amd64/v0.1.0`, and so on.
2. Push those tags and let the proxy see them.
3. Update the `require` block in the root `go.mod` to match.
4. Tag the root module.

The `replace` block in the root `go.mod` points at the platform modules in the
working tree, so `go build`, `go test` and `go mod tidy` all work here before
any of that happens, and afterwards use the archives in the tree rather than
whatever was last published. It has no effect on consumers: the go command
applies replace directives from the main module only and ignores them in
dependencies.

The flip side is that CI cannot check the require block, because the replaces
override it locally. Step 3 is the one to get right by hand.

## Archives over 100 MiB are split

GitHub refuses any single file larger than 100 MiB, and `libblitz.a` for the
64-bit Linux targets is past that. Since a consumer never runs anything from
this repository, it cannot reassemble a file that was chunked — so the archive
is divided into several *archives* instead, `libblitz0.a`, `libblitz1.a`, …,
each holding a subset of the object files. `build-blitz.sh` does this
automatically for any target whose archive would be over the limit, and leaves
smaller targets as a single `libblitz.a`.

Note this is GitHub's limit, not Go's — the module cap is handled by the split
above.

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

`lib.go` is generated with whatever `-l` flags the split produced, so a change
in the number of pieces cannot quietly break downstream links.

## Archives are slimmed before committing

`build-blitz.sh` runs every archive through `llvm-objcopy` to drop two things
that a cgo link never uses:

- `.llvmbc` / `.llvmcmd` — LLVM bitcode. Cargo builds this crate's own
  dependencies with `-Cembed-bitcode=no`, but `std`, `core` and `alloc` come
  out of rustup's precompiled rlibs, which carry it.
- debug info — `std` ships with it and the release profile's `strip = "none"`
  keeps it.

Worth about 13% on linux-amd64 (129.9 → 112.2 MiB). Symbols are left alone:
`--strip-debug`, never `--strip-unneeded`.

This is *not* the same as building blitz-c's `production` profile. That sets
`lto = true`, which forces `embed-bitcode` back on, and because rustc defers LTO
for a staticlib to whoever links it, the archive comes out **larger** — 195.7
MiB against the release profile's 129.9 MiB. Measured, not guessed.

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

The names here are the module paths, and they're also the keys of the `TARGETS`
map in `build-blitz.sh`. Renaming one is a breaking change for anything that
already resolved it.

## native-static-libs.txt

Records the system libraries that archive expects to be linked against,
captured during the build via
`CARGO_TARGET_<TRIPLE>_RUSTFLAGS=--print=native-static-libs`. If a build fails
with undefined symbols, diff this against `SYSTEM_LIBS[<target>]` in
`build-blitz.sh`, then regenerate with `./build-blitz.sh --modules-only`.
