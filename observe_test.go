package jsonrpc

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

type obsRecorder struct {
	mu    sync.Mutex
	calls []CallInfo
}

func (r *obsRecorder) fn() ObserveFunc {
	return func(_ context.Context, info CallInfo) {
		r.mu.Lock()
		r.calls = append(r.calls, info)
		r.mu.Unlock()
	}
}

func (r *obsRecorder) list() []CallInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]CallInfo(nil), r.calls...)
}

func TestObserverFiresOnSuccess(t *testing.T) {
	j := New()
	rec := &obsRecorder{}
	j.SetObserver(rec.fn())
	if err := j.RegisterMethod("echo", func(_ context.Context, d json.RawMessage) (json.RawMessage, int, error) {
		return d, OK, nil
	}); err != nil {
		t.Fatal(err)
	}

	j.HandleRPCJSONRawMessage(context.Background(),
		json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"echo","params":5}`))

	calls := rec.list()
	if len(calls) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(calls))
	}
	c := calls[0]
	if c.Method != "echo" || c.Code != OK || c.Err != nil || c.Notification {
		t.Fatalf("unexpected observation: %+v", c)
	}
}

func TestObserverFiresOnMethodNotFound(t *testing.T) {
	j := New()
	rec := &obsRecorder{}
	j.SetObserver(rec.fn())

	j.HandleRPCJSONRawMessage(context.Background(),
		json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"nope"}`))

	calls := rec.list()
	if len(calls) != 1 || calls[0].Code != MethodNotFoundErrorCode {
		t.Fatalf("method-not-found must be observed with -32601, got: %+v", calls)
	}
	if calls[0].Err == nil {
		t.Fatal("error outcome must carry a client-facing Err")
	}
}

func TestObserverFiresOnTimeout(t *testing.T) {
	j := New()
	j.SetDefaultTimeOut(20 * time.Millisecond)
	rec := &obsRecorder{}
	j.SetObserver(rec.fn())
	if err := j.RegisterMethod("slow", func(_ context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		time.Sleep(80 * time.Millisecond)
		return json.RawMessage(`1`), OK, nil
	}); err != nil {
		t.Fatal(err)
	}

	j.HandleRPCJSONRawMessage(context.Background(),
		json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"slow"}`))

	calls := rec.list()
	if len(calls) != 1 || calls[0].Code != RequestTimeLimit {
		t.Fatalf("timeout must be observed with -32605, got: %+v", calls)
	}
	if calls[0].Duration < 20*time.Millisecond {
		t.Fatalf("duration must reflect the elapsed time, got %v", calls[0].Duration)
	}
}

func TestObserverFiresOnNotification(t *testing.T) {
	j := New()
	rec := &obsRecorder{}
	j.SetObserver(rec.fn())
	ran := false
	if err := j.RegisterMethod("fire", func(_ context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		ran = true
		return json.RawMessage(`1`), OK, nil
	}); err != nil {
		t.Fatal(err)
	}

	resp := j.HandleRPCJSONRawMessage(context.Background(),
		json.RawMessage(`{"jsonrpc":"2.0","method":"fire"}`))
	if resp != nil {
		t.Fatalf("notification must produce no response, got: %s", resp)
	}
	if !ran {
		t.Fatal("handler must run for a notification")
	}
	calls := rec.list()
	if len(calls) != 1 || !calls[0].Notification || calls[0].Method != "fire" {
		t.Fatalf("notification must be observed with Notification=true, got: %+v", calls)
	}
}

func TestObserverFiresPerBatchEntry(t *testing.T) {
	j := New()
	rec := &obsRecorder{}
	j.SetObserver(rec.fn())
	if err := j.RegisterMethod("ok", func(_ context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		return json.RawMessage(`1`), OK, nil
	}); err != nil {
		t.Fatal(err)
	}

	j.HandleRPCJSONRawMessage(context.Background(), json.RawMessage(`[
		{"jsonrpc":"2.0","id":1,"method":"ok"},
		{"jsonrpc":"2.0","method":"ok"},
		{"jsonrpc":"2.0","id":3,"method":"missing"}
	]`))

	calls := rec.list()
	if len(calls) != 3 {
		t.Fatalf("expected 3 observations (one per batch entry), got %d: %+v", len(calls), calls)
	}
	var notifs, notFound int
	for _, c := range calls {
		if c.Notification {
			notifs++
		}
		if c.Code == MethodNotFoundErrorCode {
			notFound++
		}
	}
	if notifs != 1 || notFound != 1 {
		t.Fatalf("batch observations wrong: notifs=%d notFound=%d", notifs, notFound)
	}
}

// A batch entry whose JSON does not decode into a request object must still
// be observed, so the documented "fires per entry" holds.
func TestObserverFiresOnUndecodableBatchEntry(t *testing.T) {
	j := New()
	rec := &obsRecorder{}
	j.SetObserver(rec.fn())
	if err := j.RegisterMethod("ok", func(_ context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		return json.RawMessage(`1`), OK, nil
	}); err != nil {
		t.Fatal(err)
	}

	// Second entry is a bare number: a valid JSON element that is not a
	// request object (parseFailed path).
	j.HandleRPCJSONRawMessage(context.Background(), json.RawMessage(`[
		{"jsonrpc":"2.0","id":1,"method":"ok"},
		42
	]`))

	calls := rec.list()
	if len(calls) != 2 {
		t.Fatalf("both entries must be observed, got %d: %+v", len(calls), calls)
	}
	var invalid int
	for _, c := range calls {
		if c.Code == InvalidRequestErrorCode {
			invalid++
		}
	}
	if invalid != 1 {
		t.Fatalf("the undecodable entry must be observed with -32600, got: %+v", calls)
	}
}

// A panicking observer must not crash the server: the panic is recovered and
// the request still gets its response.
func TestObserverPanicDoesNotCrash(t *testing.T) {
	j := New()
	j.SetLogger(nil)
	j.SetObserver(func(_ context.Context, _ CallInfo) {
		panic("observer bug")
	})
	if err := j.RegisterMethod("ok", func(_ context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		return json.RawMessage(`"fine"`), OK, nil
	}); err != nil {
		t.Fatal(err)
	}

	// Single request must survive.
	resp := j.HandleRPCJSONRawMessage(context.Background(),
		json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"ok"}`))
	if !strings.Contains(string(resp), `"fine"`) {
		t.Fatalf("panicking observer must not break the response, got: %s", resp)
	}

	// Batch must survive too (a panic in a worker goroutine would otherwise
	// crash the process).
	resp = j.HandleRPCJSONRawMessage(context.Background(), json.RawMessage(`[
		{"jsonrpc":"2.0","id":1,"method":"ok"},
		{"jsonrpc":"2.0","id":2,"method":"ok"}
	]`))
	var batch []json.RawMessage
	if err := json.Unmarshal(resp, &batch); err != nil || len(batch) != 2 {
		t.Fatalf("panicking observer must not break the batch, got: %s", resp)
	}
}

func TestObserverFiresOnPanic(t *testing.T) {
	j := New()
	j.SetLogger(nil)
	rec := &obsRecorder{}
	j.SetObserver(rec.fn())
	if err := j.RegisterMethod("boom", func(_ context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		panic("kaboom")
	}); err != nil {
		t.Fatal(err)
	}

	j.HandleRPCJSONRawMessage(context.Background(),
		json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"boom"}`))

	calls := rec.list()
	if len(calls) != 1 || calls[0].Code != InternalErrorCode {
		t.Fatalf("panic must be observed as internal_error, got: %+v", calls)
	}
}

func TestNoObserverIsNoOp(t *testing.T) {
	j := New()
	if err := j.RegisterMethod("echo", func(_ context.Context, d json.RawMessage) (json.RawMessage, int, error) {
		return d, OK, nil
	}); err != nil {
		t.Fatal(err)
	}
	// No observer set — must not panic.
	resp := j.HandleRPCJSONRawMessage(context.Background(),
		json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"echo","params":1}`))
	if len(resp) == 0 {
		t.Fatal("dispatch must work without an observer")
	}

	// SetObserver(nil) explicitly disables.
	rec := &obsRecorder{}
	j.SetObserver(rec.fn())
	j.SetObserver(nil)
	j.HandleRPCJSONRawMessage(context.Background(),
		json.RawMessage(`{"jsonrpc":"2.0","id":2,"method":"echo","params":1}`))
	if len(rec.list()) != 0 {
		t.Fatal("SetObserver(nil) must disable observation")
	}
}
