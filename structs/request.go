package structs

import (
	"encoding/json"
)

// Request represents a JSON-RPC 2.0 request object.
// See https://www.jsonrpc.org/specification#request_object
//
// A nil ID means the id member was absent: the request is a notification.
// The (un)marshaling code is hand-written in request_codec.go (easyjson:skip)
// because the generator's null handling would collapse a present "id":null
// into an absent id, erasing the notification distinction.
//
//easyjson:skip
type Request struct {
	Version string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      ID              `json:"id"`
}

// Requests is a batch of JSON-RPC requests.
//
//easyjson:json
type Requests []Request
