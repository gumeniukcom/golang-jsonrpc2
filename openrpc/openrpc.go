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
	"encoding/json"
	"reflect"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
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

	return json.Marshal(doc)
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
