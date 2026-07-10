# golang-jsonrpc2/v2

Implementation for JSON-RPC 2.0 protocol in Go.

Full specification: https://www.jsonrpc.org/specification

## Install

```bash
go get github.com/gumeniukcom/golang-jsonrpc2/v2
```

## HTTP example

The `jsonrpchttp` subpackage turns the dispatcher into an `http.Handler`: it
bounds the request body (1 MiB by default, `WithMaxBodySize` to tune),
answers notifications with `204 No Content`, checks `Content-Type`, and maps
transport-level failures to HTTP codes (405/415/413/400) while JSON-RPC
errors stay HTTP 200 with an error body.

```go
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	jrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpchttp"
)

func main() {
	serv := jrpc.New()

	if err := serv.RegisterMethod("sum", sum); err != nil {
		panic(err)
	}

	srv := &http.Server{
		Addr:              ":8088",
		Handler:           jsonrpchttp.Handler(serv),
		ReadHeaderTimeout: 5 * time.Second, // slow-client protection lives here
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      60 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		panic(err)
	}
}

type income struct {
	A int `json:"a"`
	B int `json:"b"`
}
type outcome struct {
	Sum int `json:"sum"`
}

func sum(_ context.Context, data json.RawMessage) (json.RawMessage, int, error) {
	inc := &income{}
	if err := json.Unmarshal(data, inc); err != nil {
		return nil, jrpc.InvalidParamsErrorCode, err
	}
	mdata, err := json.Marshal(outcome{Sum: inc.A + inc.B})
	if err != nil {
		return nil, jrpc.InternalErrorCode, err
	}
	return mdata, jrpc.OK, nil
}
```

Prefer a custom transport? `HandleRPCJSONRawMessage(ctx, body)` is the whole
contract — feed it raw bytes, write back what it returns (empty result means
"no response": a notification).

### Request

```bash
curl -d '{"jsonrpc":"2.0", "id":"qwe", "method":"sum", "params":{"a":5, "b":3}}' -H "Content-Type: application/json" -X POST http://localhost:8088/
```

### Response

```json
{"jsonrpc":"2.0","result":{"sum":8},"id":"qwe"}
```

## Fiber (v2 and v3)

Adapters for the [Fiber](https://gofiber.io) framework live in separate nested
modules, so Fiber and fasthttp never enter the core module's `go.mod`:

```go
// Fiber v2 — go get github.com/gumeniukcom/golang-jsonrpc2/jsonrpcfiber
import "github.com/gumeniukcom/golang-jsonrpc2/jsonrpcfiber"
app.Post("/rpc", jsonrpcfiber.Handler(rpc))

// Fiber v3 — go get github.com/gumeniukcom/golang-jsonrpc2/jsonrpcfiberv3
import "github.com/gumeniukcom/golang-jsonrpc2/jsonrpcfiberv3"
app.Post("/rpc", jsonrpcfiberv3.Handler(rpc))
```

Same semantics as the net/http adapter: 415 on a non-JSON Content-Type, 204
for notifications, JSON-RPC errors as HTTP 200. Bound the body with Fiber's
`BodyLimit`; compressed bodies (`Content-Encoding`) are rejected with 415 to
avoid decompression bombs. Method routing is Fiber's job — register with
`app.Post`. Because these are nested modules pinned to the core version,
`go get` them only after the matching core `v2.x` tag is published.

## WebSocket

The `jsonrpcws` subpackage (built on `github.com/coder/websocket`) serves
JSON-RPC over WebSocket: one frame per message, concurrent dispatch with
bounded fan-out (`WithMaxConcurrentCalls`, 16 by default), responses
correlate by id and may arrive out of order, notifications produce no frame.
Browser handshakes are same-origin by default (`WithOriginPatterns` to allow
more); frames above `WithMaxMessageSize` (1 MiB default) close the
connection with status 1009.

```go
mux.Handle("/ws", jsonrpcws.Handler(serv))
```

After the upgrade the connection is hijacked — `http.Server` timeouts no
longer apply, and the handler manages the lifecycle itself: when the read
side ends, in-flight calls are canceled; each response write is bounded by
`WithWriteTimeout` (10s default), and a write that cannot complete (slow
reader) closes the connection. Idle policy (pings/deadlines) and per-client
connection limits are the application's call.

## stdio (LSP / MCP)

The `jsonrpcstdio` subpackage (stdlib only, no new dependencies) serves
JSON-RPC over a byte stream — the transport of Language Server Protocol and
Model Context Protocol servers. The framing is an explicit choice, because
the two ecosystems are mutually unintelligible on the wire:
`FramingContentLength` (LSP: `Content-Length: N` header blocks) or
`FramingNDJSON` (MCP stdio: one JSON message per line).

```go
// The whole server: blocks until the peer closes stdin (returns nil) or the
// stream fails (returns the error). Logs must go to stderr — stdout is the
// protocol channel.
err := jsonrpcstdio.Serve(ctx, serv, jsonrpcstdio.FramingContentLength, os.Stdin, os.Stdout)
```

Dispatch is strictly sequential and in-order by default (LSP's ordering
rules; MCP SDKs do the same) — `WithMaxConcurrentCalls` opts into
ws-style bounded fan-out. One inbound frame is capped by
`WithMaxMessageSize` (8 MiB default); violating the cap is fatal to the
stream, so for a graceful band set the dispatcher's `SetMaxMessageSize` at
or below it. Handlers push server-initiated notifications
(`publishDiagnostics`, resource updates) through the `jsonrpc.Pusher` in the
request context, same as WebSocket.

The client side mirrors `jsonrpcws`: `NewClient(framing, r, w)` over the
child process's `StdoutPipe`/`StdinPipe`, multiplexed calls correlated by
id, pushes delivered to `WithNotificationHandler`. Process lifecycle
(spawning, stderr, the close-stdin → wait → SIGTERM ladder) stays with the
application.


## Documentation

Task-oriented guides live in [docs/](docs/README.md):
[choosing a transport](docs/transports.md) ·
[building an MCP/LSP server](docs/mcp-lsp.md) ·
[typed handlers & errors](docs/typed-handlers.md) ·
[clients & batches](docs/clients.md) ·
[middleware & auth](docs/middleware-auth.md) ·
[server push](docs/push-subscriptions.md) ·
[production hardening & limits](docs/production.md) ·
[observability](docs/observability.md) ·
[OpenRPC generation](docs/openrpc.md) ·
[migrating](docs/migrating.md).
API reference: [pkg.go.dev](https://pkg.go.dev/github.com/gumeniukcom/golang-jsonrpc2/v2).

## Changelog

See [CHANGELOG.md](CHANGELOG.md).
