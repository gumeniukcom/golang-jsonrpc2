# golang-jsonrpc2/v2

Implementation for JSON-RPC 2.0 protocol in Go.

Full specification: https://www.jsonrpc.org/specification

## Install

```bash
go get github.com/gumeniukcom/golang-jsonrpc2/v2
```

## HTTP example

### Server
```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	jrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
)

func main() {
	serv := jrpc.New()

	if err := serv.RegisterMethod("sum", sum); err != nil {
		panic(err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}
		defer r.Body.Close()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err = w.Write(serv.HandleRPCJSONRawMessage(r.Context(), body)); err != nil {
			panic(err)
		}
	})

	if err := http.ListenAndServe(":8088", nil); err != nil {
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
	if data == nil {
		return nil, jrpc.InvalidRequestErrorCode, fmt.Errorf("empty request")
	}
	inc := &income{}
	err := json.Unmarshal(data, inc)
	if err != nil {
		return nil, jrpc.InvalidRequestErrorCode, err
	}

	C := outcome{
		Sum: inc.A + inc.B,
	}

	mdata, err := json.Marshal(C)
	if err != nil {
		return nil, jrpc.InternalErrorCode, err
	}
	return mdata, jrpc.OK, nil
}
```

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

## v2.1.0 changes

- **Security / behavior change:** error responses no longer echo `err.Error()`
  into `error.data`. Internal detail is logged server-side (`SetLogger`,
  defaults to `slog.Default()`; internal errors at `Error`, timeouts at
  `Warn`, client-caused errors at `Debug`); client-visible detail is opt-in
  via `RPCError.WithData`.
- Added `RPCError` (authoritative code + client-safe data + wrapped
  server-side error). `WithData` returns a copy — package-level sentinels are
  safe to share.
- Added generic `Typed` / `RegisterTyped` handler adapters.
- Added `SetMaxBatchSize` (checked before unmarshaling, rejects with
  `batch_too_large`) and `SetBatchConcurrency` (worker pool; batch responses
  now keep request order).
- A response entry with unmarshalable `data` no longer destroys the whole
  batch: only that entry degrades to `internal_error`, keeping its id.

## v2.3.0 changes

- **Performance:** the dispatcher no longer spawns a goroutine + channel +
  `select` per request — handlers run inline with a plain
  `context.WithTimeout`. Configuration moved from an `RWMutex` to an
  atomically-swapped immutable snapshot (copy-on-write in setters), removing
  all locking and the interceptor-slice copy from the hot path.
- **Behavior change (timeouts):** by default the time-limit response is now
  produced when the handler returns after the deadline, instead of being
  forced at the deadline from a watchdog goroutine. `SetEnforcedTimeout(true)`
  restores the old semantics.
- **Behavior change (safe defaults):** `New()` now caps batches at
  `DefaultMaxBatchSize` (100) and batch concurrency at 4×GOMAXPROCS.
  `SetMaxBatchSize(0)` / `SetBatchConcurrency(0)` restore the old unlimited
  behavior.
- Added `SetMaxMessageSize` — reject oversized raw messages (with
  `request_too_large`) before any parsing; disabled by default.
- **Spec compliance (notifications):** a request without an `id` member is a
  notification — it executes but produces NO response (`HandleRPC` returns
  nil, `HandleRPCJSONRawMessage` returns nil); notification entries are
  filtered from batch responses, and an all-notification batch returns
  nothing. A present `"id":null` still gets a response. Transports must
  handle an empty result (HTTP: 204 No Content).
- **Spec compliance (parse errors):** malformed JSON now yields
  `-32700 parse_error` instead of `-32600`; valid JSON that is not a request
  object stays `-32600`. Whitespace around the message is accepted.
- **Breaking (structs):** `structs.Request.ID` / `structs.Response.ID`
  changed from `any` to `structs.ID` (raw JSON bytes; nil = absent id).
  Ids are now echoed byte-exact — no float64 round-trip, large integer ids
  keep precision — and marshal without reflection. `NewResponse` still
  accepts plain Go values. Interceptors now receive `structs.ID` in their
  `id any` parameter.
- id values are validated (string, number, or null only); registering method
  names with the reserved `rpc.` prefix is rejected.
- Static pre-serialized responses for reject paths (parse error, invalid
  request, too-large). The `json.RawMessage` returned by
  `HandleRPCJSONRawMessage` must be treated as **read-only**: on these paths
  it is shared package state.
- Batch entries are decoded individually: an undecodable entry gets its own
  `-32600` response with `id:null` and no longer destroys the responses of
  its valid siblings; `[1,2,3]` yields three error entries per the spec.
- `example/` hardened: `http.MaxBytesReader` + `http.Server` timeouts.
- Benchmarks live in the repo (`make bench`); CI runs an informational
  benchstat comparison against the base branch on every PR.
- CI: weekly `govulncheck` job; `toolchain go1.25.12` pin in go.mod.

## v2.2.0 changes

- Added an introspectable method registry. `RegisterTyped` now records the
  reflect types of `P`/`R` and accepts variadic `MethodOption`s (`WithSummary`,
  `WithDescription`, `WithTags`, `WithDeprecated`, `WithErrors`, `WithExample`,
  `WithExtra`); `Methods()` returns a name-sorted, defensively-copied snapshot
  of `MethodInfo`. Backward compatible — existing `RegisterTyped(j, name, fn)`
  calls and `RegisterMethod` (recorded name-only) are unchanged.
