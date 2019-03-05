package jsonrpc

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/gumeniukcom/golang-jsonrpc2/structs"
)

// HandleRPCJsonRawMessage receive jsonRaw , parse, magic and send jsonRaw
func (j *JSONRPC) HandleRPCJsonRawMessage(ctx context.Context, data json.RawMessage) json.RawMessage {

	reqLen := len(data)
	if reqLen < 2 {
		return errorInvalidRequest()
	}

	if data[0] == '[' && data[reqLen-1] == ']' {
		var batchReq structs.Requests
		if err := batchReq.UnmarshalJSON(data); err != nil {
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

// HandleRPC make rpc request
// TODO: think about timeout
func (j *JSONRPC) HandleRPC(ctx context.Context, data *structs.Request) *structs.Response {
	if err := validateRequest(data); err != nil {
		return j.NewError(ctx, err, InvalidRequestErrorCode, data.ID)
	}
	return j.call(ctx, data.Method, data.Params, data.ID)
}

// HandleBatchRPC make batch rpc
func (j *JSONRPC) HandleBatchRPC(ctx context.Context, data structs.Requests) structs.BatchFullResponse {
	var fullResponses structs.BatchFullResponse

	requestsChan := make(chan structs.Response, len(data))
	wg := sync.WaitGroup{}

	for _, r := range data {
		wg.Add(1)

		go j.HandleRPC(ctx, &r)
	}

	wg.Wait()
	close(requestsChan)

	for r := range requestsChan {
		fullResponses = append(fullResponses, r)
	}

	return fullResponses
}

func validateRequest(data *structs.Request) error {
	if data.Version != JSONRPCVersion {
		return errors.New("not valid version")
	}

	if data.Method == "" {
		return errors.New("method is required")
	}

	return nil
}
