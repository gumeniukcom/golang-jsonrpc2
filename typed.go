package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/gumeniukcom/golang-jsonrpc2/v2/internal/codec"
)

// Typed wraps a strongly-typed handler into an RPCMethod, removing the
// json.RawMessage boilerplate: params are unmarshaled into P and the result R
// is marshaled back. Absent params yield the zero value of P, and so does an
// empty positional list ("params": []) when P is not itself list-shaped —
// many clients spell "no parameters" that way (the OpenRPC spec's own
// rpc.discover definition among them), and rejecting it with -32602 would
// punish a conformant caller of a zero-parameter method.
//
// Handler errors are interpreted by JSONRPC.Error: a *RPCError anywhere in
// the chain supplies the response code and client-visible data; any other
// error maps to InternalErrorCode.
func Typed[P any, R any](fn func(ctx context.Context, params P) (R, error)) RPCMethod {
	return func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		var params P
		if len(data) > 0 {
			if err := codec.Unmarshal(data, &params); err != nil {
				// "params": [] into a non-list P means "no positional
				// params" — treat it like absent params (zero value), the
				// same meaning "params": {} already gets. Only the
				// empty-list spelling is forgiven; a non-empty array into a
				// struct stays an invalid-params error.
				if isEmptyJSONArray(data) {
					params = *new(P)
				} else {
					return nil, InvalidParamsErrorCode, fmt.Errorf("unmarshal params: %w", err)
				}
			}
		}

		result, err := fn(ctx, params)
		if err != nil {
			return nil, InternalErrorCode, err
		}

		raw, err := codec.Marshal(result)
		if err != nil {
			return nil, InternalErrorCode, fmt.Errorf("marshal result: %w", err)
		}

		return raw, OK, nil
	}
}

// isEmptyJSONArray reports whether data is exactly the JSON document [],
// modulo insignificant whitespace.
func isEmptyJSONArray(data []byte) bool {
	seen := 0 // 0 = expecting '[', 1 = expecting ']'
	for _, b := range data {
		switch b {
		case ' ', '\t', '\n', '\r':
		case '[':
			if seen != 0 {
				return false
			}
			seen = 1
		case ']':
			if seen != 1 {
				return false
			}
			seen = 2
		default:
			return false
		}
	}
	return seen == 2
}

// RegisterTyped registers a strongly-typed handler under the given method name.
// It captures the reflect types of P and R and any documentation metadata from
// opts into the introspectable registry (see MethodInfo and Methods), so the
// server can describe itself for schema/documentation generation.
func RegisterTyped[P any, R any](
	j *JSONRPC,
	name string,
	fn func(ctx context.Context, params P) (R, error),
	opts ...MethodOption,
) error {
	info := MethodInfo{
		Name:   name,
		Params: reflect.TypeOf((*P)(nil)).Elem(),
		Result: reflect.TypeOf((*R)(nil)).Elem(),
	}
	for _, opt := range opts {
		opt(&info)
	}
	return j.registerMethod(name, Typed(fn), info)
}
