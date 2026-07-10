package openrpc

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// schema is the subset of JSON Schema that OpenRPC embeds. Ref, when set,
// points into #/components/schemas.
type schema struct {
	Ref                  string             `json:"$ref,omitempty"`
	Type                 string             `json:"type,omitempty"`
	Format               string             `json:"format,omitempty"`
	ContentEncoding      string             `json:"contentEncoding,omitempty"`
	Items                *schema            `json:"items,omitempty"`
	Properties           map[string]*schema `json:"properties,omitempty"`
	Required             []string           `json:"required,omitempty"`
	AdditionalProperties *schema            `json:"additionalProperties,omitempty"`
}

var (
	timeType       = reflect.TypeOf(time.Time{})
	rawMessageType = reflect.TypeOf(json.RawMessage{})
	byteSliceType  = reflect.TypeOf([]byte(nil))
)

// isWellKnown reports struct types rendered as scalars, not objects.
func isWellKnown(t reflect.Type) bool {
	return t == timeType
}

// builder accumulates named-struct definitions in defs; refs (reserved
// before the definition is built) make recursive types terminate.
type builder struct {
	defs  map[string]*schema
	names map[reflect.Type]string
}

func newBuilder() *builder {
	return &builder{
		defs:  map[string]*schema{},
		names: map[reflect.Type]string{},
	}
}

// schemaOf maps a Go type to a JSON schema. nil (no type information)
// yields the empty schema — "any value".
func (b *builder) schemaOf(t reflect.Type) *schema {
	if t == nil {
		return &schema{}
	}
	switch t {
	case rawMessageType:
		return &schema{} // any JSON value
	case timeType:
		return &schema{Type: "string", Format: "date-time"}
	case byteSliceType:
		return &schema{Type: "string", ContentEncoding: "base64"}
	}

	switch t.Kind() {
	case reflect.Pointer:
		return b.schemaOf(t.Elem())
	case reflect.Bool:
		return &schema{Type: "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return &schema{Type: "integer"}
	case reflect.Float32, reflect.Float64:
		return &schema{Type: "number"}
	case reflect.String:
		return &schema{Type: "string"}
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return &schema{Type: "string", ContentEncoding: "base64"}
		}
		return &schema{Type: "array", Items: b.schemaOf(t.Elem())}
	case reflect.Map:
		return &schema{Type: "object", AdditionalProperties: b.schemaOf(t.Elem())}
	case reflect.Struct:
		return b.structSchema(t)
	case reflect.Interface:
		return &schema{}
	default:
		// chan/func/complex/unsafe have no JSON representation.
		return &schema{}
	}
}

// structSchema renders named structs as components refs (cycle-safe) and
// anonymous structs inline.
func (b *builder) structSchema(t reflect.Type) *schema {
	if t.Name() == "" {
		return b.objectSchema(t)
	}
	if name, ok := b.names[t]; ok {
		return &schema{Ref: "#/components/schemas/" + name}
	}
	name := b.defName(t)
	b.names[t] = name
	b.defs[name] = nil // reserve: recursive fields resolve to the ref above
	b.defs[name] = b.objectSchema(t)
	return &schema{Ref: "#/components/schemas/" + name}
}

// defName picks a stable component name, qualifying with the package path
// on collision. Names are always sanitized to ^[a-zA-Z0-9._-]+$ — generic
// instantiations like Node[string] would otherwise produce keys strict
// OpenRPC/OpenAPI validators reject.
func (b *builder) defName(t reflect.Type) string {
	name := sanitizeName(t.Name())
	if _, taken := b.defs[name]; !taken {
		return name
	}
	qualified := sanitizeName(t.String())
	for i := 2; ; i++ {
		if _, taken := b.defs[qualified]; !taken {
			return qualified
		}
		qualified = sanitizeName(t.String()) + "_" + strconv.Itoa(i)
	}
}

func sanitizeName(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '.':
			return r
		default:
			return '_'
		}
	}, s)
}

type fieldInfo struct {
	jsonName  string
	omitempty bool
	typ       reflect.Type
	index     []int
}

// flatFields lists the JSON-visible fields of a struct, flattening anonymous
// embedded structs the way encoding/json does (shallow name-shadowing is not
// replicated; explicit tags win by order).
func flatFields(t reflect.Type) []fieldInfo {
	var out []fieldInfo
	seen := map[string]bool{}
	var walk func(t reflect.Type, prefix []int)
	walk = func(t reflect.Type, prefix []int) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			tag := f.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name, opts, _ := strings.Cut(tag, ",")
			if f.Anonymous && name == "" {
				ft := f.Type
				for ft.Kind() == reflect.Pointer {
					ft = ft.Elem()
				}
				if ft.Kind() == reflect.Struct {
					walk(ft, append(append([]int(nil), prefix...), i))
					continue
				}
			}
			if name == "" {
				name = f.Name
			}
			if seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, fieldInfo{
				jsonName:  name,
				omitempty: hasOpt(opts, "omitempty"),
				typ:       f.Type,
				index:     append(append([]int(nil), prefix...), i),
			})
		}
	}
	walk(t, nil)
	return out
}

func hasOpt(opts, want string) bool {
	for opts != "" {
		var o string
		o, opts, _ = strings.Cut(opts, ",")
		if o == want {
			return true
		}
	}
	return false
}

func (b *builder) objectSchema(t reflect.Type) *schema {
	s := &schema{Type: "object", Properties: map[string]*schema{}}
	for _, f := range flatFields(t) {
		s.Properties[f.jsonName] = b.schemaOf(f.typ)
		if !f.omitempty {
			s.Required = append(s.Required, f.jsonName)
		}
	}
	return s
}
