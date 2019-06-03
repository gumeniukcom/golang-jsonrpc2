package jsonrpc

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gumeniukcom/golang-jsonrpc2/structs"
)

//JSONRPC container for jsonrpc
type JSONRPC struct {
	errors             ErrorMessages
	methods            RPCMethods
	globalInterceptors InterceptorCallMethods
	defaultTimeOut     time.Duration
}

// New creates new instance of JSONRPC
func New() *JSONRPC {

	return &JSONRPC{
		errors: ErrorMessages{
			ParseErrorCode:          "parse_error_not_well_formed",
			InvalidRequestErrorCode: "invalid_request_not_conforming_to_spec",
			MethodNotFoundErrorCode: "requested_method_not_found",
			InvalidParamsErrorCode:  "invalid_method_parameters",
			InternalErrorCode:       "internal_error",
			MethodNotImplemented:    "method_not_implemented",
			RequestTimeLimit:        "request_time_limit",
		},
		methods:            RPCMethods{},
		globalInterceptors: InterceptorCallMethods{},
		defaultTimeOut:     30,
	}
}

//SetDefaultTimeOut set timeout for func run
func (j *JSONRPC) SetDefaultTimeOut(timeout int) {
	j.defaultTimeOut = time.Duration(timeout)
}

// call method for call function from registry
// TODO: think about validate income data
func (j *JSONRPC) call(
	ctx context.Context,
	methodName string,
	data json.RawMessage,
	id interface{},
) *structs.Response {
	ctx, code, err := j.callGlobalInterceptors(ctx, methodName, data, id)
	if err != nil {
		return j.NewError(ctx, err, code, id)
	}

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
