package jsonrpc

import (
	"encoding/json"

	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

// NewResponse creates a new JSON-RPC response.
func NewResponse(
	id any,
	data *json.RawMessage,
	rpcErr *structs.Error,
) *structs.Response {
	return &structs.Response{
		Version: Version,
		Result:  data,
		Error:   rpcErr,
		ID:      id,
	}
}
