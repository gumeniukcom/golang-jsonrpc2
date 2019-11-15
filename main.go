package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
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
) (resp *structs.Response) {
	defer func() {
		if r := recover(); r != nil {
			resp = j.Error(ctx, fmt.Errorf("%#v", r), InternalErrorCode, id)
		}
	}()

	ctx, code, err := j.callGlobalInterceptors(ctx, methodName, data, id)
	if err != nil {
		return j.Error(ctx, err, code, id)
	}

	method, ok := j.methods[methodName]
	if !ok {
		return j.Error(ctx, nil, MethodNotFoundErrorCode, id)
	}
	res, errCode, err := method(ctx, data)
	if err != nil {
		return j.Error(ctx, err, errCode, id)
	}

	resp = Response(ctx, id, &res, nil)
	return resp
}
