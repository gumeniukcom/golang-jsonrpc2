package structs

import "encoding/json"

// Response struct for full jsonrpc response
// @see https://www.jsonrpc.org/specification#response_object
type Response struct {
	Version string           `json:"jsonrpc"`
	Result  *json.RawMessage `json:"result,omitempty"`
	Error   *Error           `json:"error,omitempty"`
	ID      interface{}      `json:"id"`
}

// Error struct for JSONRPC2 error
// @see https://www.jsonrpc.org/specification#error_object
type Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// BatchFullResponse is type for batch response
//easyjson:json
type BatchFullResponse []Response
