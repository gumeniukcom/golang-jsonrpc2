package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gumeniukcom/golang-jsonrpc2/v2/internal/codec"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

// Spec describes one request in a client-side batch.
type Spec struct {
	Method string
	Params any
	// Notify sends this entry as a notification: no id, and no result slot
	// in the batch response.
	Notify bool
}

// BatchResult is the outcome of one non-notification Spec, aligned by index
// with the specs passed to CallBatch. On a returned call entry exactly one of
// Result / Error is meaningful; a notification's entry is the zero value.
type BatchResult struct {
	Result json.RawMessage
	Error  *structs.Error
}

// BatchCaller is implemented by transport clients that can send a JSON-RPC
// batch (jsonrpchttp.Client, jsonrpcws.Client). Results align by index with
// specs; a notification spec yields the zero BatchResult. An empty batch
// makes no network call and returns nil, nil.
type BatchCaller interface {
	CallBatch(ctx context.Context, specs []Spec) ([]BatchResult, error)
}

// BatchResultAs unmarshals a batch result into R, surfacing a JSON-RPC error
// response as its *structs.Error.
func BatchResultAs[R any](br BatchResult) (R, error) {
	var r R
	if br.Error != nil {
		return r, br.Error
	}
	if len(br.Result) == 0 {
		return r, nil
	}
	if err := codec.Unmarshal(br.Result, &r); err != nil {
		return r, fmt.Errorf("unmarshal batch result: %w", err)
	}
	return r, nil
}

// MarshalBatch builds the raw JSON-RPC batch request frame for specs and
// returns, for each spec, the id assigned to it (nil for notifications).
// nextID allocates a fresh id per non-notification spec; a transport passes
// its own id source so batch ids share the client's id space (WebSocket
// correlates concurrent calls and batch entries in one pending map).
//
// It is exported for the transport client packages; most callers use the
// clients' CallBatch instead.
func MarshalBatch(specs []Spec, nextID func() structs.ID) (json.RawMessage, []structs.ID, error) {
	ids := make([]structs.ID, len(specs))
	reqs := make(structs.Requests, len(specs))
	for i, s := range specs {
		raw, err := MarshalParams(s.Params)
		if err != nil {
			return nil, nil, err
		}
		var id structs.ID
		if !s.Notify {
			id = nextID()
		}
		ids[i] = id
		reqs[i] = structs.Request{Version: Version, Method: s.Method, Params: raw, ID: id}
	}
	frame, err := reqs.MarshalJSON()
	if err != nil {
		return nil, nil, fmt.Errorf("marshal batch: %w", err)
	}
	return frame, ids, nil
}

// BatchResultFromResponse converts a decoded response into a BatchResult.
// Exported for the transport client packages.
func BatchResultFromResponse(resp *structs.Response) BatchResult {
	if resp.Error != nil {
		return BatchResult{Error: resp.Error}
	}
	if resp.Result != nil {
		return BatchResult{Result: *resp.Result}
	}
	return BatchResult{}
}
