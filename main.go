package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
	methodInfo         map[string]MethodInfo
	globalInterceptors InterceptorCallMethods
	defaultTimeOut     time.Duration
	logger             *slog.Logger
	maxBatchSize       int
	batchConcurrency   int
}

// New creates a new JSONRPC instance with default error messages, a 30-second
// timeout, slog.Default() as the logger, and no batch limits.
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
		methodInfo:         map[string]MethodInfo{},
		globalInterceptors: InterceptorCallMethods{},
		defaultTimeOut:     30 * time.Second,
		logger:             slog.Default(),
	}
}

// SetDefaultTimeOut sets the timeout duration for method execution.
func (j *JSONRPC) SetDefaultTimeOut(timeout time.Duration) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.defaultTimeOut = timeout
}

// SetLogger sets the logger used for server-side logging of handler errors.
// Pass nil to disable logging.
func (j *JSONRPC) SetLogger(logger *slog.Logger) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.logger = logger
}

// SetMaxBatchSize limits how many requests a single batch may contain.
// Batches above the limit are rejected with an invalid-request response.
// Zero (the default) means unlimited.
func (j *JSONRPC) SetMaxBatchSize(n int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.maxBatchSize = n
}

// SetBatchConcurrency limits how many requests of a batch execute
// concurrently. Zero (the default) means unlimited — one goroutine per
// batch entry.
func (j *JSONRPC) SetBatchConcurrency(n int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.batchConcurrency = n
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
