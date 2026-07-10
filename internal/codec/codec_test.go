package codec

import "testing"

func TestRoundTrip(t *testing.T) {
	type point struct {
		X int `json:"x"`
		Y int `json:"y"`
	}
	raw, err := Marshal(point{X: 1, Y: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !Valid(raw) {
		t.Fatalf("marshaled output must be valid JSON: %s", raw)
	}
	var got point
	if err := Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.X != 1 || got.Y != 2 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestValidRejectsGarbage(t *testing.T) {
	if Valid([]byte(`{broken`)) {
		t.Fatal("malformed JSON must be rejected")
	}
}
