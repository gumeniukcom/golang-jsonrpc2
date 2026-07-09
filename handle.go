package jsonrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

// HandleRPCJSONRawMessage parses raw JSON, dispatches single or batch requests,
// and returns the serialized JSON-RPC response.
//
// Oversized messages (SetMaxMessageSize) and oversized batches
// (SetMaxBatchSize) are rejected BEFORE the payload is unmarshaled, so the
// limits actually bound parsing work. The message-size check does not bound
// raw body size — cap that at the transport layer too (e.g.
// http.MaxBytesReader), so oversized payloads are not even read into memory.
func (j *JSONRPC) HandleRPCJSONRawMessage(ctx context.Context, data json.RawMessage) json.RawMessage {
	cfg := j.cfg.Load()

	reqLen := len(data)
	if reqLen < 2 {
		return errorInvalidRequest()
	}

	if cfg.maxMessageSize > 0 && reqLen > cfg.maxMessageSize {
		// Debug, not Warn: client-caused rejects must not let a flood of bad
		// requests spam the log (same policy as levelForCode).
		if cfg.logger != nil {
			cfg.logger.DebugContext(ctx, "jsonrpc: request too large",
				slog.Int("max", cfg.maxMessageSize), slog.Int("got", reqLen))
		}
		return errorRequestTooLarge()
	}

	if data[0] == '[' && data[reqLen-1] == ']' {
		if cfg.maxBatchSize > 0 {
			if n := approxBatchLen(data, cfg.maxBatchSize); n > cfg.maxBatchSize {
				if cfg.logger != nil {
					cfg.logger.DebugContext(ctx, "jsonrpc: batch too large",
						slog.Int("max", cfg.maxBatchSize), slog.Int("got_at_least", n))
				}
				return errorBatchTooLarge()
			}
		}

		var batchReq structs.Requests
		if err := batchReq.UnmarshalJSON(data); err != nil {
			return errorInvalidRequest()
		}
		if len(batchReq) == 0 {
			return errorInvalidRequest()
		}
		batchResp := j.handleBatchRPC(ctx, cfg, batchReq)
		batchRespRaw, err := batchResp.MarshalJSON()
		if err != nil {
			// One unmarshalable error.data payload must not destroy the whole
			// batch: replace only the broken entries (keeping their ids) and
			// re-marshal.
			j.logMarshalFailure(ctx, cfg, err)
			for i := range batchResp {
				if _, merr := batchResp[i].MarshalJSON(); merr != nil {
					batchResp[i] = *safeInternalResponse(cfg, batchResp[i].ID)
				}
			}
			batchRespRaw, err = batchResp.MarshalJSON()
			if err != nil {
				return errorInternal()
			}
		}
		return batchRespRaw
	} else if data[0] == '{' && data[reqLen-1] == '}' {
		var req structs.Request
		if err := req.UnmarshalJSON(data); err != nil {
			return errorInvalidRequest()
		}
		resp := j.handleRPC(ctx, cfg, &req)
		respRaw, err := resp.MarshalJSON()
		if err != nil {
			j.logMarshalFailure(ctx, cfg, err)
			respRaw, err = safeInternalResponse(cfg, req.ID).MarshalJSON()
			if err != nil {
				return errorInternal()
			}
		}
		return respRaw
	}

	return errorInvalidRequest()
}

// approxBatchLen counts top-level array elements of a JSON array without
// unmarshaling it, returning early once the count exceeds limit. For valid
// JSON the count is exact; malformed input may miscount, which is fine — it
// is only used to reject oversized batches cheaply, and anything under the
// limit still goes through real unmarshaling.
func approxBatchLen(data []byte, limit int) int {
	depth, count := 0, 0
	inString, escaped, seen := false, false, false

	for i := 0; i < len(data); i++ {
		b := data[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case b == '\\':
				escaped = true
			case b == '"':
				inString = false
			}
			continue
		}
		switch b {
		case '"':
			inString = true
			if depth == 1 {
				seen = true
			}
		case '[', '{':
			if depth == 1 {
				seen = true
			}
			depth++
		case ']', '}':
			depth--
		case ',':
			if depth == 1 {
				count++
				if count+1 > limit {
					return count + 1
				}
			}
		case ' ', '\t', '\n', '\r':
		default:
			if depth == 1 {
				seen = true
			}
		}
	}
	if seen {
		count++
	}
	return count
}

// safeInternalResponse builds an internal_error response with the given id
// that is guaranteed to marshal (its data field is nil).
func safeInternalResponse(cfg *config, id any) *structs.Response {
	return newError(cfg.errors[InternalErrorCode], InternalErrorCode, nil, id)
}

func (j *JSONRPC) logMarshalFailure(ctx context.Context, cfg *config, err error) {
	if cfg.logger != nil {
		cfg.logger.ErrorContext(ctx, "jsonrpc: response marshal failed", slog.Any("error", err))
	}
}

// HandleRPC executes a single JSON-RPC request with the configured timeout.
func (j *JSONRPC) HandleRPC(ctx context.Context, data *structs.Request) *structs.Response {
	return j.handleRPC(ctx, j.cfg.Load(), data)
}

// handleRPC validates and executes one request against a config snapshot.
//
// By default the handler runs inline on the caller's goroutine and the
// time-limit error is produced when the deadline has already expired by the
// time the handler returns. With SetEnforcedTimeout the call is delegated to
// a goroutine so the timeout response is delivered at the deadline no matter
// what the handler does.
//
// Only a genuine deadline expiry becomes RequestTimeLimit: cancellation of
// the parent context (client disconnect, shutdown) keeps the handler's own
// response — inline it is already computed, and in enforced mode the caller
// has stopped listening anyway, so the abort is reported distinctly.
func (j *JSONRPC) handleRPC(ctx context.Context, cfg *config, data *structs.Request) *structs.Response {
	if err := validateRequest(data); err != nil {
		return j.errorResponse(ctx, cfg, err, InvalidRequestErrorCode, data.ID)
	}

	ctxt, cancel := context.WithTimeout(ctx, cfg.defaultTimeOut)
	defer cancel()

	if cfg.enforcedTimeout {
		c := make(chan *structs.Response, 1)
		go func() {
			c <- j.call(ctxt, cfg, data.Method, data.Params, data.ID)
		}()
		select {
		case resp := <-c:
			return resp
		case <-ctxt.Done():
			// Prefer a response that is already ready over the abort path.
			select {
			case resp := <-c:
				return resp
			default:
			}
			if errors.Is(ctxt.Err(), context.DeadlineExceeded) {
				return j.errorResponse(ctx, cfg,
					fmt.Errorf("method %q took too long", data.Method), RequestTimeLimit, data.ID)
			}
			// Parent canceled before the handler finished; nobody is
			// listening for this response, but keep the server-side log
			// truthful about why the call was abandoned.
			return j.errorResponse(ctx, cfg,
				fmt.Errorf("method %q aborted: %w", data.Method, context.Cause(ctxt)), RequestTimeLimit, data.ID)
		}
	}

	resp := j.call(ctxt, cfg, data.Method, data.Params, data.ID)
	if errors.Is(ctxt.Err(), context.DeadlineExceeded) {
		return j.errorResponse(ctx, cfg,
			fmt.Errorf("method %q took too long", data.Method), RequestTimeLimit, data.ID)
	}
	return resp
}

// HandleBatchRPC executes a batch of JSON-RPC requests on a worker pool of
// SetBatchConcurrency workers (4×GOMAXPROCS by default, unlimited when set to
// zero), so goroutine count is bounded by the concurrency setting, not the
// batch length. Responses keep the order of the requests.
//
// With enforced timeouts the bound counts started requests: a handler that
// ignores context cancellation after its timeout keeps running in the
// background and no longer occupies a worker slot. In the default inline mode
// the worker itself runs the handler, so the bound holds strictly.
func (j *JSONRPC) HandleBatchRPC(ctx context.Context, data structs.Requests) structs.BatchFullResponse {
	return j.handleBatchRPC(ctx, j.cfg.Load(), data)
}

func (j *JSONRPC) handleBatchRPC(ctx context.Context, cfg *config, data structs.Requests) structs.BatchFullResponse {
	concurrency := cfg.batchConcurrency
	if concurrency <= 0 || concurrency > len(data) {
		concurrency = len(data)
	}

	responses := make(structs.BatchFullResponse, len(data))
	indexes := make(chan int)
	wg := sync.WaitGroup{}

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range indexes {
				responses[idx] = *j.handleRPC(ctx, cfg, &data[idx])
			}
		}()
	}

	for idx := range data {
		indexes <- idx
	}
	close(indexes)
	wg.Wait()

	return responses
}

func validateRequest(data *structs.Request) error {
	if data.Version != Version {
		return errors.New("not valid version")
	}

	if data.Method == "" {
		return errors.New("method is required")
	}

	return nil
}
