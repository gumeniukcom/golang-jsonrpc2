# OpenRPC generation

The `openrpc` subpackage renders an [OpenRPC 1.3.2](https://spec.open-rpc.org)
service description straight from the dispatch registry — typed param/result
schemas, tags, errors, examples. Because it reads the same registry the
server dispatches against, the document cannot drift from the code.

```go
import "github.com/gumeniukcom/golang-jsonrpc2/v2/openrpc"

doc, _ := openrpc.Document(openrpc.Info{Title: "My API", Version: "1.0.0"}, serv.Methods())
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
	jrpc.WithExtra("auth", "public"),
)
```

`Methods()` returns a name-sorted snapshot whose slices and `Extra` map are
copied, so you may filter and reorder freely before generating. A method
registered through the untyped `RegisterMethod` appears with nil
`Params`/`Result` (name-only); a typed method with `struct{}` params keeps
that non-nil zero-field type, so a generator can distinguish "no parameters"
from "no type information".

## Security notes

The document is a complete map of your API — method names, type shapes,
`Examples` and `Extra` values are published verbatim:

- Put the endpoint behind auth, or keep it on an internal listener.
- Don't put secrets in `WithExample` payloads or `WithExtra` values.
- Filter internal-only methods out of `Methods()` before generating:

  ```go
  public := slices.DeleteFunc(serv.Methods(), func(m jrpc.MethodInfo) bool {
  	return m.Extra["auth"] != "public"
  })
  doc, _ := openrpc.Document(info, public)
  ```
