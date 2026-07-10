// Internal benchmarking tooling for golang-jsonrpc2: compares this library
// against other Go JSON-RPC implementations. Not tagged, not supported, may
// break or vanish at any time — all code lives in _test.go files and an
// internal/ package, so nothing here is importable.
module github.com/gumeniukcom/golang-jsonrpc2/benchmarks

go 1.25.0

// In-repo builds run against the working tree; consumers of this repo never
// build this module.
replace github.com/gumeniukcom/golang-jsonrpc2/v2 => ../

require (
	github.com/creachadair/jrpc2 v1.3.5
	github.com/gumeniukcom/golang-jsonrpc2/v2 v2.5.0
	github.com/sourcegraph/jsonrpc2 v0.2.1
)

require (
	github.com/creachadair/mds v0.26.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/mailru/easyjson v0.9.1 // indirect
	golang.org/x/sync v0.19.0 // indirect
)
