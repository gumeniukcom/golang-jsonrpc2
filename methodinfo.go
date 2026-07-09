package jsonrpc

import (
	"reflect"
	"sort"
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
	Extra       map[string]any
}

// clone returns a copy safe to hand to callers: the slices and the Extra map
// are duplicated so external mutation cannot corrupt the internal registry.
// Element values (ErrorInfo is a value type; the any values in Examples and
// Extra) are shared by reference and are documented as read-only.
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

// WithExtra sets an arbitrary key/value on the method's metadata, an escape
// hatch for generator-specific hints (auth level, PAT-allowed, internal-only).
// Repeated calls with the same key overwrite.
func WithExtra(key string, value any) MethodOption {
	return func(mi *MethodInfo) {
		if mi.Extra == nil {
			mi.Extra = make(map[string]any)
		}
		mi.Extra[key] = value
	}
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
