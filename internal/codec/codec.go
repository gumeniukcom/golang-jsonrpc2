// Package codec is the single seam through which the library performs generic,
// reflection-based JSON (de)serialization — method params and results, client
// payloads, and structural validation. Consolidating these calls here makes
// the eventual migration to encoding/json/v2 a one-file change instead of
// touching every call site.
//
// encoding/json/v2 (and encoding/json/jsontext) is available in Go 1.25/1.26
// only under GOEXPERIMENT=jsonv2 and is targeted to become the default,
// importable-without-a-flag package in a later Go release. Until then this
// package delegates to encoding/json (v1), so the default build is unchanged
// and compiles on Go 1.25+ with no build flags. When json/v2 is stable, swap
// the bodies here (optionally opting into stricter behavior such as
// RejectDuplicateNames) — see MIGRATION.md.
//
// It does not cover the easyjson-generated struct codecs (Request, Response,
// Error, ID) or the two streaming batch-array decoders in the transport
// clients; those are separate, larger migration steps documented in
// MIGRATION.md.
package codec

import "encoding/json"

// Marshal encodes v to JSON. It mirrors encoding/json.Marshal.
func Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// Unmarshal decodes JSON data into v. It mirrors encoding/json.Unmarshal.
func Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// Valid reports whether data is well-formed JSON. It mirrors
// encoding/json.Valid.
func Valid(data []byte) bool {
	return json.Valid(data)
}
