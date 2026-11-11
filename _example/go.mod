module github.com/xo/blitz/_example

go 1.27.1

require (
	github.com/xo/blitz v0.5.0
	github.com/xo/transit v0.7.8
	github.com/xo/transit/grammars/bash v0.7.8
	github.com/xo/transit/grammars/go v0.7.8
	github.com/xo/transit/grammars/json v0.7.8
	github.com/xo/transit/grammars/python v0.7.8
	github.com/xo/transit/styles v0.7.8
)

require (
	github.com/xo/blitz/libblitz/freebsd-amd64 v0.1.8 // indirect
	github.com/xo/blitz/libblitz/linux-amd64 v0.1.8 // indirect
	github.com/xo/blitz/libblitz/linux-amd64-musl v0.1.8 // indirect
	github.com/xo/blitz/libblitz/linux-arm64 v0.1.8 // indirect
	github.com/xo/blitz/libblitz/linux-arm64-musl v0.1.8 // indirect
	github.com/xo/blitz/libblitz/linux-armv7 v0.1.8 // indirect
	github.com/xo/blitz/libblitz/macos-amd64 v0.1.8 // indirect
	github.com/xo/blitz/libblitz/macos-arm64 v0.1.8 // indirect
	github.com/xo/blitz/libblitz/windows-amd64 v0.1.8 // indirect
	github.com/xo/blitz/libblitz/windows-arm64 v0.1.8 // indirect
)

// The program builds against this working tree. Remove the replace, and run
// `go mod tidy`, to build it against the release of blitz that go.mod names.
replace github.com/xo/blitz => ../
