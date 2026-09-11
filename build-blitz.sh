#!/usr/bin/env bash
#
# build-blitz.sh - build libblitz.a from github.com/xo/blitz-c for each target
# and install it into ./libblitz/<os>-<arch>/.
#
#   ./build-blitz.sh                    # every target
#   ./build-blitz.sh linux-amd64
#   ./build-blitz.sh linux-arm64 windows-amd64
#   ./build-blitz.sh --rev a1b2c3d      # pin a commit instead of the latest tag
#   ./build-blitz.sh --local ../blitz-c # build from a working copy
#   ./build-blitz.sh --engine docker    # override the container engine
#   ./build-blitz.sh --slim-only        # strip dead weight from committed archives
#   ./build-blitz.sh --modules-only     # regenerate libblitz/*/go.mod and lib.go
#
# Every target is built with `cross` under podman, including the Apple targets,
# which use locally built cross-toolchain images. There is no native build path:
# one toolchain for every target means the archives can't differ by which
# machine produced them.
#
# Layout produced:
#
#   libblitz/blitz.h
#   libblitz/linux-amd64/libblitz.a
#   libblitz/linux-amd64/native-static-libs.txt
#   ...
#
# An archive over 100 MiB is split into libblitz0.a, libblitz1.a, ... instead,
# because GitHub rejects any file past that and a module consumer cannot run
# anything to put it back together. The pieces are linked as a group; blitz.go
# names them, and this script says so when that needs updating.

set -euo pipefail

REPO_DEFAULT="https://github.com/xo/blitz-c"
REPO="${BLITZ_REPO:-$REPO_DEFAULT}"
REV=""
LOCAL_SRC=""
WORK="${BLITZ_WORK:-$HOME/src/blitz}"
OUT="libblitz"
PROFILE="${BLITZ_PROFILE:-release}"
JOBS_FLAG=""
# --modules-only and --slim-only work on the archives already in libblitz/,
# with no container and no Rust toolchain involved.
MODULES_ONLY=0
SLIM_ONLY=0

# cross reads this; podman unless overridden.
export CROSS_CONTAINER_ENGINE="${CROSS_CONTAINER_ENGINE:-podman}"

selected=()

# Keys are the directory names under libblitz/, referenced by hand in the #cgo
# directives in blitz.go. Renaming one means editing blitz.go too.
declare -A TARGETS=(
    [linux-amd64]=x86_64-unknown-linux-gnu
    [linux-arm64]=aarch64-unknown-linux-gnu
    [linux-armv7]=armv7-unknown-linux-gnueabihf
    [macos-amd64]=x86_64-apple-darwin
    [macos-arm64]=aarch64-apple-darwin
    # -gnu, not -msvc: cgo on Windows links with mingw gcc, and an MSVC-produced
    # .lib does not combine with it cleanly. The gnu target also names its
    # output libblitz.a, so `-lblitz` works unchanged everywhere.
    [windows-amd64]=x86_64-pc-windows-gnu
)

ALL_TARGETS=(linux-amd64 linux-arm64 linux-armv7 macos-amd64 macos-arm64 windows-amd64)

# GOOS,GOARCH for each target key, used to check the #cgo directives in
# blitz.go still name the archives this script produced.
declare -A GO_PLATFORM=(
    [linux-amd64]="linux,amd64"
    [linux-arm64]="linux,arm64"
    [linux-armv7]="linux,arm"
    [macos-amd64]="darwin,amd64"
    [macos-arm64]="darwin,arm64"
    [windows-amd64]="windows,amd64"
)

# //go:build constraint for each target, used in the generated lib.go.
declare -A GO_BUILD_TAG=(
    [linux-amd64]="linux && amd64"
    [linux-arm64]="linux && arm64"
    [linux-armv7]="linux && arm"
    [macos-amd64]="darwin && amd64"
    [macos-arm64]="darwin && arm64"
    [windows-amd64]="windows && amd64"
)

# GOOS_GOARCH for each target, naming the root module's link_*.go files.
declare -A GO_FILE_SUFFIX=(
    [linux-amd64]=linux_amd64
    [linux-arm64]=linux_arm64
    [linux-armv7]=linux_arm
    [macos-amd64]=darwin_amd64
    [macos-arm64]=darwin_arm64
    [windows-amd64]=windows_amd64
)

# System libraries each target needs on top of the blitz archives, emitted into
# the generated lib.go. These are NOT just a copy of native-static-libs.txt:
# rustc's list is incomplete on Windows and over-broad on macOS, so it is a
# starting point for comparison rather than an answer. See the notes below
# before editing any of them.
declare -A SYSTEM_LIBS=(
    [linux-amd64]="-lfontconfig -lfreetype -ldl -lgcc_s -lutil -lrt -lpthread -lm"
    [linux-arm64]="-lfontconfig -lfreetype -ldl -lgcc_s -lutil -lrt -lpthread -lm"
    [linux-armv7]="-lfontconfig -lfreetype -ldl -lgcc_s -lutil -lrt -lpthread -lm"
    # Foundation and -lobjc are not optional: fontique calls
    # NSSearchPathForDirectoriesInDomains to find the font directories.
    # fontconfig, CoreGraphics, SystemConfiguration and libc++ are absent
    # because nothing in the archive refers to them — check with
    # `llvm-nm --undefined-only` before adding any of them back.
    [macos-amd64]="-framework Security -framework CoreFoundation -framework Foundation -framework CoreText -lobjc -liconv -lm"
    [macos-arm64]="-framework Security -framework CoreFoundation -framework Foundation -framework CoreText -lobjc -liconv -lm"
    # Wider than native-static-libs.txt reports, deliberately: rustc omits
    # dwrite and friends, but the archive calls DWriteCreateFactory and ~50
    # other DirectWrite/GDI/OLE entry points.
    [windows-amd64]="-lws2_32 -lbcrypt -lcrypt32 -lsecur32 -lncrypt -lntdll -luserenv -lkernel32 -ldbghelp -lole32 -loleaut32 -ldwrite -lgdi32 -lusp10 -lshell32 -ladvapi32 -luuid -lmsvcrt -lpthread"
)

# Each target ships as its own Go module nested under libblitz/, so a consumer
# downloads only the archives for the platform it is building for. The root
# module's 648 MiB tree is past the 500 MiB cap the go command puts on a module
# (golang.org/x/mod/zip.MaxZipFile, applied to the extracted content as well as
# the zip), and even under it, one module means every consumer pays for all six
# targets. Directories holding a go.mod are excluded from the parent module's
# zip, so this also keeps the root module small.
MODULE_PREFIX="github.com/xo/blitz/libblitz"
# The `go` directive for the generated modules. Deliberately lower than the root
# module's: these packages are a cgo directive and nothing else, so there is no
# reason to make them demand a recent toolchain.
MODULE_GO_DIRECTIVE="1.21"

# GitHub refuses any file over 100 MiB, and libblitz.a for the 64-bit Linux
# targets is past that, so an archive that size has to be committed in pieces.
MAX_ARCHIVE_BYTES=$((100 * 1024 * 1024))
# Parts are packed to this, not to the cap: it leaves room for the archive's own
# overhead and for the library to grow before a part crosses the limit again.
PART_BUDGET_BYTES=$((80 * 1024 * 1024))

die()  { printf 'error: %s\n' "$*" >&2; exit 1; }
info() { printf '\033[1m==>\033[0m %s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }

usage() {
    awk 'NR>2 && /^#/ { sub(/^# ?/, ""); print; next } NR>2 { exit }' "${BASH_SOURCE[0]}"
    exit "${1:-0}"
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --rev|-r)  REV="${2:?--rev needs a value}"; shift 2 ;;
        --repo)    REPO="${2:?--repo needs a value}"; shift 2 ;;
        --local)   LOCAL_SRC="${2:?--local needs a path}"; shift 2 ;;
        --profile) PROFILE="${2:?--profile needs a value}"; shift 2 ;;
        --modules-only) MODULES_ONLY=1; shift ;;
        --slim-only)    SLIM_ONLY=1; shift ;;
        --jobs|-j) JOBS_FLAG="--jobs ${2:?--jobs needs a value}"; shift 2 ;;
        --engine)  export CROSS_CONTAINER_ENGINE="${2:?--engine needs a value}"; shift 2 ;;
        -h|--help) usage 0 ;;
        -*)        die "unknown option: $1" ;;
        *)         selected+=("$1"); shift ;;
    esac
done

if [[ ${#selected[@]} -eq 0 ]]; then
    selected=("${ALL_TARGETS[@]}")
fi

for t in "${selected[@]}"; do
    [[ -n "${TARGETS[$t]:-}" ]] || die "unknown target '$t' (valid: ${ALL_TARGETS[*]})"
done

command -v git >/dev/null 2>&1 || die "git is required"
if [[ "$MODULES_ONLY" -eq 0 && "$SLIM_ONLY" -eq 0 ]]; then
    command -v cross >/dev/null 2>&1 || die "cross is required (cargo install cross)"
fi

# --- source ------------------------------------------------------------------

fetch_source() {
    if [[ -n "$LOCAL_SRC" ]]; then
        SRC="$(cd "$LOCAL_SRC" && pwd)"
        [[ -f "$SRC/Cargo.toml" ]] || die "no Cargo.toml in $SRC"
        info "using local source $SRC"
        printf 'local:%s\n' "$SRC" > version.txt
        return
    fi

    mkdir -p "$(dirname "$WORK")"
    SRC="$WORK/blitz-c"

    if [[ -d "$SRC/.git" ]]; then
        info "updating $REPO"
        git -C "$SRC" fetch --quiet --tags origin
    else
        info "cloning $REPO"
        git clone --quiet "$REPO" "$SRC"
    fi

    local ref
    if [[ -n "$REV" ]]; then
        ref="$REV"
    else
        # Prefer the latest tag; blitz-c may not be tagged yet, in which case
        # track the default branch.
        ref="$(git -C "$SRC" describe --abbrev=0 --tags 2>/dev/null || true)"
        if [[ -z "$ref" ]]; then
            ref="$(git -C "$SRC" symbolic-ref --quiet --short refs/remotes/origin/HEAD 2>/dev/null || echo origin/main)"
            warn "no tags in $REPO, using $ref"
        fi
    fi

    git -C "$SRC" reset --hard --quiet
    git -C "$SRC" clean -qfxd
    git -C "$SRC" checkout --quiet --detach "$ref"

    local sha
    sha="$(git -C "$SRC" rev-parse --short HEAD)"
    info "source at $ref ($sha)"

    # Committed alongside the archives: the only record of what they were built
    # from, since the archives themselves carry no version.
    printf '%s\n%s\n' "$ref" "$sha" > version.txt
}

# --- splitting ---------------------------------------------------------------

file_size() { wc -c < "$1" | tr -d '[:space:]'; }

# Drops what the consumer's link will never use from $1/libblitz.a, in place.
#
# Two things ride along in a rustc staticlib and neither survives contact with
# cgo's linker:
#
#   .llvmbc / .llvmcmd   LLVM bitcode. Cargo already builds this crate's own
#                        deps with -Cembed-bitcode=no, but the std, core and
#                        alloc objects come out of rustup's precompiled rlibs,
#                        which carry it. Nothing here is ever LTO'd by the Go
#                        build, so it is pure freight.
#   debug info           std ships with it and the release profile's
#                        `strip = "none"` keeps it. A consumer debugging their
#                        own Go code has no use for Rust std line tables.
#
# Worth ~13% on linux-amd64 (129.9 -> 112.2 MiB), verified to still link and
# pass the test suite. Note this is NOT the same as switching to the
# `production` profile in blitz-c: `lto = true` there forces embed-bitcode back
# on, and because rustc defers LTO for a staticlib to whoever links it, the
# archive comes out *larger* (195.7 MiB).
#
# Symbols are left alone — --strip-debug, never --strip-unneeded. The archive
# is nothing but symbols as far as the linker is concerned.
slim_archive() {
    local archive="$1" before after members bad attempts

    command -v llvm-objcopy >/dev/null 2>&1 || {
        warn "llvm-objcopy not found; shipping $archive unslimmed"
        return 0
    }

    before="$(file_size "$archive")"
    members="$(llvm-ar t "$archive" | wc -l)"

    # llvm-objcopy rewrites an archive in place, member by member. Doing it this
    # way rather than extract / process / repack matters for two reasons:
    #
    #   - The Windows archive has ~40 duplicate member names (mingw import
    #     stubs, several .o with the same basename). Extraction flattens
    #     everything into one directory, so duplicates would overwrite each
    #     other and silently drop objects.
    #   - Member order and the symbol table are preserved for free.
    #
    # The only thing it refuses is a member that is not an object file at all.
    # Shipped libblitz0.a archives have exactly one, the stray `.parts` scratch
    # file from the split bug; drop whatever it names and retry rather than
    # giving up on the whole archive.
    attempts=0
    while (( attempts < 8 )); do
        # `|| true` because a refused member is the expected path here, and
        # pipefail would otherwise make set -e kill the script mid-slim.
        bad="$(llvm-objcopy --remove-section=.llvmbc --remove-section=.llvmcmd \
                   --strip-debug "$archive" 2>&1 \
               | sed -n "s/.*'[^']*(\(.*\))': The file was not recognized.*/\1/p" \
               | head -1 || true)"

        if [[ -z "$bad" ]]; then
            break
        fi

        warn "$(basename "$archive"): member '$bad' is not an object file, dropping it"
        llvm-ar d "$archive" "$bad"
        attempts=$(( attempts + 1 ))
    done

    if (( attempts >= 8 )); then
        warn "$(basename "$archive"): gave up slimming after $attempts bad members"
        return 0
    fi

    after="$(file_size "$archive")"
    info "slimmed $(basename "$archive"): $((before / 1024 / 1024)) -> $((after / 1024 / 1024)) MiB, $members -> $(llvm-ar t "$archive" | wc -l) members"
}


# Splits $1/libblitz.a into $1/libblitz0.a, libblitz1.a, ... and removes the
# original. Sets SPLIT_PARTS to the number of pieces.
#
# The pieces are linked as a group rather than concatenated back together: a
# consumer never runs anything from this repository, so reassembly is not an
# option. See the #cgo directives in blitz.go.
SPLIT_PARTS=0
split_archive() {
    local dest="$1" archive work parts i
    dest="$(cd "$dest" && pwd)"
    archive="$dest/libblitz.a"

    # llvm-ar, not ar: it reads and writes ELF, Mach-O and COFF archives alike,
    # so one code path covers all six targets from this one Linux host. GNU ar
    # cannot produce a symbol table the Apple linker will accept.
    command -v llvm-ar >/dev/null 2>&1 \
        || die "llvm-ar is required to split archives over $((MAX_ARCHIVE_BYTES / 1024 / 1024)) MiB (install llvm)"

    # Extraction flattens every member into one directory, so two members
    # sharing a name would overwrite each other and silently drop objects.
    if [[ -n "$(llvm-ar t "$archive" | sort | uniq -d)" ]]; then
        die "$archive has duplicate member names; cannot split it by extraction"
    fi

    work="$(mktemp -d)"
    ( cd "$work" && llvm-ar x "$archive" )

    # The plan lives OUTSIDE $work. Writing it inside raced with the `find`
    # below: a pipeline starts every stage at once, so the shell created the
    # file for awk's redirection while find was still walking the directory,
    # and find picked it up as a zero-byte member. Every shipped libblitz0.a
    # has a bogus `.parts` member because of it, which breaks llvm-strip and
    # llvm-objcopy on the whole archive.
    plan="$(mktemp)"

    # Pack members into the fewest parts the budget allows, then even them out,
    # so no single part sits just under the cap while another is nearly empty.
    find "$work" -maxdepth 1 -type f -printf '%s %f\n' \
      | awk -v budget="$PART_BUDGET_BYTES" '
            { size[NR] = $1; name[NR] = $2; total += $1 }
            END {
                parts = int(total / budget); if (total % budget) parts++
                if (parts < 1) parts = 1
                target = total / parts
                part = 0; acc = 0
                for (i = 1; i <= NR; i++) {
                    if (acc > 0 && acc + size[i] > target && part < parts - 1) {
                        part++; acc = 0
                    }
                    acc += size[i]
                    print part, name[i]
                }
            }' > "$plan"

    parts="$(awk '{ if ($1 + 1 > m) m = $1 + 1 } END { print m + 0 }' "$plan")"
    [[ "$parts" -ge 1 ]] || die "$archive: could not work out how to split it"

    for (( i = 0; i < parts; i++ )); do
        rm -f "$dest/libblitz$i.a"
        # Through xargs because the member list runs to thousands of objects;
        # `q` appends, so the repeated invocations it may make are fine, and `s`
        # rewrites the symbol table each time.
        awk -v p="$i" '$1 == p { print $2 }' "$plan" \
          | ( cd "$work" && xargs llvm-ar qcs "$dest/libblitz$i.a" )
    done

    rm -f "$archive"
    rm -rf "$work" "$plan"

    local part size
    for (( i = 0; i < parts; i++ )); do
        part="$dest/libblitz$i.a"
        size="$(file_size "$part")"
        if [[ "$size" -ge "$MAX_ARCHIVE_BYTES" ]]; then
            die "libblitz$i.a is $((size / 1024 / 1024)) MiB, still over the limit — lower PART_BUDGET_BYTES"
        fi
        chmod -x "$part"
    done

    SPLIT_PARTS="$parts"
}

# The `-l` flags blitz.go needs for a target, given how many parts it has.
archive_ldflags() {
    local target="$1" parts="$2" names="" i

    if [[ "$parts" -eq 0 ]]; then
        printf -- '-lblitz'
        return
    fi

    for (( i = 0; i < parts; i++ )); do
        names+=" -lblitz$i"
    done
    names="${names# }"

    # --start-group is GNU ld syntax, and the pieces reference each other's
    # symbols in both directions, so the linker has to be allowed to revisit
    # them. Apple's ld64 has no such flag and needs none — it resolves the
    # whole set of archives together.
    case "$target" in
        macos-*) printf '%s' "$names" ;;
        *)       printf -- '-Wl,--start-group %s -Wl,--end-group' "$names" ;;
    esac
}

# Writes the Go module that ships this target's archives: go.mod, an untagged
# doc.go so the package always builds, and lib.go carrying the #cgo LDFLAGS.
#
# Generated rather than hand-written, because the flags depend on how many
# pieces the archive was split into. blitz.go used to name the archives by hand
# and this script could only warn when the two disagreed; now there is one
# source of truth and nothing to keep in sync.
write_module() {
    # Split across statements on purpose: `local` expands all of its arguments
    # before assigning any of them, so referring to $target in the same `local`
    # that declares it trips set -u.
    local target="$1" parts="$2"
    local dest="$OUT/$target"
    local tag="${GO_BUILD_TAG[$target]}" libs="${SYSTEM_LIBS[$target]}"
    local ldflags

    ldflags="$(archive_ldflags "$target" "$parts") $libs"

    cat > "$dest/go.mod" <<EOF
module $MODULE_PREFIX/$target

go $MODULE_GO_DIRECTIVE
EOF

    cat > "$dest/doc.go" <<EOF
// Package lib ships the prebuilt Blitz static archives for $target and the
// cgo link flags that go with them. It has no API: import it for its side
// effect on the link, which is what github.com/xo/blitz does.
//
// Code generated by build-blitz.sh. DO NOT EDIT.
package lib
EOF

    # The build constraint keeps the #cgo line out of every other platform's
    # build. doc.go above carries no constraint, so the package still resolves
    # when something imports it on a platform it does not cover, instead of
    # failing with "build constraints exclude all Go files".
    cat > "$dest/lib.go" <<EOF
// Code generated by build-blitz.sh. DO NOT EDIT.

//go:build $tag

package lib

/*
#cgo LDFLAGS: -L\${SRCDIR} $ldflags
*/
import "C"
EOF

    info "$target: wrote go.mod, doc.go, lib.go"
}

# Writes the root module's link_GOOS_GOARCH.go files: one build-tagged blank
# import per target. Always writes all of them, not just the selected targets —
# the root package has to compile everywhere, whichever archives were rebuilt.
write_link_files() {
    local target file tag

    for target in "${ALL_TARGETS[@]}"; do
        file="link_${GO_FILE_SUFFIX[$target]}.go"
        tag="${GO_BUILD_TAG[$target]}"

        cat > "$file" <<EOF
// Code generated by build-blitz.sh. DO NOT EDIT.

//go:build $tag

package blitz

// Blank import for the link only: the package has no API, it just carries the
// $target archives and the #cgo LDFLAGS naming them. Because this file is
// build-tagged, the go command never downloads the other five platforms'
// modules when building for this one.
import _ "$MODULE_PREFIX/$target"
EOF
    done

    info "wrote link_*.go for ${#ALL_TARGETS[@]} targets"
}

# --- build -------------------------------------------------------------------

build_target() {
    local target="$1" triple dest archive profile_flag profile_dir rustflags_var
    triple="${TARGETS[$target]}"

    info "building $target ($triple)"

    case "$PROFILE" in
        release) profile_flag="--release";          profile_dir="release" ;;
        debug)   profile_flag="";                   profile_dir="debug" ;;
        *)       profile_flag="--profile $PROFILE"; profile_dir="$PROFILE" ;;
    esac

    dest="$OUT/$target"
    mkdir -p "$dest"

    # --print=native-static-libs rides along with the build that produces the
    # archive, rather than a second invocation that could disagree with it.
    rustflags_var="CARGO_TARGET_$(tr '[:lower:]-' '[:upper:]_' <<< "$triple")_RUSTFLAGS"

    (
        cd "$SRC"
        export "$rustflags_var=--print=native-static-libs"
        set -x
        cross build --verbose $profile_flag --target "$triple" $JOBS_FLAG
    ) 2>&1 | tee "$dest/build.log"

    archive="$SRC/target/$triple/$profile_dir/libblitz.a"
    [[ -f "$archive" ]] || die "$target: expected $archive to exist"

    cp "$archive" "$dest/libblitz.a"
    # cargo marks the archive executable and it propagates into git.
    chmod -x "$dest/libblitz.a"

    # Before the size check below, so a target only gets split if it is still
    # over the limit once the dead weight is gone.
    slim_archive "$dest/libblitz.a"

    # Anything over the limit has to be committed as several archives, since
    # GitHub will reject the push otherwise.
    rm -f "$dest"/libblitz[0-9].a
    SPLIT_PARTS=0
    if [[ "$(file_size "$dest/libblitz.a")" -ge "$MAX_ARCHIVE_BYTES" ]]; then
        info "$target: over $((MAX_ARCHIVE_BYTES / 1024 / 1024)) MiB, splitting"
        split_archive "$dest"
    fi

    # Recorded so a later undefined-symbol failure can be diffed against the
    # LDFLAGS for this platform in blitz.go.
    grep 'native-static-libs:' "$dest/build.log" \
        | tail -1 \
        | sed -e 's/.*native-static-libs: *//' \
        > "$dest/native-static-libs.txt" || true

    local produced
    produced="$(target_archives "$target")"
    info "$target: $produced"
    if [[ -s "$dest/native-static-libs.txt" ]]; then
        printf '    links: %s\n' "$(cat "$dest/native-static-libs.txt")"
    else
        warn "$target: could not capture native-static-libs"
    fi

    write_module "$target" "$SPLIT_PARTS"
}

# "libblitz.a (130M)" or "libblitz0.a (65M) libblitz1.a (65M)".
target_archives() {
    local dest="$OUT/$1" out="" f
    for f in "$dest"/libblitz.a "$dest"/libblitz[0-9].a; do
        [[ -f "$f" ]] || continue
        out+=" $(basename "$f") ($(du -h "$f" | cut -f1))"
    done
    printf '%s' "${out# }"
}

# --- main --------------------------------------------------------------------

# Both of these work on what is already committed, so neither needs a source
# checkout or a container.
if [[ "$MODULES_ONLY" -eq 1 || "$SLIM_ONLY" -eq 1 ]]; then
    for t in "${selected[@]}"; do
        dest="$OUT/$t"
        [[ -d "$dest" ]] || die "$dest does not exist; build it first"

        if [[ "$SLIM_ONLY" -eq 1 ]]; then
            for a in "$dest"/libblitz.a "$dest"/libblitz[0-9].a; do
                if [[ -f "$a" ]]; then
                    slim_archive "$a"
                fi
            done
        fi

        if [[ "$MODULES_ONLY" -eq 1 ]]; then
            # Derive the piece count from what is on disk rather than rebuilding
            # to find out: 0 means a single libblitz.a.
            # Plain assignment, not (( parts++ )) — post-increment evaluates to
            # the old value, so the first one would return 1 and trip set -e.
            parts=0
            for a in "$dest"/libblitz[0-9].a; do
                if [[ -f "$a" ]]; then
                    parts=$(( parts + 1 ))
                fi
            done
            write_module "$t" "$parts"
        fi
    done
    if [[ "$MODULES_ONLY" -eq 1 ]]; then
        write_link_files
    fi
    info "done"
    exit 0
fi

info "container engine: $CROSS_CONTAINER_ENGINE"

fetch_source

# cross looks for Cross.toml next to the manifest it is building, so copy this
# repo's copy into the checkout.
if [[ -z "$LOCAL_SRC" && -f Cross.toml ]]; then
    cp Cross.toml "$SRC/Cross.toml"
fi

mkdir -p "$OUT"
cp "$SRC/include/blitz.h" "$OUT/blitz.h"
info "header -> $OUT/blitz.h"

for t in "${selected[@]}"; do
    build_target "$t"
done

write_link_files

info "done"
printf '\nbuilt archives:\n'
for t in "${selected[@]}"; do
    printf '  %-16s %s\n' "$t" "$(target_archives "$t")"
done
printf '\nfrom blitz-c %s\n' "$(tr '\n' ' ' < version.txt)"
printf 'next: go test -race ./...\n'
