package structs

import (
	"encoding/json"

	"github.com/mailru/easyjson/jlexer"
	"github.com/mailru/easyjson/jwriter"
)

// UnmarshalEasyJSON decodes a Request. Unlike generated code it keeps a
// present "id":null as the literal bytes "null", so an absent id (nil ID, a
// notification) stays distinguishable.
func (v *Request) UnmarshalEasyJSON(in *jlexer.Lexer) {
	isTopLevel := in.IsStart()
	if in.IsNull() {
		if isTopLevel {
			in.Consumed()
		}
		in.Skip()
		return
	}
	in.Delim('{')
	for !in.IsDelim('}') {
		key := in.UnsafeFieldName(false)
		in.WantColon()
		switch key {
		case "jsonrpc":
			v.Version = in.String()
		case "method":
			v.Method = in.String()
		case "params":
			if in.IsNull() {
				in.Skip()
				v.Params = nil
			} else {
				// Raw returns a subslice of the lexer's input buffer — copy,
				// so a decoded Request never aliases a caller-reused buffer.
				v.Params = append(json.RawMessage(nil), in.Raw()...)
			}
		case "id":
			// Raw returns "null" for a present null id; absence keeps nil.
			// Copied for the same buffer-reuse reason as params.
			v.ID = append(ID(nil), in.Raw()...)
		default:
			in.SkipRecursive()
		}
		in.WantComma()
	}
	in.Delim('}')
	if isTopLevel {
		in.Consumed()
	}
}

// UnmarshalJSON implements json.Unmarshaler via the easyjson lexer.
func (v *Request) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	v.UnmarshalEasyJSON(&r)
	return r.Error()
}

// MarshalEasyJSON encodes a Request. The id member is omitted entirely for a
// nil ID (a notification) and written verbatim otherwise.
func (v Request) MarshalEasyJSON(w *jwriter.Writer) {
	w.RawString(`{"jsonrpc":`)
	w.String(v.Version)
	w.RawString(`,"method":`)
	w.String(v.Method)
	if len(v.Params) > 0 {
		w.RawString(`,"params":`)
		w.Raw(v.Params, nil)
	}
	if v.ID != nil {
		w.RawString(`,"id":`)
		v.ID.MarshalEasyJSON(w)
	}
	w.RawByte('}')
}

// MarshalJSON implements json.Marshaler via the easyjson writer.
func (v Request) MarshalJSON() ([]byte, error) {
	var w jwriter.Writer
	v.MarshalEasyJSON(&w)
	return w.BuildBytes()
}
