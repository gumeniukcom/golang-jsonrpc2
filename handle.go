package jsonrpc

import (
	"bytes"
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

	if cfg.maxMessageSize > 0 && len(data) > cfg.maxMessageSize {
		// Debug, not Warn: client-caused rejects must not let a flood of bad
		// requests spam the log (same policy as levelForCode).
		if cfg.logger != nil {
			cfg.logger.DebugContext(ctx, "jsonrpc: request too large",
				slog.Int("max", cfg.maxMessageSize), slog.Int("got", len(data)))
		}
		return errorRequestTooLarge()
	}

	data = bytes.TrimSpace(data)
	reqLen := len(data)
	if reqLen == 0 {
		return errorParse()
	}
	if reqLen < 2 {
		return errorForMalformed(data)
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

		// Decode entries individually: one undecodable entry must not
		// destroy the responses of its valid siblings (spec §6 examples) —
		// it gets its own -32600 entry with id:null instead.
		elems, ok := splitBatchElements(data)
		if !ok {
			return errorForMalformed(data)
		}
		if len(elems) == 0 {
			return errorInvalidRequest()
		}
		batchReq := make(structs.Requests, len(elems))
		parseFailed := make([]bool, len(elems))
		for i, e := range elems {
			if err := batchReq[i].UnmarshalJSON(e); err != nil {
				if !json.Valid(e) {
					// A syntactically broken element means the whole payload
					// is invalid JSON: a single ParseError response.
					return errorParse()
				}
				batchReq[i] = structs.Request{}
				parseFailed[i] = true
			}
		}
		batchResp := j.handleBatchRPC(ctx, cfg, batchReq, parseFailed)
		if len(batchResp) == 0 {
			// Batch of notifications only: the server MUST NOT return
			// anything at all.
			return nil
		}
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
			return errorForMalformed(data)
		}
		resp := j.handleRPC(ctx, cfg, &req)
		if resp == nil {
			// Notification: the server MUST NOT reply.
			return nil
		}
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

	return errorForMalformed(data)
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

// splitBatchElements splits the raw bytes of a JSON array into its top-level
// elements without unmarshaling them, so each element can be decoded (and
// fail) independently. ok is false when the array structure itself is broken
// (unbalanced brackets, unterminated string, stray or trailing comma).
func splitBatchElements(data []byte) ([]json.RawMessage, bool) {
	inner := data[1 : len(data)-1] // caller checked data[0]=='[' && data[len-1]==']'
	var elems []json.RawMessage
	depth := 0
	inString, escaped := false, false
	start := -1

	for i := 0; i < len(inner); i++ {
		b := inner[i]
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
			if start == -1 {
				start = i
			}
		case '[', '{':
			if start == -1 {
				start = i
			}
			depth++
		case ']', '}':
			depth--
			if depth < 0 {
				return nil, false
			}
		case ',':
			if depth == 0 {
				if start == -1 {
					return nil, false // empty element: [,] or [a,,b]
				}
				elems = append(elems, json.RawMessage(inner[start:i]))
				start = -1
			}
		case ' ', '\t', '\n', '\r':
		default:
			if start == -1 {
				start = i
			}
		}
	}
	if inString || depth != 0 {
		return nil, false
	}
	if start != -1 {
		elems = append(elems, json.RawMessage(inner[start:]))
	} else if len(elems) > 0 {
		return nil, false // trailing comma: [a,]
	}
	return elems, true
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
// It returns nil for a notification (a valid request without an id): the
// handler still runs, but per the JSON-RPC 2.0 spec the server MUST NOT
// reply, including with errors — those are only logged server-side.
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
	// Only a syntactically valid request without an id member is a
	// notification; a request that fails validation is answered with an
	// id:null error even when it carries no id (spec §5 examples).
	if err := validateRequest(data); err != nil {
		id := data.ID
		if len(id) > 0 && !json.Valid(id) {
			// Never echo a broken id back — it would corrupt the response.
			id = nil
		}
		return j.errorResponse(ctx, cfg, err, InvalidRequestErrorCode, id)
	}

	resp := j.dispatch(ctx, cfg, data)
	if len(data.ID) == 0 {
		// Notification: MUST NOT reply. Handler/timeout errors were already
		// logged by errorResponse.
		return nil
	}
	return resp
}

// dispatch runs one validated request under the configured timeout regime.
func (j *JSONRPC) dispatch(ctx context.Context, cfg *config, data *structs.Request) *structs.Response {
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
	return j.handleBatchRPC(ctx, j.cfg.Load(), data, nil)
}

// handleBatchRPC executes a batch; parseFailed marks entries whose raw JSON
// did not decode into a request object — they get an individual -32600
// response with id:null instead of being dispatched.
func (j *JSONRPC) handleBatchRPC(ctx context.Context, cfg *config, data structs.Requests, parseFailed []bool) structs.BatchFullResponse {
	concurrency := cfg.batchConcurrency
	if concurrency <= 0 || concurrency > len(data) {
		concurrency = len(data)
	}

	// Nil entries mark notifications; they are filtered out below so the
	// batch response only contains entries for id-carrying requests, in
	// request order.
	raw := make([]*structs.Response, len(data))
	indexes := make(chan int)
	wg := sync.WaitGroup{}

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range indexes {
				if parseFailed != nil && parseFailed[idx] {
					raw[idx] = j.errorResponse(ctx, cfg,
						errors.New("batch entry is not a valid request object"), InvalidRequestErrorCode, nil)
					continue
				}
				raw[idx] = j.handleRPC(ctx, cfg, &data[idx])
			}
		}()
	}

	for idx := range data {
		indexes <- idx
	}
	close(indexes)
	wg.Wait()

	responses := make(structs.BatchFullResponse, 0, len(data))
	for _, r := range raw {
		if r != nil {
			responses = append(responses, *r)
		}
	}
	return responses
}

func validateRequest(data *structs.Request) error {
	if data.Version != Version {
		return errors.New("not valid version")
	}

	if data.Method == "" {
		return errors.New("method is required")
	}

	// Spec: id, when present, must be a string, number, or null. The first
	// byte cheaply rejects objects/arrays/booleans; json.Valid then catches
	// broken scalar tokens ("1e", "-", "1.", bad escapes) that easyjson's
	// lazy lexer lets through Raw() — without this they would be echoed
	// byte-exact into the response and corrupt it.
	if len(data.ID) > 0 {
		switch data.ID[0] {
		case '"', '-', 'n', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			if !json.Valid(data.ID) {
				return errors.New("id is not a valid JSON value")
			}
		default:
			return errors.New("id must be a string, number, or null")
		}
	}

	return nil
}
