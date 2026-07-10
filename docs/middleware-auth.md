# Middleware and authentication

## The middleware chain

Global middleware wraps every method (including ones registered later) with
pre/post-call capability — metrics, tracing, auth, result rewriting.
Composition happens copy-on-write at registration time, so middleware adds
zero per-request overhead beyond the wrappers themselves. First registered
is outermost:

```go
serv.Use(func(method string, next jrpc.RPCMethod) jrpc.RPCMethod {
	return func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		start := time.Now()
		res, code, err := next(ctx, data)
		slog.Debug("rpc", "method", method, "took", time.Since(start))
		return res, code, err
	}
})
```

Middleware wraps *registered* handlers only — method-not-found and invalid
requests never reach it. To observe those, use `SetObserver`
([observability.md](observability.md)).

## Where identity comes from: the transport

JSON-RPC has no auth of its own; authenticate at the transport and carry
identity through `context.Context`:

- **HTTP / Fiber** — wrap the handler like any other; a standard
  `net/http` middleware validates the token and stores the principal in
  the request context, which the dispatcher passes to handlers.
- **WebSocket** — authenticate the HTTP request *before* the upgrade (the
  handshake is a GET you can wrap the same way). Keep origins tight:
  same-origin is the default, and `WithOriginPatterns("*")` on an
  authenticated endpoint disables cross-site-hijacking protection — don't.
- **stdio** — the peer is the process that spawned you; OS-level trust is
  the boundary. There is usually nothing to authenticate, but all input
  remains untrusted.

```go
func withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, err := verifyBearer(r.Header.Get("Authorization"))
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(withPrincipal(r.Context(), principal)))
	})
}

mux.Handle("/rpc", withAuth(jsonrpchttp.Handler(serv)))
```

## Per-method authorization

Authorization (which principal may call which method) belongs in RPC
middleware, where the method name is in hand. Method metadata registered
via `WithExtra` makes the policy declarative:

```go
jrpc.RegisterTyped(serv, "sum", sumHandler, jrpc.WithExtra("auth", "public"))
jrpc.RegisterTyped(serv, "admin.purge", purgeHandler, jrpc.WithExtra("auth", "admin"))

authz := buildAuthzIndex(serv.Methods()) // method → required role, built once

serv.Use(func(method string, next jrpc.RPCMethod) jrpc.RPCMethod {
	required := authz[method]
	return func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		if !principalFrom(ctx).Has(required) {
			return nil, codeForbidden, errors.New("forbidden") // text stays server-side
		}
		return next(ctx, data)
	}
})
```

Register `codeForbidden` with `RegisterError` so clients get a stable
machine-readable code; the error text never leaves the server
([typed-handlers.md](typed-handlers.md#the-error-model)).

Note the closure shape: `method` (and anything derived from it, like
`required`) is resolved **once at composition time**, outside the returned
function — per-request work stays minimal.

## Lifecycle gating (the LSP pattern)

The same mechanism enforces protocol state machines — e.g. LSP's "no
requests before `initialize`":

```go
var initialized atomic.Bool

serv.Use(func(method string, next jrpc.RPCMethod) jrpc.RPCMethod {
	if method == "initialize" || method == "exit" {
		return next // always allowed
	}
	return func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		if !initialized.Load() {
			return nil, codeServerNotInitialized, errors.New("not initialized")
		}
		return next(ctx, data)
	}
})
```

## Ordering and composition rules

- First `Use` is outermost: register logging/tracing first, authz closer to
  the handler.
- Middleware may rewrite `data` before calling `next` and rewrite the
  result after — both are plain `json.RawMessage`.
- Per-method timeouts (`WithTimeout`) are not middleware: the dispatcher
  applies them around the whole composed chain.
