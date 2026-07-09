package structs

import (
	"github.com/mailru/easyjson/jlexer"
	"github.com/mailru/easyjson/jwriter"
)

// ID holds the raw JSON bytes of a request/response id (a string, number, or
// null — validation happens at the dispatch layer). Keeping the raw bytes
// avoids reflection on the hot path and preserves large integer ids exactly.
//
// A nil ID means the id member was ABSENT from the request — per JSON-RPC 2.0
// that request is a notification. A present "id":null decodes to the literal
// bytes "null", so the two cases stay distinguishable. Both marshal as null.
type ID []byte

// MarshalEasyJSON writes the raw id bytes; an absent (nil) id serializes as
// null, which is what error responses for id-less invalid requests need.
func (v ID) MarshalEasyJSON(w *jwriter.Writer) {
	if len(v) == 0 {
		w.RawString("null")
		return
	}
	w.Raw(v, nil)
}

// UnmarshalEasyJSON captures the raw bytes of the id value. A JSON null is
// kept as the literal "null" so it stays distinct from an absent id (nil).
func (v *ID) UnmarshalEasyJSON(l *jlexer.Lexer) {
	*v = ID(l.Raw())
}

// MarshalJSON implements encoding/json interop; absent ids marshal as null.
func (v ID) MarshalJSON() ([]byte, error) {
	if len(v) == 0 {
		return []byte("null"), nil
	}
	return v, nil
}

// UnmarshalJSON implements encoding/json interop. The input is copied because
// encoding/json reuses its buffer after the call.
func (v *ID) UnmarshalJSON(data []byte) error {
	*v = append(ID(nil), data...)
	return nil
}
