package jsonrpc

import (
	"encoding/json"
	"math"
	"strconv"

	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

// NewResponse creates a new JSON-RPC response. id accepts the raw
// structs.ID of a request as well as plain Go values (string, integers,
// float64) for convenience; anything else is marshaled with encoding/json.
func NewResponse(
	id any,
	data *json.RawMessage,
	rpcErr *structs.Error,
) *structs.Response {
	return &structs.Response{
		Version: Version,
		Result:  data,
		Error:   rpcErr,
		ID:      toID(id),
	}
}

// toID converts convenience id values to raw JSON bytes without reflection
// for the common types; the default case falls back to encoding/json.
func toID(id any) structs.ID {
	switch v := id.(type) {
	case nil:
		return nil
	case structs.ID:
		return v
	case json.RawMessage:
		return structs.ID(v)
	case string:
		b, err := json.Marshal(v)
		if err != nil {
			return structs.ID("null")
		}
		return structs.ID(b)
	case int:
		return structs.ID(strconv.AppendInt(nil, int64(v), 10))
	case int64:
		return structs.ID(strconv.AppendInt(nil, v, 10))
	case float64:
		// NaN and ±Inf are not JSON tokens and would corrupt the response.
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return structs.ID("null")
		}
		return structs.ID(strconv.AppendFloat(nil, v, 'g', -1, 64))
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return structs.ID("null")
		}
		return structs.ID(b)
	}
}
