// Package codec is the single seam through which the library performs generic,
// reflection-based JSON (de)serialization — method params and results, client
// payloads, arbitrary Error.Data values, and structural validation.
// Consolidating these calls here keeps a migration to encoding/json/v2 a
// one-file change instead of one that touches every call site.
//
// Go 1.27 reimplemented encoding/json on top of encoding/json/v2, so builds on
// Go 1.27+ already get v2's faster unmarshal through the calls below — with no
// source change here and no go.mod floor bump, because the switch is a build
// tag (goexperiment.jsonv2) rather than a language-version gate. This package
// therefore stays on the v1 API: importing encoding/json/v2 directly would
// require a floor of Go 1.27, and the module's declared floor is Go 1.26.
//
// Switching these bodies to encoding/json/v2 is NOT a drop-in change. v2's
// defaults differ from v1 in ways that are silent and wire-visible: a
// case-mismatched member decodes to the zero value with no error, duplicate
// members become an error, and nil slices and maps marshal as [] and {}
// instead of null. Any flip must pass json.DefaultOptionsV1() to stay
// bug-for-bug compatible; adopting the stricter defaults is a breaking change
// that belongs in a major version. See docs/dev/json-v2-plan.md.
//
// It does not cover the easyjson-generated struct codecs (Request, Response,
// Error, ID) or the two streaming batch-array decoders in the transport
// clients; those are separate, larger migration steps documented in
// docs/dev/json-v2-plan.md.
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
