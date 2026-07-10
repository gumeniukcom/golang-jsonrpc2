# Production hardening

Everything you should set (or consciously leave at its default) before
exposing a dispatcher to traffic you do not control. The library's defaults
are deliberately DoS-aware, but several bounds live at the transport layer
and are yours to configure.

## The limits, in one table

Requests pass through up to three size gates. Keep them ordered
`transport ≥ core`, so oversized-but-transportable messages draw a polite
JSON-RPC error instead of a connection-level rejection:

| Layer | Knob | Default | On violation |
|---|---|---|---|
| HTTP body | `jsonrpchttp.WithMaxBodySize` | 1 MiB | HTTP 413 |
| Fiber body | Fiber's `BodyLimit` (app-level) | 4 MiB (Fiber default) | HTTP 413 |
| WebSocket frame | `jsonrpcws.WithMaxMessageSize` | 1 MiB | close 1009 |
| stdio frame | `jsonrpcstdio.WithMaxMessageSize` | 8 MiB | fatal — `Serve` returns |
| Core message | `SetMaxMessageSize` | 0 (disabled) | `request_too_large`, id:null¹ |
| Core batch length | `SetMaxBatchSize` | 100 | `batch_too_large`, id:null¹ |
| Batch concurrency | `SetBatchConcurrency` | 4×GOMAXPROCS | (bound, not an error) |

¹ When the rejected payload verifiably carries no id (a notification, or a
batch of only notifications), the rejection is silent per the spec's
notification rule.

`SetMaxBatchSize` and `SetMaxMessageSize` bound parsing work — both are
checked on the raw bytes before unmarshaling. They do not bound how many
bytes the transport reads into memory; that is what the transport-layer caps
are for.

```go
serv.SetMaxBatchSize(20)        // 0 disables
serv.SetBatchConcurrency(4)     // 0 = goroutine per batch entry
serv.SetMaxMessageSize(1 << 20) // 0 (default) disables
```

## Timeouts

Handlers run inline on the caller's goroutine with a per-request
`context.WithTimeout` (`SetDefaultTimeOut`, 30s default). If the deadline
has expired by the time the handler returns, the client gets a
`request_time_limit` error — so a handler that ignores `ctx.Done()` delays
the (still time-limit) response until it returns. Handlers should respect
context cancellation.

If you need the time-limit response delivered exactly at the deadline even
when a handler hangs, opt in to enforced mode:

```go
serv.SetEnforcedTimeout(true) // goroutine per call; responds at the deadline,
                              // the stuck handler keeps running in the background
```

Enforced mode trades a goroutine + channel per request for the guarantee,
and it weakens the concurrency bounds: the batch worker pool and the
WebSocket/stdio transports' `WithMaxConcurrentCalls` count *started*
requests, so handlers that ignore cancellation accumulate without bound. **Keep the default inline
mode for servers exposed to untrusted peers.**

Cancellation of the caller's context (client disconnect, shutdown) is not a
time limit: a handler that completes keeps its response. Only enforced mode
aborting a still-running call reports `request_time_limit` on cancellation,
logging the real cause server-side.

Per-method overrides beat the server default:

```go
jrpc.RegisterTyped(serv, "report.build", buildReport,
	jrpc.WithTimeout(5*time.Minute))
```

And the HTTP server itself needs its own timeouts — the library cannot set
them for you:

```go
srv := &http.Server{
	Addr:              ":8088",
	Handler:           jsonrpchttp.Handler(serv),
	ReadHeaderTimeout: 5 * time.Second, // Slowloris protection lives here
	ReadTimeout:       10 * time.Second,
	WriteTimeout:      60 * time.Second,
}
```

## Error privacy

Error texts are never sent to clients by default: internal detail (driver
errors, wrapped messages, panic values) goes to the configured logger only,
and client-visible detail is opt-in via `RPCError.WithData`. Do not weaken
this by echoing internal errors into `WithData` yourself. See
[typed-handlers.md](typed-handlers.md) for the full error model.

## Transport-specific care

- **WebSocket origins.** Browser handshakes are same-origin by default.
  `jsonrpcws.WithOriginPatterns` widens that; never use `"*"` on an
  authenticated endpoint — it disables cross-site WebSocket-hijacking
  protection. Authenticate the HTTP request *before* the upgrade.
- **WebSocket lifetimes.** The upgrade hijacks the connection: the
  `http.Server` read/write timeouts above stop applying to it. The only
  built-in write bound is `WithWriteTimeout` (10s); idle policy
  (pings/read deadlines) and per-client connection limits are yours to
  enforce at the application or load-balancer layer.
- **stdio trust model.** A stdio server's peer is whatever process spawned
  it. Everything arriving on stdin is untrusted input; stdout is the
  protocol channel and must never receive log output (`slog.Default()`
  writes to stderr, which is already correct).
- **Compressed bodies.** The Fiber adapters reject
  `Content-Encoding != identity` with 415: Fiber decompresses before the
  body-size limit applies, which would otherwise allow decompression bombs.

## Log and metric hygiene

Method names, error messages, and `error.data` payloads arrive from the
peer and are untrusted:

- Bound and sanitize `info.Method` before using it as a metrics label —
  unbounded label cardinality is a metrics-DoS vector (see
  [observability.md](observability.md)).
- The default `slog` handlers escape structured attributes; if you install
  a custom plain-text sink, escape peer-derived strings yourself (log
  injection).
- Log levels are already flood-aware: client-caused errors log at `Debug`,
  timeouts at `Warn`, internal errors at `Error`.

## Keeping dependencies clean

The core module carries three direct dependencies; Fiber adapters live in
separate nested modules so their dependency graphs never reach your build
unless you import them. CI runs `govulncheck` weekly — do the same in your
own pipeline (`golang.org/x/vuln/cmd/govulncheck@latest ./...`).
