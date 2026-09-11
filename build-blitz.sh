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

command -v git   >/dev/null 2>&1 || die "git is required"
command -v cross >/dev/null 2>&1 || die "cross is required (cargo install cross)"

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
            }' > "$work/.parts"

    parts="$(awk '{ if ($1 + 1 > m) m = $1 + 1 } END { print m + 0 }' "$work/.parts")"
    [[ "$parts" -ge 1 ]] || die "$archive: could not work out how to split it"

    for (( i = 0; i < parts; i++ )); do
        rm -f "$dest/libblitz$i.a"
        # Through xargs because the member list runs to thousands of objects;
        # `q` appends, so the repeated invocations it may make are fine, and `s`
        # rewrites the symbol table each time.
        awk -v p="$i" '$1 == p { print $2 }' "$work/.parts" \
          | ( cd "$work" && xargs llvm-ar qcs "$dest/libblitz$i.a" )
    done

    rm -f "$archive"
    rm -rf "$work"

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

# blitz.go names the archives by hand, so a change in the number of parts has to
# be reflected there or consumers get an undefined-symbol wall. Check rather
# than trusting it.
#
# Only the archive flags are checked, not the system libraries. Those need
# judgement that this script has no business making: rustc's list is incomplete
# on Windows (it omits dwrite, which the archive definitely calls) and carries
# entries macOS does not need, so it is a starting point rather than an answer.
# native-static-libs.txt records it for exactly that comparison.
check_cgo_directive() {
    local target="$1" parts="$2" platform expected line
    platform="${GO_PLATFORM[$target]}"
    expected="$(archive_ldflags "$target" "$parts")"

    line="$(grep -E "^#cgo +${platform}[[:space:]]+LDFLAGS:" blitz.go 2>/dev/null || true)"

    if [[ -n "$line" && "$line" == *"$expected"* ]]; then
        return
    fi

    warn "$target: blitz.go does not name the archives just built. It needs:"
    printf '    #cgo %s LDFLAGS: -L${SRCDIR}/%s/%s %s <system libs>\n' \
        "$platform" "$OUT" "$target" "$expected" >&2
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

    check_cgo_directive "$target" "$SPLIT_PARTS"
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

info "done"
printf '\nbuilt archives:\n'
for t in "${selected[@]}"; do
    printf '  %-16s %s\n' "$t" "$(target_archives "$t")"
done
printf '\nfrom blitz-c %s\n' "$(tr '\n' ' ' < version.txt)"
printf 'next: go test -race ./...\n'
