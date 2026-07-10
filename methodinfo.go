package jsonrpc

import (
	"reflect"
	"sort"
	"time"
)

// ErrorInfo describes one error a method may return. It is documentation-only
// metadata — the authoritative error code at runtime still comes from the
// handler's returned *RPCError (see rpcerror.go). Description is a human/agent
// readable explanation of when the error is emitted.
type ErrorInfo struct {
	Code        int
	Message     string
	Description string
}

// ExamplePair is a named request/response example for a method. Params is the
// value passed as the method's params object (matching the handler's P type)
// and Result is the corresponding return value (matching R). Both are held as
// documentation data and are never executed; callers must not mutate them
// after registration.
type ExamplePair struct {
	Name   string
	Params any
	Result any
}

// MethodInfo is the introspectable description of a registered method: its
// name, the reflect types of its params and result, and any documentation
// metadata supplied via MethodOption values. It is the source of truth for
// out-of-band schema/documentation generation (OpenRPC, OpenAPI, …) — the same
// registry the server dispatches against also describes itself.
//
// Params and Result are the concrete reflect types captured by RegisterTyped.
// They are nil for a method registered through the untyped RegisterMethod,
// which carries no type information. A method whose params type is a zero-field
// struct (e.g. struct{}) keeps that non-nil type, so a generator can tell
// "explicitly no parameters" apart from "no type information".
type MethodInfo struct {
	Name        string
	Params      reflect.Type
	Result      reflect.Type
	Summary     string
	Description string
	Deprecated  bool
	Tags        []string
	Errors      []ErrorInfo
	Examples    []ExamplePair

	// Extra carries arbitrary generator/middleware hints (auth level,
	// internal-only, ...). It is PRIVATE metadata: never published in
	// generated service descriptions. Use PublishedExtra for values that
	// should appear in the document.
	Extra map[string]any

	// PublishedExtra carries key/values that generators publish verbatim
	// (the openrpc package emits them as the x-extra extension). Anything
	// here is visible to every consumer of the document — no secrets, no
	// authorization markup.
	PublishedExtra map[string]any

	// Public opts the method into generated service discovery
	// (openrpc.RegisterDiscover / openrpc.Public). The zero value hides the
	// method from discovery — fail-closed: a forgotten annotation hides
	// instead of leaking. Public is documentation visibility only, NOT
	// access control: the method remains callable; gate calls with
	// middleware.
	Public bool

	// Timeout overrides the server default (SetDefaultTimeOut) for this
	// method. Zero means inherit the default.
	Timeout time.Duration
}

// clone returns a copy safe to hand to callers: the slices and both metadata
// maps are duplicated so external mutation cannot corrupt the internal
// registry. Element values (ErrorInfo is a value type; the any values in
// Examples, Extra, and PublishedExtra) are shared by reference and are
// documented as read-only.
func (mi MethodInfo) clone() MethodInfo {
	c := mi
	if mi.Tags != nil {
		c.Tags = append([]string(nil), mi.Tags...)
	}
	if mi.Errors != nil {
		c.Errors = append([]ErrorInfo(nil), mi.Errors...)
	}
	if mi.Examples != nil {
		c.Examples = append([]ExamplePair(nil), mi.Examples...)
	}
	if mi.Extra != nil {
		c.Extra = make(map[string]any, len(mi.Extra))
		for k, v := range mi.Extra {
			c.Extra[k] = v
		}
	}
	if mi.PublishedExtra != nil {
		c.PublishedExtra = make(map[string]any, len(mi.PublishedExtra))
		for k, v := range mi.PublishedExtra {
			c.PublishedExtra[k] = v
		}
	}
	return c
}

// MethodOption supplies documentation metadata to RegisterTyped. Options are
// applied in order; the slice-appending options (WithTags, WithErrors,
// WithExample) accumulate across repeated calls.
type MethodOption func(*MethodInfo)

// WithSummary sets a one-line summary of what the method does.
func WithSummary(summary string) MethodOption {
	return func(mi *MethodInfo) { mi.Summary = summary }
}

// WithDescription sets a longer prose description of the method.
func WithDescription(description string) MethodOption {
	return func(mi *MethodInfo) { mi.Description = description }
}

// WithTags appends grouping tags (e.g. a namespace, an auth level). Repeated
// calls accumulate.
func WithTags(tags ...string) MethodOption {
	return func(mi *MethodInfo) { mi.Tags = append(mi.Tags, tags...) }
}

// WithDeprecated marks the method as deprecated.
func WithDeprecated() MethodOption {
	return func(mi *MethodInfo) { mi.Deprecated = true }
}

// WithErrors appends documented errors the method may return. Repeated calls
// accumulate.
func WithErrors(errs ...ErrorInfo) MethodOption {
	return func(mi *MethodInfo) { mi.Errors = append(mi.Errors, errs...) }
}

// WithExample appends a named request/response example. Repeated calls
// accumulate.
func WithExample(name string, params, result any) MethodOption {
	return func(mi *MethodInfo) {
		mi.Examples = append(mi.Examples, ExamplePair{Name: name, Params: params, Result: result})
	}
}

// WithTimeout sets a per-method execution timeout that overrides the server
// default (SetDefaultTimeOut) for this method only. Non-positive values are
// ignored (the default stays in effect).
func WithTimeout(d time.Duration) MethodOption {
	return func(mi *MethodInfo) {
		if d > 0 {
			mi.Timeout = d
		}
	}
}

// WithExtra sets an arbitrary key/value on the method's PRIVATE metadata, an
// escape hatch for generator/middleware hints (auth level, PAT-allowed,
// internal-only). Private means never published in generated service
// descriptions — safe for authorization markup. To publish a value in the
// document, use WithPublishedExtra. Repeated calls with the same key
// overwrite.
func WithExtra(key string, value any) MethodOption {
	return func(mi *MethodInfo) {
		if mi.Extra == nil {
			mi.Extra = make(map[string]any)
		}
		mi.Extra[key] = value
	}
}

// WithPublishedExtra sets a key/value that generators publish verbatim in the
// service description (the openrpc package emits it as the x-extra extension
// member). Everything set here is visible to every consumer of the document:
// no secrets, no authorization markup — keep those in WithExtra, which is
// never published. Repeated calls with the same key overwrite.
func WithPublishedExtra(key string, value any) MethodOption {
	return func(mi *MethodInfo) {
		if mi.PublishedExtra == nil {
			mi.PublishedExtra = make(map[string]any)
		}
		mi.PublishedExtra[key] = value
	}
}

// WithPublic opts the method into generated service discovery
// (openrpc.RegisterDiscover and the openrpc.Public filter). Discovery is
// default-deny: methods without this option never appear in the generated
// document, so a forgotten annotation hides instead of leaking. Public
// controls documentation visibility only, NOT access control — the method
// remains callable by anyone the transport lets through; gate calls with
// middleware.
func WithPublic() MethodOption {
	return func(mi *MethodInfo) { mi.Public = true }
}

// Methods returns a snapshot of every registered method's MethodInfo, sorted
// by name. The returned values are deep-copied at the slice/map level so the
// caller may read and reorder them freely without affecting the registry.
func (j *JSONRPC) Methods() []MethodInfo {
	cfg := j.cfg.Load()

	out := make([]MethodInfo, 0, len(cfg.methodInfo))
	for _, info := range cfg.methodInfo {
		out = append(out, info.clone())
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out
}
