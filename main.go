package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

// JSONRPC is the core container for JSON-RPC 2.0 method dispatch, error handling,
// and interceptor chains.
type JSONRPC struct {
	mu                 sync.RWMutex
	errors             ErrorMessages
	methods            RPCMethods
	globalInterceptors InterceptorCallMethods
	defaultTimeOut     time.Duration
}

// New creates a new JSONRPC instance with default error messages and a 30-second timeout.
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
		defaultTimeOut:     30 * time.Second,
	}
}

// SetDefaultTimeOut sets the timeout duration for method execution.
func (j *JSONRPC) SetDefaultTimeOut(timeout time.Duration) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.defaultTimeOut = timeout
}

// call dispatches a method from the registry, running interceptors first.
func (j *JSONRPC) call(
	ctx context.Context,
	methodName string,
	data json.RawMessage,
	id any,
) (resp *structs.Response) {
	defer func() {
		if r := recover(); r != nil {
			resp = j.Error(ctx, fmt.Errorf("%v", r), InternalErrorCode, id)
		}
	}()

	ctx, code, err := j.callGlobalInterceptors(ctx, methodName, data, id)
	if err != nil {
		return j.Error(ctx, err, code, id)
	}

	j.mu.RLock()
	method, ok := j.methods[methodName]
	j.mu.RUnlock()

	if !ok {
		return j.Error(ctx, nil, MethodNotFoundErrorCode, id)
	}
	res, errCode, err := method(ctx, data)
	if err != nil {
		return j.Error(ctx, err, errCode, id)
	}

	resp = NewResponse(id, &res, nil)
	return resp
}
