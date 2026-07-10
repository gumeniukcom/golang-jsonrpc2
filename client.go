package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gumeniukcom/golang-jsonrpc2/v2/internal/codec"
)

// Caller is the client-side contract implemented by the transport clients
// (jsonrpchttp.Client, jsonrpcws.Client).
//
// Call issues a request and returns the raw result. A JSON-RPC error
// response is returned as *structs.Error (match with errors.As); transport
// failures are ordinary errors. Notify sends a notification — no id, no
// response, no server-side error reporting.
type Caller interface {
	Call(ctx context.Context, method string, params any) (json.RawMessage, error)
	Notify(ctx context.Context, method string, params any) error
}

// CallResult calls method via c and unmarshals the result into R,
// complementing the server-side Typed/RegisterTyped adapters.
func CallResult[R any](ctx context.Context, c Caller, method string, params any) (R, error) {
	var result R
	raw, err := c.Call(ctx, method, params)
	if err != nil {
		return result, err
	}
	if len(raw) == 0 {
		return result, nil
	}
	if err := codec.Unmarshal(raw, &result); err != nil {
		return result, fmt.Errorf("unmarshal result: %w", err)
	}
	return result, nil
}

// MarshalParams converts convenience params values to raw JSON: nil stays
// nil (the params member is omitted), json.RawMessage passes through
// unchanged, anything else goes through encoding/json.
func MarshalParams(params any) (json.RawMessage, error) {
	switch v := params.(type) {
	case nil:
		return nil, nil
	case json.RawMessage:
		return v, nil
	default:
		raw, err := codec.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshal params: %w", err)
		}
		return raw, nil
	}
}
