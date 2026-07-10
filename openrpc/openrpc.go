// Package openrpc generates an OpenRPC 1.3.2 service description from the
// introspectable method registry of a jsonrpc.JSONRPC server (Methods()).
// The same registry the server dispatches against also documents it: typed
// params/result schemas are derived from the reflect types captured by
// RegisterTyped, and documentation metadata (summary, tags, errors,
// examples) comes from MethodOption values.
//
// See https://spec.open-rpc.org for the document format.
package openrpc

import (
	"context"
	"encoding/json"
	"reflect"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/internal/codec"
)

// Info describes the service; it maps to the OpenRPC "info" object.
type Info struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
}

type document struct {
	OpenRPC    string      `json:"openrpc"`
	Info       Info        `json:"info"`
	Methods    []method    `json:"methods"`
	Components *components `json:"components,omitempty"`
}

type components struct {
	Schemas map[string]*schema `json:"schemas,omitempty"`
}

type tag struct {
	Name string `json:"name"`
}

type contentDescriptor struct {
	Name     string  `json:"name"`
	Required bool    `json:"required,omitempty"`
	Schema   *schema `json:"schema"`
}

type errObj struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type example struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

type examplePairing struct {
	Name   string    `json:"name"`
	Params []example `json:"params"`
	Result *example  `json:"result,omitempty"`
}

type method struct {
	Name           string              `json:"name"`
	Summary        string              `json:"summary,omitempty"`
	Description    string              `json:"description,omitempty"`
	Deprecated     bool                `json:"deprecated,omitempty"`
	Tags           []tag               `json:"tags,omitempty"`
	ParamStructure string              `json:"paramStructure,omitempty"`
	Params         []contentDescriptor `json:"params"`
	Result         *contentDescriptor  `json:"result,omitempty"`
	Errors         []errObj            `json:"errors,omitempty"`
	Examples       []examplePairing    `json:"examples,omitempty"`
	// Extra carries MethodInfo.Extra as a single x-extension object; OpenRPC
	// allows x- prefixed extension members.
	Extra map[string]any `json:"x-extra,omitempty"`
}

// RegisterDiscover registers the OpenRPC service-discovery method
// "rpc.discover" on rpc. Each call builds the document from the server's own
// Methods() at that moment (~a hundred microseconds for a realistic
// registry), so it always reflects the live registry — including methods
// registered after RegisterDiscover, and rpc.discover itself. Registration
// order does not matter. "rpc.discover" is the one "rpc."-prefixed name a
// server may register: JSON-RPC 2.0 §4.1 reserves the prefix for extensions,
// and this is the extension the OpenRPC spec defines.
//
// The method takes no params and returns the raw OpenRPC document. A build
// error (rare — an Example or Extra value that fails to marshal) reaches the
// client as a generic internal_error; the detail is logged server-side only.
// The default summary/description can be overridden by passing WithSummary /
// WithDescription in opts (options apply in order, last write wins).
//
// Security: the document is the complete map of the API — every method name,
// param/result schema, Examples and Extra values verbatim — served to anyone
// who can call methods, with no filtering. If any method is internal-only,
// either gate "rpc.discover" itself with middleware (see
// docs/middleware-auth.md; note an authz index built from Methods() must be
// built AFTER this registration, or default-deny unknown methods) or skip
// RegisterDiscover and serve a filtered Document() from your own handler or
// endpoint instead (see docs/openrpc.md).
func RegisterDiscover(rpc *jsonrpc.JSONRPC, info Info, opts ...jsonrpc.MethodOption) error {
	handler := func(context.Context, struct{}) (json.RawMessage, error) {
		return Document(info, rpc.Methods())
	}
	base := []jsonrpc.MethodOption{
		jsonrpc.WithSummary("Return the service's OpenRPC description"),
		jsonrpc.WithDescription("The service-discovery method from the OpenRPC specification: returns the OpenRPC 1.3.2 document describing every method, its param/result schemas, and its documentation."),
	}
	return jsonrpc.RegisterTyped(rpc, "rpc.discover", handler, append(base, opts...)...)
}

// Document renders the OpenRPC 1.3.2 JSON document for the given methods —
// typically the result of (*jsonrpc.JSONRPC).Methods().
func Document(info Info, methods []jsonrpc.MethodInfo) (json.RawMessage, error) {
	b := newBuilder()

	doc := document{
		OpenRPC: "1.3.2",
		Info:    info,
		Methods: make([]method, 0, len(methods)),
	}

	for _, mi := range methods {
		m := method{
			Name:        mi.Name,
			Summary:     mi.Summary,
			Description: mi.Description,
			Deprecated:  mi.Deprecated,
			Params:      []contentDescriptor{},
		}
		for _, t := range mi.Tags {
			m.Tags = append(m.Tags, tag{Name: t})
		}

		m.ParamStructure, m.Params = b.paramsOf(mi.Params)

		resultSchema := b.schemaOf(mi.Result)
		m.Result = &contentDescriptor{Name: "result", Schema: resultSchema}

		for _, e := range mi.Errors {
			eo := errObj{Code: e.Code, Message: e.Message}
			if e.Description != "" {
				eo.Data = e.Description
			}
			m.Errors = append(m.Errors, eo)
		}

		for _, ex := range mi.Examples {
			m.Examples = append(m.Examples, examplePairing{
				Name:   ex.Name,
				Params: exampleParams(ex.Params),
				Result: &example{Name: "result", Value: ex.Result},
			})
		}

		if len(mi.Extra) > 0 {
			m.Extra = mi.Extra
		}

		doc.Methods = append(doc.Methods, m)
	}

	if len(b.defs) > 0 {
		doc.Components = &components{Schemas: b.defs}
	}

	return codec.Marshal(doc)
}

// paramsOf maps a params type to OpenRPC content descriptors: a struct type
// becomes by-name parameters (one per exported field); nil (no type info or
// explicitly no params) becomes an empty list; anything else becomes a
// single positional "params" descriptor.
func (b *builder) paramsOf(t reflect.Type) (string, []contentDescriptor) {
	if t == nil {
		return "", []contentDescriptor{}
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() == reflect.Struct && !isWellKnown(t) {
		cds := make([]contentDescriptor, 0, t.NumField())
		for _, f := range flatFields(t) {
			cds = append(cds, contentDescriptor{
				Name:     f.jsonName,
				Required: !f.omitempty,
				Schema:   b.schemaOf(f.typ),
			})
		}
		if len(cds) == 0 {
			// A zero-field struct means "no parameters": announcing
			// paramStructure by-name would (meaninglessly) forbid the
			// positional spelling of an empty params list.
			return "", cds
		}
		return "by-name", cds
	}
	return "", []contentDescriptor{{Name: "params", Schema: b.schemaOf(t)}}
}

// exampleParams splits a struct or map example value into per-parameter
// examples matching by-name params; other values become one "params" entry.
func exampleParams(v any) []example {
	if v == nil {
		return []example{}
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return []example{}
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Struct:
		if isWellKnown(rv.Type()) {
			break
		}
		out := make([]example, 0, rv.NumField())
		for _, f := range flatFields(rv.Type()) {
			// FieldByIndexErr instead of FieldByIndex: stepping through a nil
			// embedded pointer must skip the field, not panic the generator.
			fv, err := rv.FieldByIndexErr(f.index)
			if err != nil {
				continue
			}
			out = append(out, example{Name: f.jsonName, Value: fv.Interface()})
		}
		return out
	case reflect.Map:
		if rv.Type().Key().Kind() == reflect.String {
			out := make([]example, 0, rv.Len())
			for _, k := range rv.MapKeys() {
				out = append(out, example{Name: k.String(), Value: rv.MapIndex(k).Interface()})
			}
			return out
		}
	default:
	}
	return []example{{Name: "params", Value: v}}
}
