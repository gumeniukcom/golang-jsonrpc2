# Documentation

Task-oriented guides for `github.com/gumeniukcom/golang-jsonrpc2/v2`. The API
reference lives on [pkg.go.dev](https://pkg.go.dev/github.com/gumeniukcom/golang-jsonrpc2/v2);
these pages show how the pieces fit together for a given job.

Suggested reading order:

1. [Choosing a transport](transports.md) — HTTP vs Fiber vs WebSocket vs
   stdio, and how the size limits of each layer interact.
2. [Building an MCP or LSP server](mcp-lsp.md) — stdio transport end to end.
3. [Typed handlers and errors](typed-handlers.md) — `RegisterTyped`,
   parameter handling, `RPCError`, the error-privacy model.
4. [Calling a server](clients.md) — HTTP, WebSocket, and stdio clients,
   typed results, batches.
5. [Middleware and authentication](middleware-auth.md) — `Use` chains,
   per-method policy.
6. [Server push and subscriptions](push-subscriptions.md) — `Pusher`
   lifecycle over WebSocket and stdio.
7. [Production hardening](production.md) — every limit and timeout in one
   place, and the security posture.
8. [Observability](observability.md) — the `SetObserver` hook.
9. [OpenRPC generation](openrpc.md) — self-describing APIs.
10. [Migrating](migrating.md) — from v1 of this library.

Comparison and performance data:

- [Feature comparison with other Go JSON-RPC libraries](COMPARISON.md)
- [Benchmarks](BENCHMARKS.md)

Engineering notes that are about developing this library itself (not using
it) live under [docs/dev/](dev/).
