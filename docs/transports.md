# Choosing a transport

One dispatcher, many wires. You register methods once on a
`*jsonrpc.JSONRPC` and mount it on any (or several) of the transports below
— they can serve the same instance simultaneously.

## Decision table

| Transport | Package | Use it for | Server push | Dispatch model | Frame/body cap (default) |
|---|---|---|---|---|---|
| HTTP | `jsonrpchttp` | classic request/response APIs, curl-able | no | per-request (net/http) | `WithMaxBodySize` (1 MiB) |
| Fiber v2/v3 | `jsonrpcfiber`, `jsonrpcfiberv3` | apps already on Fiber | no | per-request (Fiber) | Fiber `BodyLimit` |
| WebSocket | `jsonrpcws` | browsers, live updates, many small calls on one connection | yes | concurrent, bounded (`WithMaxConcurrentCalls`, 16) | `WithMaxMessageSize` (1 MiB) |
| stdio | `jsonrpcstdio` | LSP servers, MCP servers, child-process plugins | yes | sequential by default (opt-in fan-out) | `WithMaxMessageSize` (8 MiB) |
| custom | — | anything else | your call | your call | your call |

A custom transport is one call: feed raw bytes to
`HandleRPCJSONRawMessage(ctx, data)` and write back what it returns — an
empty result means a notification, write nothing.

## Picking, in words

- **HTTP** is the default for public APIs: stateless, cache-headers set,
  load-balancer friendly. Notifications answer `204 No Content`, JSON-RPC
  errors ride HTTP 200, transport-level problems map to 405/415/413/400.
- **WebSocket** buys you multiplexed calls (correlated by id, out-of-order
  responses fine) and server push over one connection. The upgrade hijacks
  the connection, so the handler manages its own lifecycle: read side
  ending cancels in-flight calls; each write is bounded by
  `WithWriteTimeout` (10s).
- **stdio** is the wire of the LSP and MCP ecosystems. Framing is an
  explicit choice — `FramingContentLength` (LSP) or `FramingNDJSON` (MCP) —
  because the two are mutually unintelligible; see
  [mcp-lsp.md](mcp-lsp.md). Dispatch is strictly in-order by default, which
  is what LSP expects; `WithMaxConcurrentCalls(n)` opts into fan-out.
- **Fiber** adapters mirror `jsonrpchttp` semantics and live in separate
  nested modules, so Fiber and fasthttp never enter the core module's
  dependency graph.

## How the size limits stack

A message can be bounded at up to three layers; keep them ordered
`transport ≥ core` so oversized-but-transportable requests get a polite
JSON-RPC error instead of a dropped connection:

```
bytes on the wire
  → transport cap        WithMaxBodySize / WithMaxMessageSize / BodyLimit
      HTTP: 413 · WS: close 1009 · stdio: fatal (stream cannot resync)
  → core message cap     SetMaxMessageSize (0 = off by default)
      "request_too_large" error response, connection survives
  → core batch cap       SetMaxBatchSize (100 by default)
      "batch_too_large" error response, connection survives
```

The full hardening checklist (timeouts, concurrency bounds, origins, trust
models) is in [production.md](production.md).

## Mixing transports

Mounting the same dispatcher on several transports is normal:

```go
mux.Handle("/rpc", jsonrpchttp.Handler(serv))
mux.Handle("/ws", jsonrpcws.Handler(serv))
```

Handlers stay transport-agnostic. Where a capability is
transport-specific — server push exists on WebSocket and stdio but not
HTTP — feature-detect it: `jsonrpc.PusherFromContext(ctx)` reports
`ok == false` on transports without a push channel, and the handler
degrades gracefully.
