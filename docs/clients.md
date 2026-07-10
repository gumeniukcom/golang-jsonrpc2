# Calling a server

The HTTP, WebSocket, and stdio transports each ship a client implementing the `jsonrpc.Caller`
contract (`Call`/`Notify`); `jsonrpc.CallResult[R]` mirrors the server-side
typed handlers. JSON-RPC error responses come back as `*structs.Error`
(match with `errors.As`), transport failures as ordinary errors.

## Choosing a client

| Client | Connection model | Batches | Server push |
|---|---|---|---|
| `jsonrpchttp.NewClient` | stateless, one POST per call | yes | no |
| `jsonrpcws.DialClient` | one connection, calls multiplexed by id | yes | yes |
| `jsonrpcstdio.NewClient` | child-process pipes, multiplexed by id | no (v1) | yes |

```go
// HTTP: stateless, one POST per call.
c := jsonrpchttp.NewClient("http://localhost:8088/")
sum, err := jsonrpc.CallResult[int](ctx, c, "sum", map[string]int{"a": 3, "b": 4})

// WebSocket: one connection, concurrent calls correlated by id.
wc, err := jsonrpcws.DialClient(ctx, "ws://localhost:8088/ws")
defer wc.Close()
sum, err = jsonrpc.CallResult[int](ctx, wc, "sum", map[string]int{"a": 3, "b": 4})
```

The stdio client attaches to an already-started child process — it does not
spawn it. Wire `r` to the child's stdout and `w` to its stdin:

```go
cmd := exec.Command("path/to/server")
stdin, _ := cmd.StdinPipe()
stdout, _ := cmd.StdoutPipe()
cmd.Stderr = os.Stderr // the child's logs; keep them visible
_ = cmd.Start()
sc, err := jsonrpcstdio.NewClient(jsonrpcstdio.FramingNDJSON, stdout, stdin)
```

Process lifecycle (waiting, the close-stdin → wait → SIGTERM ladder) stays
with your application; see the `jsonrpcstdio` package documentation.

## Batches

The HTTP and WebSocket clients send batches via `CallBatch` (the
`jsonrpc.BatchCaller` contract): pass `[]jsonrpc.Spec`, get
`[]jsonrpc.BatchResult` aligned by index (notification specs get a zero
slot), and decode each with `jsonrpc.BatchResultAs[R]`. Over WebSocket,
batch responses correlate by id alongside concurrent single calls on the
same connection.

```go
results, err := c.CallBatch(ctx, []jsonrpc.Spec{
	{Method: "sum", Params: map[string]int{"a": 1, "b": 2}},
	{Method: "log", Params: "hi", Notify: true}, // no response slot
})
sum, err := jsonrpc.BatchResultAs[int](results[0])
```

A batch larger than the client's limit (`WithMaxBatchSize` /
`WithClientMaxBatchSize`, default `DefaultMaxBatchSize` = 100) fails locally
with `ErrBatchTooLarge`. The limit exists because an oversized batch draws a
single unaddressable `id:null` rejection from the server — and on a
multiplexed connection an uncorrelatable error has to fail every in-flight
call, so it is far cheaper to refuse locally.

## Receiving server push

The WebSocket and stdio clients deliver server-initiated notifications
(id-less frames) to the handler registered with `WithNotificationHandler`;
without one they are dropped. The handler runs on the client's single read
loop — return promptly and offload slow work. `method` and `params` arrive
straight from the server: treat them as untrusted before mapping them to any
local action. See [push-subscriptions.md](push-subscriptions.md) for the
server side.

## Error handling notes

On a decoded error response `structs.Error.Data` holds a `json.RawMessage` —
unmarshal it into a concrete type yourself. `Error.Message` and `Data` come
from the server: treat them as untrusted input, and escape or structure them
before writing to logs.

A server rejection that carries no id (a top-level `id:null` error such as a
parse error) cannot be correlated to one call on a multiplexed connection,
so the WebSocket and stdio clients fail every in-flight call with it —
an innocent concurrent call may also fail. Keeping client-side limits at or
below the server's avoids provoking such rejections.
