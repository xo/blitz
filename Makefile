# Convenience wrapper. The real work is in build-blitz.sh and `go test`.
#
#   make libs        build every target (cross + podman)
#   make libs TARGET=linux-amd64
#   make modules     regenerate libblitz/*/{go.mod,lib.go} and link_*.go
#   make slim        strip dead weight from the committed archives
#   make check-generated  fail if the generated Go files are stale
#   make test        go test -race
#   make test-net    include the network tests
#   make render      build cmd/blitz-render
#   make clean       remove built artifacts

GO      ?= go
TARGET  ?=
INPUT   ?= testdata/sample.md

.PHONY: all libs modules slim test test-net test-short bench render lint tidy \
	check-generated clean distclean

all: test

# Every target goes through cross; there is no native path. Pass TARGET to
# narrow it: make libs TARGET="linux-amd64 linux-arm64"
libs:
	./build-blitz.sh $(TARGET)

# Neither of these needs a container or a Rust toolchain; they work on what is
# already committed. `modules` is the one to run after editing SYSTEM_LIBS.
modules:
	./build-blitz.sh --modules-only $(TARGET)

slim:
	./build-blitz.sh --slim-only $(TARGET)

# No dependency on `libs`: the archives are committed, so the normal case is
# that they're already there. Run `make libs` explicitly after changing blitz-c.
#
# -race is the point of most of the concurrency tests; without it they are
# nearly meaningless.
test:
	$(GO) test -race -v ./...

test-net:
	$(GO) test -race -v -network ./...

test-short:
	$(GO) test -short ./...

bench:
	$(GO) test -run '^$$' -bench . -benchtime 10x ./...

render:
	$(GO) build -o bin/blitz-render ./cmd/blitz-render
	@echo "try: ./bin/blitz-render $(INPUT)"

lint:
	$(GO) vet ./...
	gofmt -l -d .

# The root module and each platform module. This only works because of the
# replace block in go.mod: without it, `go mod tidy` would resolve the require
# block against libblitz/<target>/vX.Y.Z tags that may not be pushed yet.
# The require *versions* are still a release decision, maintained by hand —
# see the "Releasing" section in libblitz/README.md.
tidy:
	$(GO) mod tidy
	@for d in libblitz/*/; do \
		[ -f "$$d/go.mod" ] || continue; \
		( cd "$$d" && $(GO) mod tidy ) || exit 1; \
	done

# Fails if the generated files are stale. Cheap enough for CI.
check-generated:
	./build-blitz.sh --modules-only >/dev/null
	@git diff --exit-code -- 'libblitz/*/go.mod' 'libblitz/*/lib.go' \
		'libblitz/*/doc.go' 'link_*.go' \
		|| { echo "generated files are stale; run: make modules"; exit 1; }

clean:
	rm -rf bin testdata/output

# Removes committed artifacts — only useful before a full rebuild. Leaves
# link_*.go alone so the root package still compiles; `make libs` rewrites them.
distclean: clean
	rm -rf libblitz/*/ libblitz/blitz.h version.txt
