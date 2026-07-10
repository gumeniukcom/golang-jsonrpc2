package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

// DefaultMaxBatchSize is the batch size limit a New() instance starts with.
// Use SetMaxBatchSize to change it; zero disables the limit.
const DefaultMaxBatchSize = 100

// config is an immutable snapshot of the server configuration. The hot path
// loads it once per request with a single atomic read; setters and
// registration clone-and-swap it under JSONRPC.mu (copy-on-write), so no lock
// is ever taken while dispatching.
type config struct {
	errors             ErrorMessages
	methods            RPCMethods
	composed           RPCMethods // methods wrapped in middleware; == methods when no middleware
	methodInfo         map[string]MethodInfo
	globalInterceptors InterceptorCallMethods
	middlewares        []Middleware
	defaultTimeOut     time.Duration
	logger             *slog.Logger
	maxBatchSize       int
	batchConcurrency   int
	maxMessageSize     int
	enforcedTimeout    bool
}

// clone returns a copy whose maps and slices are safe to mutate without
// affecting the snapshot readers hold. MethodInfo values are copied shallowly:
// their inner slices/maps are shared between snapshots, which is safe only
// because registered MethodInfo is immutable (duplicates are rejected and
// Methods() hands out deep copies) — keep it that way.
func (c *config) clone() *config {
	next := *c
	next.errors = make(ErrorMessages, len(c.errors)+1)
	for k, v := range c.errors {
		next.errors[k] = v
	}
	next.methods = make(RPCMethods, len(c.methods)+1)
	for k, v := range c.methods {
		next.methods[k] = v
	}
	next.methodInfo = make(map[string]MethodInfo, len(c.methodInfo)+1)
	for k, v := range c.methodInfo {
		next.methodInfo[k] = v
	}
	next.globalInterceptors = append(InterceptorCallMethods(nil), c.globalInterceptors...)
	next.middlewares = append([]Middleware(nil), c.middlewares...)
	return &next
}

// JSONRPC is the core container for JSON-RPC 2.0 method dispatch, error
// handling, and interceptor chains.
type JSONRPC struct {
	mu  sync.Mutex // serializes configuration writers
	cfg atomic.Pointer[config]
}

// New creates a new JSONRPC instance with default error messages, a 30-second
// timeout, slog.Default() as the logger, and DoS-safe batch defaults: batches
// are capped at DefaultMaxBatchSize requests and execute on at most
// 4×GOMAXPROCS concurrent workers. Call SetMaxBatchSize(0) /
// SetBatchConcurrency(0) to restore the old unlimited behavior.
func New() *JSONRPC {
	j := &JSONRPC{}
	j.cfg.Store(&config{
		errors: ErrorMessages{
			ParseErrorCode:          "parse_error_not_well_formed",
			InvalidRequestErrorCode: "invalid_request_not_conforming_to_spec",
			MethodNotFoundErrorCode: "requested_method_not_found",
			InvalidParamsErrorCode:  "invalid_method_parameters",
			InternalErrorCode:       "internal_error",
			MethodNotImplemented:    "method_not_implemented",
			RequestTimeLimit:        "request_time_limit",
		},
		methods:          RPCMethods{},
		composed:         RPCMethods{},
		methodInfo:       map[string]MethodInfo{},
		defaultTimeOut:   30 * time.Second,
		logger:           slog.Default(),
		maxBatchSize:     DefaultMaxBatchSize,
		batchConcurrency: 4 * runtime.GOMAXPROCS(0),
	})
	return j
}

// updateConfig clones the current snapshot, applies mutate, and atomically
// installs the result. Returning an error from mutate aborts the swap. The
// composed dispatch map is carried over as-is — use updateRegistry for
// mutations that change methods or middleware.
func (j *JSONRPC) updateConfig(mutate func(*config) error) error {
	return j.update(mutate, false)
}

// updateRegistry is updateConfig for mutations that change the method
// registry or the middleware chain: it recomposes the dispatch map, running
// every middleware factory once per registered method.
func (j *JSONRPC) updateRegistry(mutate func(*config) error) error {
	return j.update(mutate, true)
}

func (j *JSONRPC) update(mutate func(*config) error, recompose bool) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	next := j.cfg.Load().clone()
	if err := mutate(next); err != nil {
		return err
	}
	if recompose {
		next.compose()
	}
	j.cfg.Store(next)
	return nil
}

// SetDefaultTimeOut sets the timeout duration for method execution.
func (j *JSONRPC) SetDefaultTimeOut(timeout time.Duration) {
	_ = j.updateConfig(func(c *config) error {
		c.defaultTimeOut = timeout
		return nil
	})
}

// SetLogger sets the logger used for server-side logging of handler errors.
// Pass nil to disable logging.
func (j *JSONRPC) SetLogger(logger *slog.Logger) {
	_ = j.updateConfig(func(c *config) error {
		c.logger = logger
		return nil
	})
}

// SetMaxBatchSize limits how many requests a single batch may contain.
// Batches above the limit are rejected with an invalid-request response.
// New() starts at DefaultMaxBatchSize; zero disables the limit.
func (j *JSONRPC) SetMaxBatchSize(n int) {
	_ = j.updateConfig(func(c *config) error {
		c.maxBatchSize = n
		return nil
	})
}

// SetBatchConcurrency limits how many requests of a batch execute
// concurrently. New() starts at 4×GOMAXPROCS; zero means unlimited — one
// goroutine per batch entry.
func (j *JSONRPC) SetBatchConcurrency(n int) {
	_ = j.updateConfig(func(c *config) error {
		c.batchConcurrency = n
		return nil
	})
}

// SetMaxMessageSize rejects raw messages larger than n bytes with a
// "request_too_large" invalid-request response before any parsing happens.
// Zero (the default) disables the check. This bounds parser work for a single
// message; prefer additionally capping the body at the transport layer
// (e.g. http.MaxBytesReader) so oversized payloads are not even read into
// memory.
func (j *JSONRPC) SetMaxMessageSize(n int) {
	_ = j.updateConfig(func(c *config) error {
		c.maxMessageSize = n
		return nil
	})
}

// SetEnforcedTimeout controls how the per-request timeout is delivered.
//
// Disabled (the default), the handler runs inline on the caller's goroutine:
// no goroutine, channel, or scheduler handoff per request. The time-limit
// error response is produced when the deadline has expired by the time the
// handler returns — a handler that ignores ctx.Done() delays the (still
// time-limit) response until it returns.
//
// Enabled, every call runs in its own goroutine and the time-limit response
// is returned as soon as the deadline expires, even if the handler keeps
// running in the background. That guarantee costs a goroutine + channel per
// request and lets handlers that ignore ctx accumulate without bound.
//
// In both modes cancellation of the parent context is NOT a time limit: a
// completed handler keeps its response. Only when enforced mode aborts a
// still-running call on parent cancellation does the client get the
// request_time_limit code too — with the real cause in the server-side log.
func (j *JSONRPC) SetEnforcedTimeout(enabled bool) {
	_ = j.updateConfig(func(c *config) error {
		c.enforcedTimeout = enabled
		return nil
	})
}

// call dispatches a method from the registry, running interceptors first.
func (j *JSONRPC) call(
	ctx context.Context,
	cfg *config,
	methodName string,
	data json.RawMessage,
	id any,
) (resp *structs.Response) {
	defer func() {
		if r := recover(); r != nil {
			resp = j.errorResponse(ctx, cfg, fmt.Errorf("%v", r), InternalErrorCode, id)
		}
	}()

	ctx, code, err := callGlobalInterceptors(ctx, cfg, methodName, data, id)
	if err != nil {
		return j.errorResponse(ctx, cfg, err, code, id)
	}

	method, ok := cfg.composed[methodName]
	if !ok {
		return j.errorResponse(ctx, cfg, nil, MethodNotFoundErrorCode, id)
	}
	res, errCode, err := method(ctx, data)
	if err != nil {
		return j.errorResponse(ctx, cfg, err, errCode, id)
	}

	resp = NewResponse(id, &res, nil)
	return resp
}
