# OpenRPC generation

The `openrpc` subpackage renders an [OpenRPC 1.3.2](https://spec.open-rpc.org)
service description straight from the dispatch registry — typed param/result
schemas, tags, errors, examples. Because it reads the same registry the
server dispatches against, the document cannot drift from the code.

```go
import "github.com/gumeniukcom/golang-jsonrpc2/v2/openrpc"

// openrpc.Public keeps only methods opted in with jrpc.WithPublic();
// pass serv.Methods() verbatim only for a trusted internal audience.
// Built once here — do it after all registrations (or rebuild per request;
// the in-band rpc.discover flow below is always per-call fresh).
doc, _ := openrpc.Document(openrpc.Info{Title: "My API", Version: "1.0.0"}, openrpc.Public(serv.Methods()))
mux.HandleFunc("/openrpc.json", func(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(doc)
})
```

The metadata comes from `RegisterTyped` options:

```go
err := jrpc.RegisterTyped(serv, "sum", sumHandler,
	jrpc.WithSummary("Add two integers"),
	jrpc.WithDescription("Returns the sum of a and b."),
	jrpc.WithTags("math", "public"),
	jrpc.WithErrors(jrpc.ErrorInfo{Code: -32602, Message: "invalid_method_parameters", Description: "a or b missing"}),
	jrpc.WithExample("basic", sumParams{A: 3, B: 5}, sumResult{Sum: 8}),
	jrpc.WithExtra("auth", "public"), // private: middleware policy, never published
	jrpc.WithPublishedExtra("stability", "beta"), // published as x-extra
	jrpc.WithPublic(), // opt into discovery (default-deny)
)
```

`Methods()` returns a name-sorted snapshot whose slices and metadata maps
(`Extra`, `PublishedExtra`) are copied, so you may filter and reorder freely
before generating. A method
registered through the untyped `RegisterMethod` appears with nil
`Params`/`Result` (name-only); a typed method with `struct{}` params keeps
that non-nil zero-field type, so a generator can distinguish "no parameters"
from "no type information".

## In-band discovery: rpc.discover

`RegisterDiscover` serves the document over the RPC surface itself as the
OpenRPC-standard `rpc.discover` method — the one `rpc.`-prefixed name the
registry permits (JSON-RPC 2.0 reserves the prefix for exactly such
extensions):

```go
jrpc.RegisterTyped(serv, "sum", sumHandler, jrpc.WithPublic()) // opt in
_ = openrpc.RegisterDiscover(serv, openrpc.Info{Title: "My API", Version: "1.0.0"})
```

**Discovery is default-deny.** Only methods registered with
`jsonrpc.WithPublic()` appear in the document (`rpc.discover` marks itself);
an unannotated method stays hidden, so a forgotten annotation hides instead
of leaking. Two boundaries to keep straight:

- *Hidden is not protected*: an unlisted method remains callable — access
  control stays middleware's job ([middleware-auth.md](middleware-auth.md)).
- *Public is documentation, not authorization*: listing a method does not
  grant anything.

Metadata follows the same split: `WithExtra` is private (authorization
markup, generator hints — never published), `WithPublishedExtra` appears in
the document verbatim as `x-extra`. Don't put secrets or authz policy in
published values.

The document is built per call from the live registry, so late registrations
appear as soon as they carry `WithPublic()`; registration order does not
matter. For custom setups (own gating, own method name), the handler is
exported as `openrpc.DiscoverHandler(serv, info)`, and the same public
subset is available for the HTTP flow as `openrpc.Public(serv.Methods())` —
`Document` itself never filters, so a trusted internal portal can still
render everything by passing `serv.Methods()` verbatim.

Notes for unauthenticated exposure: the response is the largest object the
server emits and is not bounded by `SetMaxMessageSize` (that caps requests),
and a batch of discover calls multiplies it — the default batch cap (100)
bounds the amplification, but consider rate limiting.

## Security notes

The document is a complete map of what you publish — method names, type
shapes, `Examples` and `PublishedExtra` (`x-extra`) values verbatim:

- Put the endpoint behind auth, or keep it on an internal listener.
- Don't put secrets in `WithExample` payloads or `WithPublishedExtra`
  values (`WithExtra` is private and never published).
- Publish only the opted-in subset — the first-class mechanism:

  ```go
  doc, _ := openrpc.Document(info, openrpc.Public(serv.Methods()))
  ```

  Custom policies stay possible: filter `serv.Methods()` yourself (e.g. by
  private `Extra` values) before passing the slice to `Document`.
