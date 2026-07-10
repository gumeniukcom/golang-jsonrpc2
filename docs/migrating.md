# Migrating

## From v1 of this library

### Module path

```diff
-import jrpc "github.com/gumeniukcom/golang-jsonrpc2"
+import jrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
```

### Renamed symbols

| v1 | v2 |
|----|----|
| `HandleRPCJsonRawMessage` | `HandleRPCJSONRawMessage` |
| `ParamsDataMarshaller` | `ParamsDataMarshaler` |
| `Request()` | `NewRequest()` (removed unused `ctx` param) |
| `Response()` | `NewResponse()` (removed unused `ctx` param) |

### Timeout API change

`SetDefaultTimeOut` now accepts `time.Duration` instead of `int`:

```diff
-j.SetDefaultTimeOut(5)                // was int seconds
+j.SetDefaultTimeOut(5 * time.Second)  // now time.Duration
```

### Behavior fixes you inherit

- Notifications per spec: valid requests without an id are executed but
  never answered; invalid ones draw `-32600` with `id:null`.
- `-32700` vs `-32600` classification of malformed input per spec.
- Byte-exact id echo (`structs.ID`), `"id":null` in parse/validation error
  responses.
- Goroutine-leak and interceptor context-chaining fixes; `satori/go.uuid`
  replaced with `google/uuid`.

## From sourcegraph/jsonrpc2

Rough concept mapping for the most common migration:

| sourcegraph/jsonrpc2 | here |
|---|---|
| `jsonrpc2.NewConn(ctx, stream, handler)` | `jsonrpcstdio.Serve(ctx, rpc, framing, r, w)` — the dispatcher replaces the single `Handle` callback |
| `Handler.Handle(ctx, conn, req)` + manual `switch req.Method` | one `RegisterTyped`/`RegisterMethod` per method |
| `conn.Reply / ReplyWithError` | return values of the handler |
| `VSCodeObjectCodec` (Content-Length framing) | `jsonrpcstdio.FramingContentLength` |
| `PlainObjectCodec` (newline-delimited) | `jsonrpcstdio.FramingNDJSON` |
| `conn.Notify` (server→client) | `jsonrpc.PusherFromContext(ctx)` → `Notify` |
| `conn.Call` (client side) | `jsonrpcstdio.NewClient` → `Call` / `jsonrpc.CallResult[R]` |
| batches | native here (single requests only there) |

Things to re-check rather than translate one-to-one: request-scoped state
you attached to the `*jsonrpc2.Conn` (move it to context via middleware),
and any reliance on concurrent handler execution — this library's stdio
transport dispatches sequentially by default (`WithMaxConcurrentCalls` opts
into fan-out).
