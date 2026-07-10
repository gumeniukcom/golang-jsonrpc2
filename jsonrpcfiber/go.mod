module github.com/gumeniukcom/golang-jsonrpc2/jsonrpcfiber

go 1.25

toolchain go1.25.12

require (
	github.com/gofiber/fiber/v2 v2.52.12
	github.com/gumeniukcom/golang-jsonrpc2/v2 v2.3.0
)

require (
	github.com/andybalholm/brotli v1.1.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/klauspost/compress v1.17.9 // indirect
	github.com/mailru/easyjson v0.9.1 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.51.0 // indirect
	github.com/valyala/tcplisten v1.0.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
)

// The core module is developed in the parent directory and released
// together with this adapter. A replace in a non-main module is ignored by
// consumers (they use the required version), so this only affects in-repo
// builds and tests.
replace github.com/gumeniukcom/golang-jsonrpc2/v2 => ../
