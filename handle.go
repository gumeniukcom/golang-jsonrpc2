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
// When SetMaxBatchSize is configured, oversized batches are rejected with a
// "batch_too_large" invalid-request response BEFORE the payload is
// unmarshaled, so the limit actually bounds parsing work. Note it does not
// bound raw body size — cap that at the transport layer.
func (j *JSONRPC) HandleRPCJSONRawMessage(ctx context.Context, data json.RawMessage) json.RawMessage {
	reqLen := len(data)
	if reqLen < 2 {
		return errorInvalidRequest()
	}

	if data[0] == '[' && data[reqLen-1] == ']' {
		j.mu.RLock()
		maxBatch := j.maxBatchSize
		logger := j.logger
		j.mu.RUnlock()

		if maxBatch > 0 {
			if n := approxBatchLen(data, maxBatch); n > maxBatch {
				if logger != nil {
					logger.WarnContext(ctx, "jsonrpc: batch too large",
						slog.Int("max", maxBatch), slog.Int("got_at_least", n))
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
		batchResp := j.HandleBatchRPC(ctx, batchReq)
		batchRespRaw, err := batchResp.MarshalJSON()
		if err != nil {
			// One unmarshalable error.data payload must not destroy the whole
			// batch: replace only the broken entries (keeping their ids) and
			// re-marshal.
			j.logMarshalFailure(ctx, err)
			for i := range batchResp {
				if _, merr := batchResp[i].MarshalJSON(); merr != nil {
					batchResp[i] = *j.safeInternalResponse(batchResp[i].ID)
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
		resp := j.HandleRPC(ctx, &req)
		respRaw, err := resp.MarshalJSON()
		if err != nil {
			j.logMarshalFailure(ctx, err)
			respRaw, err = j.safeInternalResponse(req.ID).MarshalJSON()
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
func (j *JSONRPC) safeInternalResponse(id any) *structs.Response {
	j.mu.RLock()
	msg := j.errors[InternalErrorCode]
	j.mu.RUnlock()
	return newError(msg, InternalErrorCode, nil, id)
}

func (j *JSONRPC) logMarshalFailure(ctx context.Context, err error) {
	j.mu.RLock()
	logger := j.logger
	j.mu.RUnlock()
	if logger != nil {
		logger.ErrorContext(ctx, "jsonrpc: response marshal failed", slog.Any("error", err))
	}
}

// HandleRPC executes a single JSON-RPC request with the configured timeout.
func (j *JSONRPC) HandleRPC(ctx context.Context, data *structs.Request) *structs.Response {
	j.mu.RLock()
	timeout := j.defaultTimeOut
	j.mu.RUnlock()

	ctxt, canc := context.WithTimeout(ctx, timeout)
	defer canc()

	c := make(chan *structs.Response, 1)

	go j.handleRPC(ctxt, data, c)

	select {
	case <-ctxt.Done():
		return j.Error(ctx, fmt.Errorf("method %q took too long", data.Method), RequestTimeLimit, data.ID)
	case resp := <-c:
		return resp
	}
}

func (j *JSONRPC) handleRPC(ctx context.Context, data *structs.Request, c chan *structs.Response) {
	if err := validateRequest(data); err != nil {
		c <- j.Error(ctx, err, InvalidRequestErrorCode, data.ID)
		return
	}
	c <- j.call(ctx, data.Method, data.Params, data.ID)
}

// HandleBatchRPC executes a batch of JSON-RPC requests on a worker pool of
// SetBatchConcurrency workers (unlimited when unset), so goroutine count is
// bounded by the concurrency setting, not the batch length. Responses keep
// the order of the requests.
//
// The bound counts started requests: a handler that ignores context
// cancellation after its timeout keeps running in the background and no
// longer occupies a worker slot.
func (j *JSONRPC) HandleBatchRPC(ctx context.Context, data structs.Requests) structs.BatchFullResponse {
	j.mu.RLock()
	concurrency := j.batchConcurrency
	j.mu.RUnlock()

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
				responses[idx] = *j.HandleRPC(ctx, &data[idx])
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
