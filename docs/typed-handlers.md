# Typed handlers and errors

## RegisterTyped

`Typed` / `RegisterTyped` remove the `json.RawMessage` boilerplate — params
are unmarshaled and the result marshaled automatically, via generics, at
compile time:

```go
type sumParams struct {
	A int `json:"a"`
	B int `json:"b"`
}
type sumResult struct {
	Sum int `json:"sum"`
}

err := jrpc.RegisterTyped(serv, "sum", func(ctx context.Context, p sumParams) (sumResult, error) {
	return sumResult{Sum: p.A + p.B}, nil
})
```

A malformed `params` object yields `InvalidParamsErrorCode` automatically;
any plain error returned by the handler maps to `InternalErrorCode` — with
its text kept out of the response (see below).

Parameter shapes:

- **Struct params** are the common case; validate them in the handler
  (required fields, ranges) — JSON decoding alone enforces types, not
  presence.
- **`struct{}`** means "no parameters"; extra params from the client are
  ignored by the decoder's defaults.
- **Slices** work for positional params (`[]int`, `[]string`, or a custom
  tuple type).
- **`json.RawMessage`** opts out of decoding for one method while keeping
  the typed result side.

The raw signature remains available where you need the int code channel or
zero-copy params:

```go
serv.RegisterMethod("sum", func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) { ... })
```

## Documentation metadata

`RegisterTyped` records the reflect types of `P` and `R` plus optional
metadata, making the registry self-describing — the source of truth for
[OpenRPC generation](openrpc.md) with no drift:

```go
err := jrpc.RegisterTyped(serv, "sum", sumHandler,
	jrpc.WithSummary("Add two integers"),
	jrpc.WithTags("math", "public"),
	jrpc.WithExample("basic", sumParams{A: 3, B: 5}, sumResult{Sum: 8}),
	jrpc.WithTimeout(10*time.Second), // per-method override, see production.md
)
```

Options are additive and backward-compatible; `Methods()` returns a
name-sorted, defensively-copied snapshot.

## The error model

Error texts are **never** sent to clients by default: internal detail
(driver errors, wrapped messages, panic values) is written to the
configured logger only. The client sees a code and its registered message —
nothing else.

To return a specific code and client-visible detail, use `RPCError`:

```go
serv.SetLogger(slog.Default()) // default; pass nil to disable server-side logging

const codeCropLimit = 4001
_ = serv.RegisterError(codeCropLimit, "custom_crop_limit_exceeded")

// inside a handler:
return sumResult{}, jrpc.NewRPCError(codeCropLimit, err).WithData(map[string]any{"limit": 5})
```

The response carries the registered message and the `WithData` payload; the
wrapped `err` goes to the server log only. Rules that make this composable:

- `RPCError` is matched through wrapping (`errors.As`), so
  `fmt.Errorf("...: %w", rpcErr)` works.
- Its `Code` is authoritative — it overrides the int code returned through
  the raw `RPCMethod` signature, so code and data always come from the same
  error.
- An unregistered `Code` degrades to `internal_error` without data and logs
  the original code.
- Don't defeat the privacy default by echoing `err.Error()` into
  `WithData` — put deliberate, structured, client-safe values there.

Log levels are flood-aware: internal errors log at `Error`, timeouts at
`Warn`, client-caused and registered application errors at `Debug` — a
storm of bad requests cannot spam the log at `Error` level.

## Panics

A panic in a handler is recovered and answered as `internal_error`; the
panic value goes to the server log. This is a seatbelt, not a control-flow
mechanism — return errors.
