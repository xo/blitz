module github.com/xo/blitz

go 1.27

// One module per platform, each holding just that target's static archives and
// the #cgo LDFLAGS naming them. They are required here so `go mod tidy` keeps
// them pinned for every platform, but only the one matching GOOS/GOARCH is ever
// downloaded: the imports live in build-tagged link_*.go files, and the go
// command fetches a module's zip only when a package it provides is actually in
// the build. A linux/amd64 build pulls ~33 MiB, not the ~190 MiB all six come
// to.
//
// Splitting them out is not optional. The archives total ~648 MiB, and the go
// command refuses a module whose zip *or extracted content* is over 500 MiB
// (golang.org/x/mod/zip.MaxZipFile). Directories holding a go.mod are excluded
// from the parent module's zip, so this also keeps the root module at ~0.3 MiB
// rather than carrying the archives twice.
require (
	github.com/xo/blitz/libblitz/linux-amd64 v0.1.0
	github.com/xo/blitz/libblitz/linux-arm64 v0.1.0
	github.com/xo/blitz/libblitz/linux-armv7 v0.1.0
	github.com/xo/blitz/libblitz/macos-amd64 v0.1.0
	github.com/xo/blitz/libblitz/macos-arm64 v0.1.0
	github.com/xo/blitz/libblitz/windows-amd64 v0.1.0
)

// Local development only, and inert for anyone who depends on this module: the
// go command applies replace directives from the main module's go.mod and
// ignores them everywhere else. Verified — a consumer building against a
// published v0.x.y resolves the versions in the require block above and never
// looks at these paths.
//
// They are what makes `go build`, `go test` and `go mod tidy` work in this
// repository before the libblitz/<target>/vX.Y.Z tags exist, and what makes
// them use the archives in the working tree afterwards, rather than whatever
// was last published. go.work would do the same job, but replace also keeps
// `go mod tidy` working, which go.work does not — it resolves the require block
// against real tags regardless of the workspace.
//
// See "Releasing" in libblitz/README.md: the require block above is maintained
// by hand, and CI cannot check it, because these lines override it here.
replace (
	github.com/xo/blitz/libblitz/linux-amd64 => ./libblitz/linux-amd64
	github.com/xo/blitz/libblitz/linux-arm64 => ./libblitz/linux-arm64
	github.com/xo/blitz/libblitz/linux-armv7 => ./libblitz/linux-armv7
	github.com/xo/blitz/libblitz/macos-amd64 => ./libblitz/macos-amd64
	github.com/xo/blitz/libblitz/macos-arm64 => ./libblitz/macos-arm64
	github.com/xo/blitz/libblitz/windows-amd64 => ./libblitz/windows-amd64
)
