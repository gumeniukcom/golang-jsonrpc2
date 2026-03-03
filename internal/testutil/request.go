package testutil

import "encoding/json"

// RequestTestStruct is a test helper that implements ParamsDataMarshaler.
type RequestTestStruct struct {
	ID int64
}

// MarshalJSON implements the json.Marshaler interface.
func (v *RequestTestStruct) MarshalJSON() ([]byte, error) {
	return json.Marshal(*v)
}
