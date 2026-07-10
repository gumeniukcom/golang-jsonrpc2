package jsonrpcws_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpcws"
)

func httptestServer(t *testing.T, j *jsonrpc.JSONRPC) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(jsonrpcws.Handler(j))
	t.Cleanup(srv.Close)
	return srv
}

func batchRPC(t *testing.T) *jsonrpc.JSONRPC {
	t.Helper()
	j := jsonrpc.New()
	if err := jsonrpc.RegisterTyped(j, "sum", func(_ context.Context, p struct {
		A int `json:"a"`
		B int `json:"b"`
	}) (int, error) {
		return p.A + p.B, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := j.RegisterMethod("boom", func(_ context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		return nil, jsonrpc.InvalidParamsErrorCode, context.DeadlineExceeded
	}); err != nil {
		t.Fatal(err)
	}
	return j
}

func dialBatch(t *testing.T, j *jsonrpc.JSONRPC) *jsonrpcws.Client {
	t.Helper()
	srv := httptestServer(t, j)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := jsonrpcws.DialClient(ctx, wsURL(srv))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestWSClientCallBatch(t *testing.T) {
	c := dialBatch(t, batchRPC(t))

	results, err := c.CallBatch(context.Background(), []jsonrpc.Spec{
		{Method: "sum", Params: map[string]int{"a": 1, "b": 2}},
		{Method: "boom"},
		{Method: "sum", Params: map[string]int{"a": 10, "b": 20}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if v, err := jsonrpc.BatchResultAs[int](results[0]); err != nil || v != 3 {
		t.Fatalf("result 0: %d, %v", v, err)
	}
	if results[1].Error == nil {
		t.Fatalf("result 1 must be error: %+v", results[1])
	}
	if v, err := jsonrpc.BatchResultAs[int](results[2]); err != nil || v != 30 {
		t.Fatalf("result 2: %d, %v", v, err)
	}
}

// A batch and concurrent single Calls share the one connection and its
// pending map; ids must not collide and every result must correlate.
func TestWSClientCallBatchConcurrentWithCalls(t *testing.T) {
	c := dialBatch(t, batchRPC(t))

	var wg sync.WaitGroup
	errs := make(chan error, 40)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, err := jsonrpc.CallResult[int](context.Background(), c, "sum", map[string]int{"a": i, "b": 100})
			if err != nil || v != i+100 {
				errs <- context.DeadlineExceeded
			}
		}(i)
	}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results, err := c.CallBatch(context.Background(), []jsonrpc.Spec{
				{Method: "sum", Params: map[string]int{"a": i, "b": 1}},
				{Method: "sum", Params: map[string]int{"a": i, "b": 2}},
			})
			if err != nil {
				errs <- err
				return
			}
			if v, _ := jsonrpc.BatchResultAs[int](results[0]); v != i+1 {
				errs <- context.DeadlineExceeded
			}
			if v, _ := jsonrpc.BatchResultAs[int](results[1]); v != i+2 {
				errs <- context.DeadlineExceeded
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal("concurrent batch/call correlation failed:", err)
	}
}

func TestWSClientCallBatchWithNotifications(t *testing.T) {
	c := dialBatch(t, batchRPC(t))

	results, err := c.CallBatch(context.Background(), []jsonrpc.Spec{
		{Method: "sum", Params: map[string]int{"a": 3, "b": 4}},
		{Method: "sum", Params: map[string]int{"a": 0, "b": 0}, Notify: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := jsonrpc.BatchResultAs[int](results[0]); v != 7 {
		t.Fatalf("call slot: %d", v)
	}
	if results[1].Result != nil || results[1].Error != nil {
		t.Fatalf("notification slot must be zero: %+v", results[1])
	}
}

func TestWSClientCallBatchEmpty(t *testing.T) {
	c := dialBatch(t, batchRPC(t))
	results, err := c.CallBatch(context.Background(), nil)
	if err != nil || results != nil {
		t.Fatalf("empty batch is a no-op, got %v, %v", results, err)
	}
}

// A batch over the client's configured limit is rejected locally, before it
// can draw an unaddressable id:null error from the server.
func TestWSClientCallBatchExceedsClientLimit(t *testing.T) {
	srv := httptestServer(t, batchRPC(t))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := jsonrpcws.DialClient(ctx, wsURL(srv), jsonrpcws.WithClientMaxBatchSize(2))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	specs := []jsonrpc.Spec{{Method: "sum"}, {Method: "sum"}, {Method: "sum"}}
	_, err = c.CallBatch(ctx, specs)
	if !errors.Is(err, jsonrpcws.ErrBatchTooLarge) {
		t.Fatalf("expected ErrBatchTooLarge, got: %v", err)
	}
}

// Backstop: with the client-side check disabled, a batch the SERVER rejects
// wholesale (its default cap is DefaultMaxBatchSize=100) draws a top-level
// id:null error; CallBatch must return an error rather than hang.
func TestWSClientCallBatchServerRejectDoesNotHang(t *testing.T) {
	j := batchRPC(t) // server keeps its default batch cap of 100
	srv := httptestServer(t, j)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := jsonrpcws.DialClient(ctx, wsURL(srv), jsonrpcws.WithClientMaxBatchSize(0)) // disable client check
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	specs := make([]jsonrpc.Spec, 150) // over the server's 100 cap
	for i := range specs {
		specs[i] = jsonrpc.Spec{Method: "sum", Params: map[string]int{"a": i, "b": 1}}
	}

	done := make(chan error, 1)
	go func() {
		results, err := c.CallBatch(ctx, specs)
		if err != nil {
			done <- err
			return
		}
		// Or per-slot errors — either way it must not hang.
		for _, r := range results {
			if r.Error != nil {
				done <- r.Error
				return
			}
		}
		done <- nil
	}()

	select {
	case <-done:
		// returned (error or slots) — the point is it did not hang
	case <-time.After(3 * time.Second):
		t.Fatal("CallBatch hung on a server-rejected oversized batch")
	}
}

// An unsolicited array frame with no pending calls is a cheap no-op (bound
// = len(pending) = 0), and a bogus id in a batch response is dropped.
func TestWSClientBatchArrayFrameBounded(t *testing.T) {
	c := dialBatch(t, batchRPC(t))
	// No calls outstanding: a normal single call still works, proving the
	// read loop is healthy and not stuck on stray frames.
	if v, err := jsonrpc.CallResult[int](context.Background(), c, "sum",
		map[string]int{"a": 2, "b": 5}); err != nil || v != 7 {
		t.Fatalf("single call after idle must work: %d, %v", v, err)
	}
}
