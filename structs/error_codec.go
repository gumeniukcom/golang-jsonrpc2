package structs

import (
	"encoding/json"

	"github.com/mailru/easyjson"
	"github.com/mailru/easyjson/jlexer"
	"github.com/mailru/easyjson/jwriter"

	"github.com/gumeniukcom/golang-jsonrpc2/v2/internal/codec"
)

// UnmarshalEasyJSON decodes an Error. Unlike generated code it captures the
// "data" member as raw bytes (json.RawMessage) rather than decoding it into
// an any with jlexer's recursive, depth-unbounded Interface() — a deeply
// nested "data" from a hostile peer would otherwise overflow the stack
// (an unrecoverable fatal error). Callers unmarshal Data into a concrete
// type themselves.
func (v *Error) UnmarshalEasyJSON(in *jlexer.Lexer) {
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
		case "code":
			if in.IsNull() {
				in.Skip()
			} else {
				v.Code = in.Int()
			}
		case "message":
			if in.IsNull() {
				in.Skip()
			} else {
				v.Message = in.String()
			}
		case "data":
			if in.IsNull() {
				in.Skip()
				v.Data = nil
			} else {
				// Raw() scans the value iteratively (no per-depth stack
				// frame), so nesting depth cannot overflow the stack.
				v.Data = json.RawMessage(append([]byte(nil), in.Raw()...))
			}
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
func (v *Error) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	v.UnmarshalEasyJSON(&r)
	return r.Error()
}

// MarshalEasyJSON encodes an Error. Data of any type is written through the
// same easyjson/json.Marshaler fast paths the generator used, falling back
// to internal/codec — so a json.RawMessage Data (as produced on decode)
// round-trips verbatim. The fallback deliberately goes through the codec
// seam rather than calling encoding/json directly: Error.Data is the one
// arbitrary, application-supplied value this package serializes by
// reflection, and routing it here keeps every generic marshal in the
// library on one implementation (see docs/dev/json-v2-plan.md).
func (v Error) MarshalEasyJSON(w *jwriter.Writer) {
	w.RawString(`{"code":`)
	w.Int(v.Code)
	w.RawString(`,"message":`)
	w.String(v.Message)
	if v.Data != nil {
		w.RawString(`,"data":`)
		switch m := v.Data.(type) {
		case easyjson.Marshaler:
			m.MarshalEasyJSON(w)
		case json.Marshaler:
			w.Raw(m.MarshalJSON())
		default:
			w.Raw(codec.Marshal(v.Data))
		}
	}
	w.RawByte('}')
}

// MarshalJSON implements json.Marshaler via the easyjson writer.
func (v Error) MarshalJSON() ([]byte, error) {
	var w jwriter.Writer
	v.MarshalEasyJSON(&w)
	return w.Buffer.BuildBytes(), w.Error
}
