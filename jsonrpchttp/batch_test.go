package jsonrpchttp_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpchttp"
)

// A server that echoes each request id and injects an extra bogus-id response
// must not corrupt correlation: expected ids resolve, the bogus one is
// ignored, and a missing id becomes an error slot.
func TestHTTPClientCallBatchToleratesBogusAndMissingIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var reqs []struct {
			ID json.RawMessage `json:"id"`
		}
		_ = json.Unmarshal(body, &reqs)
		// Respond to the FIRST request only, add a bogus-id entry; omit the rest.
		out := []json.RawMessage{
			json.RawMessage(`{"jsonrpc":"2.0","result":1,"id":` + string(reqs[0].ID) + `}`),
			json.RawMessage(`{"jsonrpc":"2.0","result":99,"id":"nope-not-sent"}`),
		}
		resp, _ := json.Marshal(out)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(resp)
	}))
	t.Cleanup(srv.Close)

	c := jsonrpchttp.NewClient(srv.URL)
	results, err := c.CallBatch(context.Background(), []jsonrpc.Spec{
		{Method: "a"},
		{Method: "b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if v, err := jsonrpc.BatchResultAs[int](results[0]); err != nil || v != 1 {
		t.Fatalf("result 0 must resolve to 1, got %d, %v", v, err)
	}
	if results[1].Error == nil || results[1].Error.Code != jsonrpc.InternalErrorCode {
		t.Fatalf("missing id 1 must be an internal_error slot, got: %+v", results[1])
	}
}

func TestHTTPClientCallBatch(t *testing.T) {
	srv := newClientServer(t)
	c := jsonrpchttp.NewClient(srv.URL)

	results, err := c.CallBatch(context.Background(), []jsonrpc.Spec{
		{Method: "sum", Params: map[string]int{"a": 1, "b": 2}},
		{Method: "boom"},
		{Method: "sum", Params: map[string]int{"a": 10, "b": 20}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	if v, err := jsonrpc.BatchResultAs[int](results[0]); err != nil || v != 3 {
		t.Fatalf("result 0: got %d, %v", v, err)
	}
	if results[1].Error == nil || results[1].Error.Code != jsonrpc.InvalidParamsErrorCode {
		t.Fatalf("result 1 must be an error: %+v", results[1])
	}
	if v, err := jsonrpc.BatchResultAs[int](results[2]); err != nil || v != 30 {
		t.Fatalf("result 2: got %d, %v", v, err)
	}
}

// Results stay aligned with specs even though the server may return
// responses in any order.
func TestHTTPClientCallBatchOrdering(t *testing.T) {
	srv := newClientServer(t)
	c := jsonrpchttp.NewClient(srv.URL)

	specs := make([]jsonrpc.Spec, 20)
	for i := range specs {
		specs[i] = jsonrpc.Spec{Method: "sum", Params: map[string]int{"a": i, "b": 1}}
	}
	results, err := c.CallBatch(context.Background(), specs)
	if err != nil {
		t.Fatal(err)
	}
	for i := range results {
		v, err := jsonrpc.BatchResultAs[int](results[i])
		if err != nil || v != i+1 {
			t.Fatalf("result %d: got %d, %v", i, v, err)
		}
	}
}

// Notifications in a batch produce no response slot and no server reply.
func TestHTTPClientCallBatchWithNotifications(t *testing.T) {
	srv := newClientServer(t)
	c := jsonrpchttp.NewClient(srv.URL)

	results, err := c.CallBatch(context.Background(), []jsonrpc.Spec{
		{Method: "sum", Params: map[string]int{"a": 2, "b": 3}},
		{Method: "sum", Params: map[string]int{"a": 0, "b": 0}, Notify: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if v, err := jsonrpc.BatchResultAs[int](results[0]); err != nil || v != 5 {
		t.Fatalf("call result: got %d, %v", v, err)
	}
	if results[1].Result != nil || results[1].Error != nil {
		t.Fatalf("notification slot must be zero, got: %+v", results[1])
	}
}

// An all-notification batch returns zero-value slots and makes no error.
func TestHTTPClientCallBatchAllNotifications(t *testing.T) {
	srv := newClientServer(t)
	c := jsonrpchttp.NewClient(srv.URL)

	results, err := c.CallBatch(context.Background(), []jsonrpc.Spec{
		{Method: "sum", Params: map[string]int{"a": 1, "b": 1}, Notify: true},
		{Method: "sum", Params: map[string]int{"a": 2, "b": 2}, Notify: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range results {
		if r.Result != nil || r.Error != nil {
			t.Fatalf("slot %d must be zero, got: %+v", i, r)
		}
	}
}

func TestHTTPClientCallBatchEmpty(t *testing.T) {
	srv := newClientServer(t)
	c := jsonrpchttp.NewClient(srv.URL)
	results, err := c.CallBatch(context.Background(), nil)
	if err != nil || results != nil {
		t.Fatalf("empty batch must be a no-op, got %v, %v", results, err)
	}
}

// The BatchCaller interface is satisfied and usable via the abstraction.
func TestHTTPClientBatchCallerInterface(t *testing.T) {
	srv := newClientServer(t)
	var bc jsonrpc.BatchCaller = jsonrpchttp.NewClient(srv.URL)
	results, err := bc.CallBatch(context.Background(), []jsonrpc.Spec{{Method: "sum", Params: map[string]int{"a": 4, "b": 4}}})
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := jsonrpc.BatchResultAs[int](results[0]); v != 8 {
		t.Fatalf("via interface: got %d", v)
	}
}
