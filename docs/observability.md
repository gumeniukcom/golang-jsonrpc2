# Observability

`SetObserver` installs a hook called once per dispatched request with its
outcome — method, client-facing code, error, duration, and whether it was a
notification. Unlike middleware (which wraps a registered handler), it runs
on the dispatch path, so it sees *every* request including method-not-found,
invalid requests, timeouts, and panics; in a batch it fires per entry.

```go
serv.SetObserver(func(ctx context.Context, info jrpc.CallInfo) {
	rpcLatency.WithLabelValues(boundMethod(info.Method), strconv.Itoa(info.Code)).
		Observe(info.Duration.Seconds())
})
```

## Contract and caveats

- The hook runs on the request goroutine — concurrently across batch
  entries. Keep it cheap and concurrency-safe; offload slow exports to a
  channel or a background goroutine.
- `info.Method` is an attacker-controlled string straight from the wire.
  **Never use it as an unbounded metrics label** — a peer cycling method
  names explodes label cardinality (a metrics-DoS). Map it through your
  registry first:

  ```go
  func boundMethod(m string) string {
  	if _, ok := knownMethods[m]; ok { return m }
  	return "_unknown"
  }
  ```

- `info.Err` is the *client-facing* error (`*structs.Error`), not the
  internal error — internal detail stays in the server log by design.
- Frame-level rejects that never become a request object are not
  observed: oversized messages/batches are logged at Debug; top-level
  parse errors are answered but not logged — meter them at the transport
  layer if you need them.
- A panic in the hook is recovered and logged; it cannot crash a batch
  worker. Do not rely on that as a feature — treat it as a seatbelt.

## Middleware vs observer

| | `Use` middleware | `SetObserver` |
|---|---|---|
| Sees method-not-found / invalid requests | no | yes |
| Sees timeouts and panics as outcomes | no | yes |
| Can rewrite params/result | yes | no |
| Runs | around the handler | after dispatch decides the outcome |

Use middleware to *change* behavior (auth, tracing spans around handler
work, result rewriting) and the observer to *record* outcomes (metrics,
request logs). They compose: middleware for `Authorization`, observer for
the latency histogram.
