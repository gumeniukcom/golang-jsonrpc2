package jsonrpc

import (
	"context"
	"time"

	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

// CallInfo is the outcome of one dispatched request, passed to an ObserveFunc.
type CallInfo struct {
	// Method is the requested method name (empty when the request failed
	// validation before a method could be read). It comes straight from the
	// client and is untrusted — escape it before writing to a plain-text log
	// sink to avoid log injection.
	Method string
	// Code is the client-facing outcome code: OK on success, otherwise the
	// error code the client received (including RequestTimeLimit on timeout
	// and MethodNotFoundErrorCode for unknown methods).
	Code int
	// Err is the client-facing error (a *structs.Error) or nil on success. It
	// carries the code and public message, not the server-side detail — use
	// SetLogger for that.
	Err error
	// Duration is the wall-clock time spent dispatching the request, measured
	// around validation and method execution (in inline timeout mode it can
	// exceed the deadline when a handler ignores cancellation).
	Duration time.Duration
	// Notification is true when the request carried no id, so no response was
	// sent to the client even though the handler ran.
	Notification bool
}

// ObserveFunc is called once per dispatched request with its outcome, for
// metrics, tracing, or request logging. Unlike a Middleware — which wraps a
// registered handler — it runs on the dispatch path, so it observes every
// request that reaches dispatch: method-not-found, requests that fail
// validation, interceptor aborts, panics, and timeouts that never reach a
// handler. In a batch it fires once per entry, including entries whose JSON
// did not decode.
//
// It does NOT observe frame-level rejects that never become a request object:
// oversized messages/batches (SetMaxMessageSize / SetMaxBatchSize), top-level
// JSON parse errors, and a single request that fails to decode are all
// answered before dispatch. Those are logged at Debug (SetLogger); meter them
// at the transport layer if you need them.
//
// It runs synchronously on the request goroutine, so it must be cheap and
// non-blocking; offload slow work (network export) to another goroutine. In a
// batch, entries run on separate worker goroutines (SetBatchConcurrency), so
// the observer may be called concurrently — make it safe for concurrent use.
// A panic in the observer is recovered and logged rather than allowed to
// crash the request goroutine (or, in a batch, the process).
type ObserveFunc func(ctx context.Context, info CallInfo)

// SetObserver installs an observer called once per dispatched request. Pass
// nil to disable. Registering an observer adds no per-request cost when unset.
func (j *JSONRPC) SetObserver(fn ObserveFunc) {
	_ = j.updateConfig(func(c *config) error {
		c.observe = fn
		return nil
	})
}

// observeOutcome extracts the client-facing code/error from a response.
func observeOutcome(resp *structs.Response) (int, error) {
	if resp != nil && resp.Error != nil {
		return resp.Error.Code, resp.Error
	}
	return OK, nil
}
