package structs

import "encoding/json"

// Response represents a JSON-RPC 2.0 response object.
// See https://www.jsonrpc.org/specification#response_object
type Response struct {
	Version string           `json:"jsonrpc"`
	Result  *json.RawMessage `json:"result,omitempty"`
	Error   *Error           `json:"error,omitempty"`
	ID      any              `json:"id"`
}

// Error represents a JSON-RPC 2.0 error object.
// See https://www.jsonrpc.org/specification#error_object
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// BatchFullResponse is a batch of JSON-RPC responses.
//
//easyjson:json
type BatchFullResponse []Response
