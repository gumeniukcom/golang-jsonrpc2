package structs

import (
	"encoding/json"
	"strconv"
)

// Response represents a JSON-RPC 2.0 response object.
// See https://www.jsonrpc.org/specification#response_object
type Response struct {
	Version string           `json:"jsonrpc"`
	Result  *json.RawMessage `json:"result,omitempty"`
	Error   *Error           `json:"error,omitempty"`
	ID      ID               `json:"id"`
}

// Error represents a JSON-RPC 2.0 error object.
// See https://www.jsonrpc.org/specification#error_object
//
// The (un)marshaling is hand-written in error_codec.go (easyjson:skip): the
// generated decoder read a present "data" member into an any via a recursive,
// depth-unbounded lexer, so a hostile peer could crash the process with a
// deeply nested "data" (unrecoverable stack overflow). The hand-written
// decoder captures "data" as raw bytes instead. On decode Data therefore
// holds a json.RawMessage; unmarshal it into a concrete type yourself.
//
//easyjson:skip
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Error implements the error interface, so clients can return a JSON-RPC
// error object directly and callers can match it with errors.As.
func (e *Error) Error() string {
	return "jsonrpc: " + e.Message + " (code " + strconv.Itoa(e.Code) + ")"
}

// BatchFullResponse is a batch of JSON-RPC responses.
//
//easyjson:json
type BatchFullResponse []Response
