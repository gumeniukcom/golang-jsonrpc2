# Building an MCP or LSP server

LSP (Language Server Protocol) and MCP (Model Context Protocol) both speak
JSON-RPC 2.0 over a byte stream — canonically the stdin/stdout of a child
process — but they frame messages differently:

- **LSP**: each message is preceded by an ASCII header block —
  `Content-Length: N`, blank line, then exactly N bytes of JSON. Use
  `jsonrpcstdio.FramingContentLength`.
- **MCP (stdio)**: one JSON message per newline-terminated line. Use
  `jsonrpcstdio.FramingNDJSON`.

The framing is a mandatory argument because the two are mutually
unintelligible on the wire — a wrong guess would half-work at best. If you
do misconfigure it, the transport detects the mismatch and fails fast with
a hint naming the constant you probably meant.

## A minimal MCP server

The complete, runnable program is
[`example/mcp-stdio/`](../example/mcp-stdio/main.go) — it compiles in CI,
so it cannot drift from the API. The skeleton:

```go
rpc := jsonrpc.New()
// register initialize, tools/list, tools/call ... (see the example)

// stdout is the protocol channel; logs go to stderr (slog's default).
// Serve returns nil when the client closes our stdin — orderly shutdown.
if err := jsonrpcstdio.Serve(context.Background(), rpc,
	jsonrpcstdio.FramingNDJSON, os.Stdin, os.Stdout); err != nil {
	slog.Error("server stopped", "error", err)
	os.Exit(1)
}
```

Try it without any MCP client:

```bash
echo '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"add","arguments":{"a":2,"b":40}},"id":1}' \
  | go run ./example/mcp-stdio/
# {"jsonrpc":"2.0","result":{"content":[{"type":"text","text":"42"}]},"id":1}
```

Protocol semantics — `initialize` version negotiation, capability shapes,
tool schemas — are application logic on top; the transport deliberately does
not implement them. What it does give you: framing, batches, notifications,
sequential ordering, size caps, and push.

## An LSP server

Identical shape, different framing and bigger payloads:

```go
err := jsonrpcstdio.Serve(ctx, rpc, jsonrpcstdio.FramingContentLength,
	os.Stdin, os.Stdout,
	jsonrpcstdio.WithMaxMessageSize(32<<20)) // didOpen can carry whole files
```

Register `initialize`, `shutdown`, `textDocument/*` etc. as ordinary
methods. LSP's lifecycle state machine (`initialize` before anything else,
`shutdown`/`exit`) is yours to enforce — a middleware that rejects calls
before `initialize` is the natural place (see
[middleware-auth.md](middleware-auth.md)).

## The rules stdio imposes

1. **stdout is sacred.** Only protocol frames may be written to it — a
   single stray `fmt.Println` corrupts the session. `slog.Default()` writes
   to stderr, so idiomatic logging is already safe; never wire a stdout log
   handler in a stdio server.
2. **The peer is untrusted.** Whatever spawned you controls stdin. All
   inputs — method names, params, sizes — are attacker-controlled;
   the transport bounds frame sizes (`WithMaxMessageSize`, 8 MiB default)
   and the dispatcher bounds batches, but your handlers must validate
   their own params.
3. **Ordering is sequential by default.** LSP's ordering rules assume it;
   MCP SDKs do the same. The cost: one slow handler delays everything
   behind it, and a cancellation notification (LSP `$/cancelRequest`)
   cannot be read while the request it targets is still running. Servers
   that need in-flight cancellation opt into
   `WithMaxConcurrentCalls(n > 1)` and handle ordering themselves.
4. **Exit codes come free.** `Serve` returns `nil` on clean stdin EOF (the
   orderly shutdown signal of both protocols), so
   `if err != nil { os.Exit(1) }` produces conventional behavior.

## Pushing notifications

Both protocols rely on server-initiated notifications
(`textDocument/publishDiagnostics`, `notifications/resources/updated`).
Handlers retrieve the connection's pusher from the request context:

```go
if p, ok := jsonrpc.PusherFromContext(ctx); ok {
	_ = p.Notify(ctx, "textDocument/publishDiagnostics", diags)
}
```

The pusher stays valid for the whole connection — background goroutines can
keep pushing after the handler returns; once the stream closes, `Notify`
returns an error, which is the subscription's signal to stop. Full
lifecycle patterns: [push-subscriptions.md](push-subscriptions.md).

## Testing your server hermetically

`Serve` takes plain `io.Reader`/`io.Writer`, so tests drive it over
`io.Pipe` without spawning processes — and `jsonrpcstdio.NewClient` speaks
both framings, so a loopback client-against-server test needs no fixtures.
The transport's own test suite uses exactly this pattern.

## What v1 deliberately does not do

Server-initiated *requests* (reverse calls with response correlation — MCP
sampling, LSP `workspace/configuration`) are not supported: inbound
response-shaped frames draw a `-32600` reply. Protocol lifecycle
(`initialize` handshakes, version negotiation) and subprocess management on
the client side are application concerns. See the `jsonrpcstdio` package
documentation for the full list.
