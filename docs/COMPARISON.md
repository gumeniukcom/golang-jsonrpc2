# How this library compares

> Snapshot as of **2026-07-10** (v2.5.0). Versions compared:
> `sourcegraph/jsonrpc2 v0.2.1`, `creachadair/jrpc2 v1.3.5`, Go 1.25 stdlib
> `net/rpc`. Libraries evolve — **corrections welcome, please open an
> issue.** Cells about other libraries link to their documentation where
> the claim is non-obvious. Performance numbers live in
> [BENCHMARKS.md](BENCHMARKS.md), reproducible via `make bench-compare`.

The matrix covers the libraries our benchmark suite tracks (so every row
has a drift-detection mechanism). Other libraries are discussed in prose
below it.

## Feature matrix

| | this library | [sourcegraph/jsonrpc2](https://github.com/sourcegraph/jsonrpc2) | [creachadair/jrpc2](https://github.com/creachadair/jrpc2) | net/rpc + jsonrpc codec |
|---|---|---|---|---|
| JSON-RPC version | 2.0 | 2.0 | 2.0 | **1.0** |
| Batch requests | yes | [no](https://github.com/sourcegraph/jsonrpc2#known-issues) | yes | no |
| Notifications (spec silence rules) | yes | yes | yes | n/a (1.0) |
| HTTP server | yes (`jsonrpchttp`, Fiber adapters) | no (stream-oriented) | yes (`jhttp`) | no |
| WebSocket | yes (`jsonrpcws`) | via any `io` stream | via channel | no |
| stdio, LSP Content-Length framing | yes (`jsonrpcstdio`) | yes (`VSCodeObjectCodec`) | yes (`channel.Header`) | no |
| stdio, MCP newline framing | yes | yes (`PlainObjectCodec`¹) | yes (`channel.Line`) | yes (line-ish) |
| Client included | yes (HTTP, WS, stdio) | yes | yes | yes |
| Client batch | yes (HTTP, WS) | no | yes | no |
| Server→client notifications (push) | yes | yes | yes | no |
| Server→client *requests* (correlated) | **no** | yes (bidirectional conn) | yes (`Callback`) | no |
| Typed handlers | generics, compile-time | no (raw `Handle`) | reflection (`handler.New`) | reflection |
| Middleware / per-method timeouts | yes / yes | no / no | no (options exist) / no | no / no |
| DoS limits (message size, batch cap, bounded concurrency) | yes, defaults on | no | partial (concurrency) | no |
| Panic recovery in handlers | yes | no | yes | no |
| Observability hook | yes (`SetObserver`) | `OnSend`/`OnRecv` | `RPCLog` interface | no |
| OpenRPC / service description | yes (generated + `rpc.discover`) | no | `rpc.serverInfo` (methods list) | no |
| Spec quirk worth knowing | — | id reuse across batch n/a | strict version checks | 1.0 wire format |
| Core direct dependencies | 3 | 1 (gorilla/websocket) | 2 | 0 (stdlib) |
| Maintenance (snapshot date) | active | maintenance mode² | active | stdlib (frozen) |
| License | MIT | MIT | BSD-3 | BSD-3 |

¹ `PlainObjectCodec` is a plain JSON value stream (no framing bytes at
all), which most NDJSON peers accept on output but is not strictly
line-framed on input.

² The sourcegraph README lists batch support as a known gap and the repo
accepts fixes but adds few features; check its activity for yourself at
the link.

## Where we lose

Honesty first — pick the competitor when these matter to you:

- **Server-initiated requests with response correlation.**
  creachadair's `Callback` and sourcegraph's symmetric connections can
  *call* the client and await an answer (LSP `workspace/configuration`,
  MCP sampling). Our transports only push notifications; reverse calls
  are a documented non-goal of `jsonrpcstdio` v1.
- **Raw TCP / arbitrary stream adapters out of the box.**
  creachadair's `channel` and sourcegraph's `ObjectStream` wrap any
  `io.ReadWriteCloser` today; here you would reuse `jsonrpcstdio.Serve`
  over the conn (works, but stdio-flavored) or write a thin adapter.
- **Dependency minimalism.** sourcegraph/jsonrpc2 carries a single dep
  (gorilla/websocket, used only by its websocket subpackage); our core
  carries three (easyjson, uuid, coder/websocket — and easyjson's
  maintenance has slowed, which is why an `encoding/json/v2` migration
  path is prepared; see `docs/dev/json-v2-plan.md`).
- **Go version floor.** We require Go 1.25+ (generics-era APIs, toolchain
  pinning); the alternatives build on much older toolchains.
- **Production mileage.** sourcegraph/jsonrpc2 has years of use inside
  editors and language servers; this library's v2 API is younger.

## Other libraries, briefly

- **[ybbus/jsonrpc](https://github.com/ybbus/jsonrpc)** — an HTTP *client*
  only (no server). If you only consume JSON-RPC over HTTP, it is a fine
  minimal choice; there is nothing to benchmark server-side.
- **[osamingo/jsonrpc](https://github.com/osamingo/jsonrpc)** — an
  `net/http`-handler server with method takeovers; HTTP-only, no streams,
  no batches on the client side.
- **[filecoin-project/go-jsonrpc](https://github.com/filecoin-project/go-jsonrpc)** —
  reflection-based, WebSocket-first, with reverse calls and channels;
  built for the Filecoin/Lotus ecosystem. Powerful, but its API binds
  whole Go structs rather than individual methods, and there is no
  message-level entry point to compare fairly.
- **[semrush/zenrpc](https://github.com/semrush/zenrpc)** — `go generate`
  based with SMD output; low activity in recent years.
- **stdlib `net/rpc`** — in the matrix above because it is the floor
  everyone knows, but remember it speaks JSON-RPC **1.0** and is frozen.

## Refresh policy

This page is reviewed at every minor release of this library (it is part
of the release checklist). The benchmark-tracked columns also get drift
signals from Dependabot bumps of `benchmarks/go.mod`. If you spot a stale
cell sooner — issues and PRs welcome.
