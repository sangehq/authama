module github.com/sangehq/authama/src

go 1.27.0

toolchain go1.27.4

// The Go module proxy cached v0.1.6 from the history that was reset to v0.1.0;
// what it serves is not this code, and its go.mod names the module
// github.com/simtabi/authama/src. Binaries ship on the GitHub release only; this
// keeps the toolchain from choosing it. v0.1.0 is the current release, so it is
// not retracted (docs/release.md).
retract v0.1.6 // cached by the proxy from the pre-reset history

require (
	github.com/fsnotify/fsnotify v1.9.0
	github.com/modelcontextprotocol/go-sdk v1.8.0
	github.com/simtabi/go-notifykit/src v0.1.1
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.1.0
	go.yaml.in/yaml/v3 v3.0.6
	golang.org/x/crypto v0.59.0
	golang.org/x/sys v0.50.0
	modernc.org/sqlite v1.62.0
)

require (
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	modernc.org/libc v1.80.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)
