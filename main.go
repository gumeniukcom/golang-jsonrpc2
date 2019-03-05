package jsonrpc

import (
	"context"
	"encoding/json"

	"github.com/gumeniukcom/golang-jsonrpc2/structs"
)

//JSONRPC container for jsonrpc
type JSONRPC struct {
	errors  ErrorMessages
	methods RPCMethods
}

// New creates new instance of JSONRPC
func New() *JSONRPC {

	return &JSONRPC{
		errors: ErrorMessages{
			ParseErrorCode:          "parse error. not well formed",
			InvalidRequestErrorCode: "invalid Request, not conforming to spec",
			MethodNotFoundErrorCode: "requested method not found",
			InvalidParamsErrorCode:  "invalid method parameterss",
			InternalErrorCode:       "internal error",
		},
		methods: RPCMethods{},
	}
}

// call method for call function from registry
// TODO: think about validate income data
func (j *JSONRPC) call(
	ctx context.Context,
	methodName string,
	data json.RawMessage,
	id interface{},
) *structs.Response {
	method, ok := j.methods[methodName]
	if !ok {
		return j.NewError(ctx, nil, MethodNotFoundErrorCode, id)
	}
	res, errCode, err := method(ctx, data)
	if err != nil {
		return j.NewError(ctx, err, errCode, id)
	}

	return newResponse(id, &res, nil)
}

func newResponse(
	id interface{},
	data *json.RawMessage,
	error *structs.Error,
) *structs.Response {
	return &structs.Response{
		Version: JSONRPCVersion,
		Result:  data,
		Error:   error,
		ID:      id,
	}
}
