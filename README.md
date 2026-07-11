# golang-jsonrpc2

[![Go Reference](https://pkg.go.dev/badge/github.com/gumeniukcom/golang-jsonrpc2/v2.svg)](https://pkg.go.dev/github.com/gumeniukcom/golang-jsonrpc2/v2)
[![Go Report Card](https://goreportcard.com/badge/github.com/gumeniukcom/golang-jsonrpc2/v2)](https://goreportcard.com/report/github.com/gumeniukcom/golang-jsonrpc2/v2)
[![CI](https://github.com/gumeniukcom/golang-jsonrpc2/actions/workflows/ci.yml/badge.svg)](https://github.com/gumeniukcom/golang-jsonrpc2/actions/workflows/ci.yml)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/13570/badge)](https://www.bestpractices.dev/projects/13570)
[![Release](https://img.shields.io/github/v/release/gumeniukcom/golang-jsonrpc2)](https://github.com/gumeniukcom/golang-jsonrpc2/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Spec-conformant JSON-RPC 2.0 for Go: one dispatcher core serving five
transports out of the box — HTTP, Fiber, WebSocket, and stdio with both LSP
`Content-Length` and MCP newline framing — with typed handlers via
generics, OpenRPC self-description, and DoS-aware defaults.

## Quick start

```go
package main

import (
	"context"
	"net/http"
	"time"

	jrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpchttp"
)

type sumParams struct{ A, B int }
type sumResult struct{ Sum int }

func main() {
	serv := jrpc.New()
	_ = jrpc.RegisterTyped(serv, "sum", func(ctx context.Context, p sumParams) (sumResult, error) {
		return sumResult{Sum: p.A + p.B}, nil
	})

	srv := &http.Server{
		Addr:              ":8088",
		Handler:           jsonrpchttp.Handler(serv),
		ReadHeaderTimeout: 5 * time.Second, // see docs/production.md for the full checklist
	}
	panic(srv.ListenAndServe())
}
```

```bash
go get github.com/gumeniukcom/golang-jsonrpc2/v2
curl -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"sum","params":{"a":5,"b":3}}' localhost:8088
# {"jsonrpc":"2.0","result":{"Sum":8},"id":1}
```

An MCP server over stdio is the same dispatcher on a different wire —
`jsonrpcstdio.Serve(ctx, serv, jsonrpcstdio.FramingNDJSON, os.Stdin, os.Stdout)`
is the whole transport. Runnable example: [example/mcp-stdio](example/mcp-stdio/main.go),
walkthrough: [docs/mcp-lsp.md](docs/mcp-lsp.md).

## Why this library

- **Fast where it counts.** A dispatched call costs ~1.1 µs — below a
  hand-rolled `encoding/json` dispatcher — and a full end-to-end call runs
  ~2× faster than the two widely-used alternatives with ~3.4× fewer
  allocations, matching stdlib `net/rpc` on large-payload throughput.
  Measured 2026-07-10; full tables, methodology, and caveats:
  [docs/BENCHMARKS.md](docs/BENCHMARKS.md), reproduce with
  `make bench-compare`.
- **One core, five wires.** Register methods once; serve them over HTTP,
  Fiber v2/v3, WebSocket, and stdio (LSP or MCP framing) simultaneously —
  plus a one-call contract for custom transports.
- **Typed handlers via generics.** `RegisterTyped` gives compile-time
  param/result types with no reflection on the hot path, and the same
  registry renders an [OpenRPC 1.3.2 document](docs/openrpc.md).
- **Safe by default.** Batch cap (100) with a bounded worker pool,
  per-request timeouts, panic recovery, byte-exact spec conformance
  (notifications, batches, id echo, -32700/-32600 classification), and
  error texts never leak to clients unless you opt in.
- **Lean dependency graph.** Three direct deps in the core module; Fiber
  adapters and the benchmark suite live in nested modules that never touch
  your build.

## Transports

| Wire | Package | Push | Notes |
|---|---|---|---|
| HTTP | [`jsonrpchttp`](https://pkg.go.dev/github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpchttp) | — | 204 for notifications, body cap, client included |
| Fiber v2/v3 | [`jsonrpcfiber`](https://pkg.go.dev/github.com/gumeniukcom/golang-jsonrpc2/jsonrpcfiber), [`jsonrpcfiberv3`](https://pkg.go.dev/github.com/gumeniukcom/golang-jsonrpc2/jsonrpcfiberv3) | — | nested modules, core stays Fiber-free |
| WebSocket | [`jsonrpcws`](https://pkg.go.dev/github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpcws) | yes | multiplexed calls, bounded fan-out, push |
| stdio (LSP/MCP) | [`jsonrpcstdio`](https://pkg.go.dev/github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpcstdio) | yes | both framings, sequential by default, client included |
| custom | — | — | `HandleRPCJSONRawMessage(ctx, bytes)` is the whole contract |

Choosing between them, and how the size limits stack:
[docs/transports.md](docs/transports.md).

## How it compares

| | this library | sourcegraph/jsonrpc2 | creachadair/jrpc2 | net/rpc |
|---|---|---|---|---|
| Batches | yes | no | yes | no (1.0) |
| stdio LSP + NDJSON framings | yes | yes | yes | partial |
| HTTP / WebSocket servers | yes / yes | no / DIY | yes / DIY | no |
| Typed handlers | generics | raw handler | reflection | reflection |
| DoS limits by default | yes | no | partial | no |
| OpenRPC generation | yes | no | methods list | no |

The full matrix — including **where the alternatives win** (reverse calls,
dependency minimalism, production mileage) — is in
[docs/COMPARISON.md](docs/COMPARISON.md).

## Documentation

Task-oriented guides in [docs/](docs/README.md):

- [Choosing a transport](docs/transports.md) — and the limits stack
- [Building an MCP or LSP server](docs/mcp-lsp.md) — stdio end to end
- [Typed handlers & errors](docs/typed-handlers.md) — the error-privacy model
- [Calling a server](docs/clients.md) — HTTP/WS/stdio clients, batches
- [Middleware & auth](docs/middleware-auth.md) — chains, per-method policy
- [Server push & subscriptions](docs/push-subscriptions.md) — `Pusher` lifecycle
- [Production hardening](docs/production.md) — every limit in one table
- [Observability](docs/observability.md) — the `SetObserver` hook
- [OpenRPC](docs/openrpc.md) · [Migrating](docs/migrating.md) ·
  [Benchmarks](docs/BENCHMARKS.md) · [Comparison](docs/COMPARISON.md)

API reference: [pkg.go.dev](https://pkg.go.dev/github.com/gumeniukcom/golang-jsonrpc2/v2).

## Versioning

The module path is `github.com/gumeniukcom/golang-jsonrpc2/v2` (SemVer,
[Keep a Changelog](CHANGELOG.md)). Go 1.23+. Migrating from v1 or from
another JSON-RPC library: [docs/migrating.md](docs/migrating.md).

## License

[MIT](LICENSE).
