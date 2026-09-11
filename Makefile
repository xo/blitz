# Convenience wrapper. The real work is in build-blitz.sh and `go test`.
#
#   make libs        build every target (cross + podman)
#   make libs TARGET=linux-amd64
#   make test        go test -race
#   make test-net    include the network tests
#   make render      build cmd/blitz-render
#   make clean       remove built artifacts

GO      ?= go
TARGET  ?=
INPUT   ?= testdata/sample.md

.PHONY: all libs test test-net test-short bench render lint tidy clean distclean

all: test

# Every target goes through cross; there is no native path. Pass TARGET to
# narrow it: make libs TARGET="linux-amd64 linux-arm64"
libs:
	./build-blitz.sh $(TARGET)

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

tidy:
	$(GO) mod tidy

clean:
	rm -rf bin testdata/output

# Removes committed artifacts — only useful before a full rebuild.
distclean: clean
	rm -rf libblitz/*/ libblitz/blitz.h version.txt
