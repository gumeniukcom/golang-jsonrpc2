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
