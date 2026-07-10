# Changelog

All notable changes to this project are documented here. The format is based
on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
follows [Semantic Versioning](https://semver.org) for the `/v2` module.

## [Unreleased] — v2.3.0

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

### Added

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
