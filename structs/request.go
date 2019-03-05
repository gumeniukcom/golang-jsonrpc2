package structs

import (
	"encoding/json"
)

// Request impementation of jsonrpc request
// @see https://www.jsonrpc.org/specification#request_object
type Request struct {
	Version string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      interface{}     `json:"id"`
}

// Requests for batch request
//easyjson:json
type Requests []Request
