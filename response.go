package jsonrpc

import (
	"context"
	"encoding/json"

	"github.com/gumeniukcom/golang-jsonrpc2/structs"
)

//Response ...
func Response(
	ctx context.Context,
	id interface{},
	data *json.RawMessage,
	error *structs.Error,
) *structs.Response {
	return &structs.Response{
		Version: Version,
		Result:  data,
		Error:   error,
		ID:      id,
	}
}
