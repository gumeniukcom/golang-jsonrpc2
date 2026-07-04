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

## Batch limits

Batches execute on a worker pool; unbounded by default. On an
internet-exposed endpoint, bound them:

```go
serv.SetMaxBatchSize(20)     // larger batches rejected with "batch_too_large" BEFORE unmarshaling
serv.SetBatchConcurrency(4)  // worker pool of 4; responses keep request order
```

`SetMaxBatchSize` bounds parsing work (the element count is checked on the
raw bytes), but not raw body size — cap that at the transport layer. The
concurrency bound counts started requests: a handler that ignores context
cancellation after its timeout keeps running in the background and no longer
occupies a worker slot.

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
