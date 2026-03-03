package structs

import (
	"encoding/json"
)

// Request represents a JSON-RPC 2.0 request object.
// See https://www.jsonrpc.org/specification#request_object
type Request struct {
	Version string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      any             `json:"id"`
}

// Requests is a batch of JSON-RPC requests.
//
//easyjson:json
type Requests []Request
