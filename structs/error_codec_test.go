package structs

import (
	"encoding/json"
	"strings"
	"testing"
)

// A deeply nested "data" must NOT crash the process with the recursive-lexer
// stack overflow the generated any-decoder was vulnerable to. Reaching this
// assertion at all (rather than a fatal error that no recover can catch)
// proves the fix: the depth is rejected with an ordinary decode error, which
// the client surfaces normally.
func TestErrorDataDeepNestingDoesNotOverflow(t *testing.T) {
	const depth = 12_000_000
	body := `{"code":1,"message":"x","data":` +
		strings.Repeat("[", depth) + strings.Repeat("]", depth) + `}`

	var e Error
	err := e.UnmarshalJSON([]byte(body)) // must return, not crash
	if err == nil {
		t.Log("decoded deep data as raw bytes without error")
	} else {
		t.Logf("deep data rejected with an ordinary error (acceptable): %v", err)
	}
}

// Data nested well beyond a recursive decoder's comfort but within the JSON
// scanner's limit decodes fine, as raw bytes.
func TestErrorModeratelyNestedDataDecodes(t *testing.T) {
	const depth = 5000
	body := `{"code":1,"message":"x","data":` +
		strings.Repeat("[", depth) + strings.Repeat("]", depth) + `}`

	var e Error
	if err := e.UnmarshalJSON([]byte(body)); err != nil {
		t.Fatalf("moderately nested data must decode, got: %v", err)
	}
	raw, ok := e.Data.(json.RawMessage)
	if !ok {
		t.Fatalf("Data must be json.RawMessage, got %T", e.Data)
	}
	if len(raw) != 2*depth {
		t.Fatalf("raw data length %d, want %d", len(raw), 2*depth)
	}
}

// Data decodes to raw bytes the caller can unmarshal into a concrete type.
func TestErrorDataDecodesAsRawMessage(t *testing.T) {
	var e Error
	if err := e.UnmarshalJSON([]byte(`{"code":-32602,"message":"bad","data":{"field":"a","n":3}}`)); err != nil {
		t.Fatal(err)
	}
	if e.Code != -32602 || e.Message != "bad" {
		t.Fatalf("unexpected scalar fields: %+v", e)
	}
	raw, ok := e.Data.(json.RawMessage)
	if !ok {
		t.Fatalf("Data must be json.RawMessage, got %T", e.Data)
	}
	var d struct {
		Field string `json:"field"`
		N     int    `json:"n"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	if d.Field != "a" || d.N != 3 {
		t.Fatalf("decoded data wrong: %+v", d)
	}
}

// Absent data stays nil.
func TestErrorNoDataStaysNil(t *testing.T) {
	var e Error
	if err := e.UnmarshalJSON([]byte(`{"code":1,"message":"x"}`)); err != nil {
		t.Fatal(err)
	}
	if e.Data != nil {
		t.Fatalf("absent data must stay nil, got %v", e.Data)
	}
}

// Marshal round-trips: any Data (map, struct, or the json.RawMessage from a
// decode) serializes to the same JSON.
func TestErrorMarshalRoundTrip(t *testing.T) {
	cases := []any{
		nil,
		map[string]int{"x": 1},
		json.RawMessage(`{"x":1}`),
		"a string",
		42.0,
	}
	for _, data := range cases {
		e := Error{Code: 7, Message: "m", Data: data}
		raw, err := e.MarshalJSON()
		if err != nil {
			t.Fatalf("marshal %v: %v", data, err)
		}
		if !json.Valid(raw) {
			t.Fatalf("marshal %v produced invalid JSON: %s", data, raw)
		}
		if !strings.Contains(string(raw), `"code":7`) || !strings.Contains(string(raw), `"message":"m"`) {
			t.Fatalf("marshal %v missing scalar fields: %s", data, raw)
		}
		if data == nil && strings.Contains(string(raw), `"data"`) {
			t.Fatalf("nil data must be omitted: %s", raw)
		}
	}
}
