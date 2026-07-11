// Package jsonrpc implements a spec-conformant, transport-agnostic JSON-RPC
// 2.0 dispatcher: register methods on a JSONRPC instance, feed it raw
// messages, write back what it returns.
//
// The core knows nothing about sockets. Transport subpackages adapt it to
// the wire and share one dispatcher instance:
//
//   - jsonrpchttp — net/http handler and a stateless HTTP client
//   - jsonrpcws — WebSocket server and multiplexed client, server push
//   - jsonrpcstdio — stdin/stdout servers and clients with LSP
//     Content-Length or MCP newline-delimited framing
//   - jsonrpcfiber, jsonrpcfiberv3 — Fiber adapters (separate nested
//     modules, so Fiber never enters this module's dependency graph)
//
// A custom transport needs exactly one call:
//
//	resp := rpc.HandleRPCJSONRawMessage(ctx, rawMessage)
//	if len(resp) > 0 { /* write resp back */ } // empty ⇒ notification, write nothing
//
// # Handlers
//
// Methods are registered either at the raw level (RegisterMethod with a
// func(ctx, json.RawMessage) (json.RawMessage, int, error)) or, preferably,
// as typed handlers via generics:
//
//	err := jsonrpc.RegisterTyped(rpc, "sum", func(ctx context.Context, p SumParams) (SumResult, error) {
//		return SumResult{Sum: p.A + p.B}, nil
//	})
//
// RegisterTyped also records parameter/result types and optional metadata
// (WithSummary, WithTags, WithErrors, WithExample, WithTimeout, WithPublic,
// ...) that the openrpc subpackage renders into an OpenRPC service
// description; WithPublic opts the method into rpc.discover service
// discovery, which is default-deny.
//
// # Spec conformance and safety defaults
//
// The dispatcher implements JSON-RPC 2.0 strictly: batches (with per-entry
// error isolation), notifications (valid requests without an id are never
// answered; invalid ones draw -32600 with id:null per §5), byte-exact id
// echo, -32700/-32600 classification of malformed input. Defaults are
// DoS-aware: batches are capped at DefaultMaxBatchSize and run on a bounded
// worker pool, per-request timeouts apply (SetDefaultTimeOut, 30s), panics
// in handlers are recovered, and error text never leaks to clients unless
// opted in via RPCError.WithData.
//
// Middleware (Use), interceptors, per-method timeouts, an observability
// hook (SetObserver), and client helpers (Caller, CallResult, BatchCaller)
// round out the surface — see the guides under docs/ in the repository for
// task-oriented walkthroughs, and https://github.com/gumeniukcom/golang-jsonrpc2
// for the project README.
package jsonrpc
