# golang-jsonrpc2/v2

Implementation for JSON-RPC 2.0 protocol in Go.

Full specification: https://www.jsonrpc.org/specification

## Install

```bash
go get github.com/gumeniukcom/golang-jsonrpc2/v2
```

## HTTP example

### Server

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

### Fiber (v2 and v3)

Adapters for the [Fiber](https://gofiber.io) framework live in separate nested
modules, so Fiber and fasthttp never enter the core module's `go.mod`:

```go
// Fiber v2 — go get github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpcfiber
import "github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpcfiber"
app.Post("/rpc", jsonrpcfiber.Handler(rpc))

// Fiber v3 — go get github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpcfiberv3
import "github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpcfiberv3"
app.Post("/rpc", jsonrpcfiberv3.Handler(rpc))
```

Same semantics as the net/http adapter: 415 on a non-JSON Content-Type, 204
for notifications, JSON-RPC errors as HTTP 200. Bound the body with Fiber's
`BodyLimit`; compressed bodies (`Content-Encoding`) are rejected with 415 to
avoid decompression bombs. Method routing is Fiber's job — register with
`app.Post`. Because these are nested modules pinned to the core version,
`go get` them only after the matching core `v2.x` tag is published.

### WebSocket

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

### Request

```bash
curl -d '{"jsonrpc":"2.0", "id":"qwe", "method":"sum", "params":{"a":5, "b":3}}' -H "Content-Type: application/json" -X POST http://localhost:8088/
```

### Response

```json
{"jsonrpc":"2.0","result":{"sum":8},"id":"qwe"}
```

## Typed handlers

`Typed` / `RegisterTyped` remove the `json.RawMessage` boilerplate — params are
unmarshaled and the result marshaled automatically:

```go
type sumParams struct {
	A int `json:"a"`
	B int `json:"b"`
}
type sumResult struct {
	Sum int `json:"sum"`
}

err := jrpc.RegisterTyped(serv, "sum", func(ctx context.Context, p sumParams) (sumResult, error) {
	return sumResult{Sum: p.A + p.B}, nil
})
```

A malformed `params` object yields `InvalidParamsErrorCode` automatically; any
plain error returned by the handler maps to `InternalErrorCode`.

## Introspection & documentation metadata

`RegisterTyped` records the reflect types of `P` and `R` plus optional
documentation metadata, so the same registry the server dispatches against can
also describe itself — the source of truth for out-of-band schema generation
(OpenRPC, OpenAPI, agent-facing docs) with no drift.

```go
err := jrpc.RegisterTyped(serv, "sum", sumHandler,
	jrpc.WithSummary("Add two integers"),
	jrpc.WithDescription("Returns the sum of a and b."),
	jrpc.WithTags("math", "public"),
	jrpc.WithErrors(jrpc.ErrorInfo{Code: -32602, Message: "invalid_method_parameters", Description: "a or b missing"}),
	jrpc.WithExample("basic", sumParams{A: 3, B: 5}, sumResult{Sum: 8}),
	jrpc.WithExtra("auth", "public"),
)

for _, m := range serv.Methods() { // sorted by name; slices/maps deep-copied
	fmt.Println(m.Name, m.Params, m.Result, m.Summary)
}
```

Options are additive and backward-compatible — `RegisterTyped(serv, name, fn)`
without options behaves exactly as before. Slice options (`WithTags`,
`WithErrors`, `WithExample`) accumulate across repeated calls. `Methods()`
returns a name-sorted snapshot whose slices and `Extra` map are copied, so the
caller may read and reorder freely. A method registered through the untyped
`RegisterMethod` appears with a nil `Params`/`Result` (name-only); a typed
method with `struct{}` params keeps that non-nil zero-field type, so a
generator can distinguish "no parameters" from "no type information".

## Application errors and the `data` field

Error texts are **never** sent to clients: internal detail (driver errors,
wrapped messages, panic values) is written to the configured logger only. To
return a specific code and client-visible detail, use `RPCError`:

```go
serv.SetLogger(slog.Default()) // default; pass nil to disable server-side logging

const codeCropLimit = 4001
_ = serv.RegisterError(codeCropLimit, "custom_crop_limit_exceeded")

// inside a handler:
return sumResult{}, jrpc.NewRPCError(codeCropLimit, err).WithData(map[string]any{"limit": 5})
```

The response carries the registered message and the `WithData` payload; the
wrapped `err` goes to the server log only. `RPCError` is matched through
wrapping (`errors.As`), so `fmt.Errorf("...: %w", rpcErr)` works, and its
`Code` is authoritative — it overrides the int code returned through the
`RPCMethod` signature, so code and data always come from the same error. An
unregistered `Code` degrades to `internal_error` without data and logs the
original code.

Log levels: internal errors log at `Error`, timeouts at `Warn`, client-caused
and registered application errors at `Debug` — a flood of bad requests cannot
spam the log at `Error` level.

## Batch and size limits

Batches execute on a worker pool. Since v2.3.0 the defaults are DoS-safe:
batches are capped at `DefaultMaxBatchSize` (100) requests and run on at most
4×GOMAXPROCS workers. Tune or disable:

```go
serv.SetMaxBatchSize(20)     // larger batches rejected with "batch_too_large" BEFORE unmarshaling; 0 disables
serv.SetBatchConcurrency(4)  // worker pool of 4; responses keep request order; 0 = goroutine per entry
serv.SetMaxMessageSize(1 << 20) // reject messages over 1 MiB before parsing; 0 (default) disables
```

`SetMaxBatchSize` and `SetMaxMessageSize` bound parsing work (both are checked
on the raw bytes before unmarshaling), but not raw body size — cap that at the
transport layer too (`http.MaxBytesReader`, see `example/`).

## Timeouts

Handlers run inline on the caller's goroutine with a per-request
`context.WithTimeout` (`SetDefaultTimeOut`, 30s default). If the deadline has
expired by the time the handler returns, the client gets a
`request_time_limit` error — so a handler that ignores `ctx.Done()` delays the
(still time-limit) response until it returns. Handlers should respect context
cancellation.

If you need the time-limit response delivered exactly at the deadline even
when a handler hangs, opt in to the pre-v2.3.0 behavior:

```go
serv.SetEnforcedTimeout(true) // goroutine per call; responds at the deadline,
                              // the stuck handler keeps running in the background
```

In enforced mode the batch concurrency bound counts started requests: a
handler that ignores cancellation no longer occupies a worker slot, so stuck
handlers can accumulate without bound.

Cancellation of the caller's context (client disconnect, shutdown) is not a
time limit: a handler that completes keeps its response. Only enforced mode
aborting a still-running call reports `request_time_limit` on cancellation,
logging the real cause server-side.

## Client

Both transport subpackages ship a client implementing the `jsonrpc.Caller`
contract (`Call`/`Notify`); `jsonrpc.CallResult[R]` mirrors the server-side
typed handlers. JSON-RPC error responses come back as `*structs.Error`
(match with `errors.As`), transport failures as ordinary errors.

```go
// HTTP: stateless, one POST per call.
c := jsonrpchttp.NewClient("http://localhost:8088/")
sum, err := jsonrpc.CallResult[int](ctx, c, "sum", map[string]int{"a": 3, "b": 4})

// WebSocket: one connection, concurrent calls correlated by id.
wc, err := jsonrpcws.DialClient(ctx, "ws://localhost:8088/ws")
defer wc.Close()
sum, err = jsonrpc.CallResult[int](ctx, wc, "sum", map[string]int{"a": 3, "b": 4})
```

### Server push (WebSocket)

Over WebSocket a handler can push server-initiated notifications to the
client. The transport injects a `jsonrpc.Pusher` into the request context;
the handler retrieves it and sends notifications that reach the client's
`WithNotificationHandler`. Plain HTTP has no push channel, so
`PusherFromContext` reports `false` there — handlers degrade gracefully.

The pusher stays valid for the life of the connection, so a subscription can
push from a background goroutine after the handler returns — use a fresh
context there, not the handler's request context (which cancels on return).
`Notify` returns an error once the connection closes, the signal to stop; a
push loop must honor it. On the client, `WithNotificationHandler` receives
`method` and `params` straight from the server — treat them as untrusted.

```go
// Server handler:
serv.RegisterMethod("subscribe", func(ctx context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
	if p, ok := jsonrpc.PusherFromContext(ctx); ok {
		_ = p.Notify(ctx, "tick", map[string]int{"n": 1})
	}
	return json.RawMessage(`"ok"`), jsonrpc.OK, nil
})

// Client:
c, _ := jsonrpcws.DialClient(ctx, "ws://localhost:8088/ws",
	jsonrpcws.WithNotificationHandler(func(method string, params json.RawMessage) {
		log.Printf("push %s: %s", method, params)
	}))
```

Both clients also send batches via `CallBatch` (the `jsonrpc.BatchCaller`
contract): pass `[]jsonrpc.Spec`, get `[]jsonrpc.BatchResult` aligned by
index (notification specs get a zero slot), and decode each with
`jsonrpc.BatchResultAs[R]`. Over WebSocket, batch responses correlate by id
alongside concurrent single calls on the same connection.

```go
results, err := c.CallBatch(ctx, []jsonrpc.Spec{
	{Method: "sum", Params: map[string]int{"a": 1, "b": 2}},
	{Method: "log", Params: "hi", Notify: true}, // no response slot
})
sum, err := jsonrpc.BatchResultAs[int](results[0])
```

A batch larger than the client's limit (`WithMaxBatchSize` /
`WithClientMaxBatchSize`, default `DefaultMaxBatchSize` = 100) fails locally
with `ErrBatchTooLarge` — the server would otherwise reject an oversized batch
with a single unaddressable error, which the WebSocket client cannot correlate
to the call.

On a decoded error response `structs.Error.Data` holds a `json.RawMessage`
— unmarshal it into a concrete type yourself. `Error.Message` and `Data`
come from the server: treat them as untrusted input, and escape or
structure them before writing to logs.

## Middleware

Global middleware wraps every method (including ones registered later) with
post-call capability — metrics, tracing, auth, result rewriting. Composition
happens copy-on-write at registration, so middleware adds zero per-request
overhead beyond the wrappers themselves. First registered is outermost:

```go
serv.Use(func(method string, next jrpc.RPCMethod) jrpc.RPCMethod {
	return func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		start := time.Now()
		res, code, err := next(ctx, data)
		log.Printf("%s took %v", method, time.Since(start))
		return res, code, err
	}
})
```

Per-method timeouts override the server default:

```go
jrpc.RegisterTyped(serv, "report.build", buildReport,
	jrpc.WithTimeout(5*time.Minute)) // this method may run longer than the default
```

## Observability

`SetObserver` installs a hook called once per dispatched request with its
outcome — method, client-facing code, error, duration, and whether it was a
notification. Unlike middleware (which wraps a registered handler), it runs on
the dispatch path, so it sees *every* request including method-not-found,
invalid requests, timeouts, and panics; in a batch it fires per entry. Use it
for metrics, tracing, or request logging:

```go
serv.SetObserver(func(ctx context.Context, info jrpc.CallInfo) {
	rpcLatency.WithLabelValues(info.Method, strconv.Itoa(info.Code)).Observe(info.Duration.Seconds())
})
```

The hook runs on the request goroutine (concurrently across batch entries) —
keep it cheap and concurrency-safe, offload slow exports, and treat
`info.Method` as untrusted. Frame-level rejects that never become a request
object (oversized messages/batches, top-level parse errors) are not observed —
they are logged at Debug instead. A panic in the hook is recovered and logged.

## OpenRPC document

The `openrpc` subpackage renders an [OpenRPC 1.3.2](https://spec.open-rpc.org)
service description straight from the dispatch registry — typed param/result
schemas, tags, errors, examples:

```go
import "github.com/gumeniukcom/golang-jsonrpc2/v2/openrpc"

doc, _ := openrpc.Document(openrpc.Info{Title: "My API", Version: "1.0.0"}, serv.Methods())
mux.HandleFunc("/openrpc.json", func(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(doc)
})
```

The document is a complete map of your API — method names, type shapes,
`Examples` and `Extra` values are published verbatim. Put the endpoint
behind auth (or keep it internal), don't put secrets in examples, and
filter internal-only methods out of `Methods()` before generating.

## Migration from v1

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
-j.SetDefaultTimeOut(5)            // was int seconds
+j.SetDefaultTimeOut(5 * time.Second) // now time.Duration
```

### Bug fixes included

- Fixed goroutine leaks in request handling
- Fixed interceptor context chaining (contexts now properly propagate)
- Fixed `"id":1` → `"id":null` in parse/validation error responses (per spec)
- Added `sync.RWMutex` for concurrent safety
- Replaced deprecated `satori/go.uuid` with `google/uuid`

## Changelog

See [CHANGELOG.md](CHANGELOG.md).
