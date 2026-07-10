package jsonrpc

import (
	"context"
	"encoding/json"
	"testing"
)

type recordingPusher struct {
	method string
	params any
}

func (p *recordingPusher) Notify(_ context.Context, method string, params any) error {
	p.method = method
	p.params = params
	return nil
}

func TestPusherRoundTripsThroughContext(t *testing.T) {
	rp := &recordingPusher{}
	ctx := ContextWithPusher(context.Background(), rp)

	got, ok := PusherFromContext(ctx)
	if !ok {
		t.Fatal("pusher must be retrievable from context")
	}
	if err := got.Notify(ctx, "event", map[string]int{"n": 1}); err != nil {
		t.Fatal(err)
	}
	if rp.method != "event" {
		t.Fatalf("expected method 'event', got %q", rp.method)
	}
}

// Absent a transport that supports push (e.g. plain HTTP), lookup reports
// unavailability instead of panicking.
func TestPusherAbsentFromPlainContext(t *testing.T) {
	if _, ok := PusherFromContext(context.Background()); ok {
		t.Fatal("no pusher must be present in a bare context")
	}
}

// A handler using push degrades gracefully when no pusher is available.
func TestHandlerUsesPusherWhenAvailable(t *testing.T) {
	j := New()
	rp := &recordingPusher{}
	if err := j.RegisterMethod("subscribe", func(ctx context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		if p, ok := PusherFromContext(ctx); ok {
			_ = p.Notify(ctx, "tick", 1)
		}
		return json.RawMessage(`"subscribed"`), OK, nil
	}); err != nil {
		t.Fatal(err)
	}

	// With a pusher in context (as a WS transport would inject it):
	ctx := ContextWithPusher(context.Background(), rp)
	resp := j.HandleRPCJSONRawMessage(ctx,
		json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"subscribe"}`))
	if rp.method != "tick" {
		t.Fatalf("handler must push via context pusher, got %q", rp.method)
	}
	if len(resp) == 0 {
		t.Fatal("the call itself must still get a normal response")
	}

	// Without a pusher (plain HTTP): no panic, normal response.
	resp = j.HandleRPCJSONRawMessage(context.Background(),
		json.RawMessage(`{"jsonrpc":"2.0","id":2,"method":"subscribe"}`))
	if len(resp) == 0 {
		t.Fatal("handler must still respond when push is unavailable")
	}
}
