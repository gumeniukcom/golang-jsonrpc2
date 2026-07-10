# Changelog

All notable changes to this project are documented here. The format is based
on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
follows [Semantic Versioning](https://semver.org) for the `/v2` module.

## [Unreleased] — v2.4.0

### Added

- `jsonrpcstdio`: stdio transport (stdlib only, lives in the core module) —
  the wire of LSP and MCP servers. Two framings behind one API, selected by
  a mandatory `Framing` enum: `FramingContentLength` (LSP base protocol)
  and `FramingNDJSON` (MCP stdio). Server is a single blocking
  `Serve(ctx, rpc, framing, r, w, opts...)`: returns `nil` on clean stdin
  EOF (orderly shutdown), drains in-flight handlers on every path, injects
  a `jsonrpc.Pusher` for server-initiated notifications, and latches the
  writer on exit so a late background `Notify` gets an error instead of a
  SIGPIPE. Dispatch is strictly sequential and in-order by default
  (`DefaultMaxConcurrentCalls = 1`, per LSP ordering rules and MCP SDK
  precedent); `WithMaxConcurrentCalls` opts into ws-style bounded fan-out.
  Inbound frames are capped by `WithMaxMessageSize` (8 MiB default, fatal
  on violation — checked before allocation); well-framed garbage draws
  -32700/-32600 and the stream survives. Wrong-framing misconfiguration is
  detected and reported with a hint at the right constant. Client mirrors
  `jsonrpcws` minus batches: `NewClient(framing, r, w)` (no ctx — no
  handshake), `Call`/`Notify`, id-correlated multiplexing,
  `WithNotificationHandler` for pushes; process lifecycle stays with the
  caller.

## [2.3.0]

### Performance

- The dispatcher no longer spawns a goroutine + channel + `select` per
  request — handlers run inline with a plain `context.WithTimeout`.
  Configuration moved from an `RWMutex` to an atomically-swapped immutable
  snapshot (copy-on-write in setters), removing all locking and the
  interceptor-slice copy from the hot path. Single dispatch: 3.7 µs → 1.0 µs,
  25 → 18 allocs (Apple M3) — faster than a hand-rolled `encoding/json`
  dispatcher.
- Static pre-serialized responses for reject paths (parse error, invalid
  request, too-large). The `json.RawMessage` returned by
  `HandleRPCJSONRawMessage` must be treated as **read-only** on these paths.

### Internal

- json/v2 migration prep (no behavior change, no new build requirements):
  all generic reflection-based JSON now routes through one `internal/codec`
  seam, so adopting `encoding/json/v2` when it stabilizes (≈ Go 1.27) is a
  one-file change. The default build stays on `encoding/json` (v1) and works
  on Go 1.25+ with no build flags. CI runs the full suite (core + adapters)
  under `GOEXPERIMENT=jsonv2`. See MIGRATION.md.

### Added

- Observability: `SetObserver(ObserveFunc)` installs a hook called once per
  dispatched request with a `CallInfo` (method, client-facing code, error,
  duration, notification flag). It runs on the dispatch path — seeing
  method-not-found, invalid requests, timeouts, and panics that a handler
  middleware never sees — and fires per entry in a batch, including entries
  whose JSON did not decode. Frame-level rejects before dispatch (oversized
  messages/batches, top-level parse errors) are not observed (logged at
  Debug instead). The hook may be called concurrently across batch workers,
  `CallInfo.Method` is untrusted client input, and a panicking hook is
  recovered and logged rather than crashing the server. Zero cost when unset.
- Client batches: the `jsonrpc.BatchCaller` contract (`CallBatch`) with
  `Spec` / `BatchResult` and the typed `BatchResultAs[R]` helper, implemented
  by both `jsonrpchttp.Client` (one POST) and `jsonrpcws.Client` (correlated
  by id over the shared connection, alongside concurrent single calls).
  Results align by index with specs; notification specs get a zero slot; an
  empty batch is a no-op. Batch responses are streamed (not unmarshaled into
  a slice) so a hostile array frame cannot amplify memory. A batch over the
  client limit (`WithMaxBatchSize` / `WithClientMaxBatchSize`, default 100)
  fails locally with `ErrBatchTooLarge`, and an unaddressable top-level error
  fails the pending calls rather than hanging them.
- Server push over WebSocket: handlers retrieve a `jsonrpc.Pusher` from the
  request context (`PusherFromContext`) to send server-initiated
  notifications, delivered to the client's `WithNotificationHandler`. The
  `Pusher` contract and `ContextWithPusher`/`PusherFromContext` live in the
  core package (transport-neutral); plain HTTP reports no pusher so handlers
  degrade gracefully. Push frames share the connection's serialized,
  time-bounded writer with responses.
- Client side: the `jsonrpc.Caller` contract (`Call`/`Notify`) with typed
  `jsonrpc.CallResult[R]`, implemented by `jsonrpchttp.NewClient`
  (stateless POST, bounded response body via `WithMaxResponseSize`) and
  `jsonrpcws.DialClient` (one connection, concurrent calls correlated by
  id, pending calls fail on close). JSON-RPC error responses surface as
  `*structs.Error`, which now implements `error`.

### Security

- `structs.Error` decoding no longer reads the `data` member into an `any`
  through easyjson's recursive, depth-unbounded lexer — a hostile peer
  could crash a client with a deeply nested `data` (an unrecoverable
  `fatal error: stack overflow`). `data` is now captured as raw bytes
  (`json.RawMessage`); on decode `Error.Data` holds a `json.RawMessage` to
  unmarshal into a concrete type. Excessive nesting is rejected as an
  ordinary decode error instead of crashing.
- `jsonrpcws` subpackage — a WebSocket transport (github.com/coder/websocket,
  the only new dependency — itself dependency-free): one frame per JSON-RPC
  message, concurrent dispatch with bounded fan-out
  (`WithMaxConcurrentCalls`), out-of-order responses correlated by id,
  no frames for notifications, same-origin handshakes by default
  (`WithOriginPatterns`), read limit with 1009 close
  (`WithMaxMessageSize`), bounded response writes (`WithWriteTimeout`,
  10s default — a slow reader closes the connection instead of wedging
  it), and in-flight call cancellation when the read side ends.
- Fiber adapters: `jsonrpcfiber` (Fiber v2) and `jsonrpcfiberv3` (Fiber v3),
  each a separate nested Go module so Fiber and fasthttp stay out of the core
  `go.mod`. Same semantics as the net/http adapter (415 / 204 / HTTP-200
  errors), body bounded by Fiber's `BodyLimit`. Compressed bodies are
  rejected (decompression-bomb guard), the request context is taken from the
  safe user-context accessor (not the pooled Fiber `Ctx`, which would race
  under enforced-timeout mode), and Fiber is pinned to patched versions
  (v2.52.12 / v3.1.0). The nested modules are `go get`-able only after the
  core `v2.x` tag is published.
- `jsonrpchttp` subpackage — an `http.Handler` transport: bounded request
  body (1 MiB default, `WithMaxBodySize`), `204 No Content` for
  notifications, `Content-Type` validation, transport failures mapped to
  HTTP codes (405/415/413/400) while JSON-RPC errors stay HTTP 200.
  Responses carry `Cache-Control: no-store` and `X-Content-Type-Options:
  nosniff`; auth/CORS/CSRF are application policy (see the package doc for
  the cookie-auth CSRF caveat). `example/` now uses it.
- Release workflow: pushing a `v2.*` tag runs the tests and publishes a
  GitHub release with generated notes.
- `Use(Middleware)` — global middleware with post-call capability, composed
  copy-on-write at registration (zero per-request overhead). First registered
  is outermost.
- `WithTimeout(d)` — per-method timeout override; exposed via
  `MethodInfo.Timeout`.
- `openrpc` subpackage — generates an OpenRPC 1.3.2 service description from
  `Methods()`: typed param/result JSON schemas (cycle-safe via
  `components/schemas`), tags, errors, examples, `x-extra` extensions.
- `SetMaxMessageSize` — reject oversized raw messages (`request_too_large`)
  before any parsing; disabled by default.
- `SetEnforcedTimeout(true)` — restores the pre-v2.3.0 watchdog-goroutine
  timeout semantics (response exactly at the deadline).
- Benchmarks in-repo (`make bench`); CI runs an informational benchstat
  comparison against the base branch on every PR, plus weekly `govulncheck`
  and a `GOEXPERIMENT=jsonv2` compatibility job.

### Changed (behavior)

- **Timeouts:** by default the time-limit response is produced when the
  handler returns after the deadline instead of being forced at the deadline
  from a watchdog goroutine. Cancellation of the caller's context is not a
  time limit: a completed handler keeps its response.
- **Safe defaults:** `New()` caps batches at `DefaultMaxBatchSize` (100) and
  batch concurrency at 4×GOMAXPROCS. `SetMaxBatchSize(0)` /
  `SetBatchConcurrency(0)` restore the old unlimited behavior.
- **Spec compliance (notifications):** a request without an `id` member is a
  notification — it executes but produces no response; notification entries
  are filtered from batch responses and an all-notification batch returns
  nothing. A present `"id":null` still gets a response. Transports must
  handle an empty result (HTTP: 204 No Content).
- **Spec compliance (parse errors):** malformed JSON yields `-32700
  parse_error` instead of `-32600`; valid JSON that is not a request object
  stays `-32600`. Whitespace around the message is accepted.
- Batch entries are decoded individually: an undecodable entry gets its own
  `-32600` response with `id:null` and no longer destroys the responses of
  its valid siblings; `[1,2,3]` yields three error entries per the spec.
- id values are validated (string, number, or null); broken scalar ids are
  never echoed back. Registering method names with the reserved `rpc.`
  prefix is rejected.

### Breaking

- `structs.Request.ID` / `structs.Response.ID` changed from `any` to
  `structs.ID` (raw JSON bytes; nil = absent id). Ids echo byte-exact — no
  float64 round-trip, large integer ids keep precision — and marshal without
  reflection. `NewResponse` still accepts plain Go values. Interceptors now
  receive `structs.ID` in their `id any` parameter.

## [2.2.0]

- Added an introspectable method registry. `RegisterTyped` records the
  reflect types of `P`/`R` and accepts variadic `MethodOption`s
  (`WithSummary`, `WithDescription`, `WithTags`, `WithDeprecated`,
  `WithErrors`, `WithExample`, `WithExtra`); `Methods()` returns a
  name-sorted, defensively-copied snapshot of `MethodInfo`.

## [2.1.0]

- **Security / behavior change:** error responses no longer echo
  `err.Error()` into `error.data`. Internal detail is logged server-side
  (`SetLogger`, defaults to `slog.Default()`); client-visible detail is
  opt-in via `RPCError.WithData`.
- Added `RPCError` (authoritative code + client-safe data + wrapped
  server-side error).
- Added generic `Typed` / `RegisterTyped` handler adapters.
- Added `SetMaxBatchSize` and `SetBatchConcurrency` (worker pool; batch
  responses keep request order).
- A response entry with unmarshalable `data` no longer destroys the whole
  batch: only that entry degrades to `internal_error`, keeping its id.

## [2.0.0]

- Module path moved to `github.com/gumeniukcom/golang-jsonrpc2/v2`.
- Renamed: `HandleRPCJsonRawMessage` → `HandleRPCJSONRawMessage`,
  `ParamsDataMarshaller` → `ParamsDataMarshaler`, `Request()`/`Response()` →
  `NewRequest()`/`NewResponse()`.
- `SetDefaultTimeOut` accepts `time.Duration` instead of `int` seconds.
- Fixed goroutine leaks, interceptor context chaining, `"id":null` in
  parse/validation error responses; replaced `satori/go.uuid` with
  `google/uuid`.
