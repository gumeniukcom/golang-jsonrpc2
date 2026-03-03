package jsonrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

// HandleRPCJSONRawMessage parses raw JSON, dispatches single or batch requests,
// and returns the serialized JSON-RPC response.
func (j *JSONRPC) HandleRPCJSONRawMessage(ctx context.Context, data json.RawMessage) json.RawMessage {
	reqLen := len(data)
	if reqLen < 2 {
		return errorInvalidRequest()
	}

	if data[0] == '[' && data[reqLen-1] == ']' {
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
			return errorInternal()
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
			return errorInternal()
		}
		return respRaw
	}

	return errorInvalidRequest()
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

// HandleBatchRPC executes a batch of JSON-RPC requests concurrently.
func (j *JSONRPC) HandleBatchRPC(ctx context.Context, data structs.Requests) structs.BatchFullResponse {
	var fullResponses structs.BatchFullResponse

	requestsChan := make(chan structs.Response, len(data))
	wg := sync.WaitGroup{}

	for idx := range data {
		wg.Add(1)
		go func(ctxi context.Context, rr *structs.Request) {
			defer wg.Done()
			requestsChan <- *j.HandleRPC(ctxi, rr)
		}(ctx, &data[idx])
	}

	wg.Wait()
	close(requestsChan)

	for r := range requestsChan {
		fullResponses = append(fullResponses, r)
	}

	return fullResponses
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
