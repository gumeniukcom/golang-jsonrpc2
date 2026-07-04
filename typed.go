package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
)

// Typed wraps a strongly-typed handler into an RPCMethod, removing the
// json.RawMessage boilerplate: params are unmarshaled into P and the result R
// is marshaled back. Absent params yield the zero value of P.
//
// Handler errors are interpreted by JSONRPC.Error: a *RPCError anywhere in
// the chain supplies the response code and client-visible data; any other
// error maps to InternalErrorCode.
func Typed[P any, R any](fn func(ctx context.Context, params P) (R, error)) RPCMethod {
	return func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		var params P
		if len(data) > 0 {
			if err := json.Unmarshal(data, &params); err != nil {
				return nil, InvalidParamsErrorCode, fmt.Errorf("unmarshal params: %w", err)
			}
		}

		result, err := fn(ctx, params)
		if err != nil {
			return nil, InternalErrorCode, err
		}

		raw, err := json.Marshal(result)
		if err != nil {
			return nil, InternalErrorCode, fmt.Errorf("marshal result: %w", err)
		}

		return raw, OK, nil
	}
}

// RegisterTyped registers a strongly-typed handler under the given method name.
func RegisterTyped[P any, R any](
	j *JSONRPC,
	name string,
	fn func(ctx context.Context, params P) (R, error),
) error {
	return j.RegisterMethod(name, Typed(fn))
}
