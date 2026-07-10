package jsonrpcws_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpcws"
)

type notice struct {
	method string
	params json.RawMessage
}

func pushServer(t *testing.T) *httptest.Server {
	t.Helper()
	j := jsonrpc.New()
	// subscribe pushes three ticks, then returns.
	if err := j.RegisterMethod("subscribe", func(ctx context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		p, ok := jsonrpc.PusherFromContext(ctx)
		if !ok {
			return nil, jsonrpc.InternalErrorCode, context.Canceled
		}
		for i := 1; i <= 3; i++ {
			if err := p.Notify(ctx, "tick", map[string]int{"n": i}); err != nil {
				return nil, jsonrpc.InternalErrorCode, err
			}
		}
		return json.RawMessage(`"subscribed"`), jsonrpc.OK, nil
	}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(jsonrpcws.Handler(j))
	t.Cleanup(srv.Close)
	return srv
}

// The server pushes notifications and the client delivers them to the
// registered handler; the call still returns its own response.
func TestServerPushDeliveredToClient(t *testing.T) {
	srv := pushServer(t)

	var mu sync.Mutex
	var got []notice
	done := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := jsonrpcws.DialClient(ctx, wsURL(srv),
		jsonrpcws.WithNotificationHandler(func(method string, params json.RawMessage) {
			mu.Lock()
			got = append(got, notice{method, params})
			if len(got) == 3 {
				close(done)
			}
			mu.Unlock()
		}))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	res, err := jsonrpc.CallResult[string](ctx, c, "subscribe", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res != "subscribed" {
		t.Fatalf("call must still return its result, got %q", res)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("expected 3 pushed notifications, got %d", len(got))
	}

	mu.Lock()
	defer mu.Unlock()
	for i, n := range got {
		if n.method != "tick" {
			t.Fatalf("notification %d method = %q, want tick", i, n.method)
		}
	}
}

// A long-lived subscription pushes from a background goroutine after the
// handler returned. The pusher must stay valid for the life of the
// connection, and the connection must survive (other calls still work) —
// pushing with a canceled request context must not tear it down.
func TestServerPushFromBackgroundGoroutine(t *testing.T) {
	j := jsonrpc.New()
	if err := j.RegisterMethod("subscribe", func(ctx context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		p, ok := jsonrpc.PusherFromContext(ctx)
		if !ok {
			return nil, jsonrpc.InternalErrorCode, context.Canceled
		}
		// Deliberately push with an ALREADY-CANCELED context to prove the
		// write is bounded by the connection, not the caller's ctx — an
		// early return of the handler would have canceled a request ctx like
		// this, and it must not fail the push or tear the connection down.
		canceled, cancelNow := context.WithCancel(context.Background())
		cancelNow()
		go func() {
			for i := 1; i <= 3; i++ {
				time.Sleep(20 * time.Millisecond)
				if err := p.Notify(canceled, "tick", map[string]int{"n": i}); err != nil {
					return
				}
			}
		}()
		return json.RawMessage(`"subscribed"`), jsonrpc.OK, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := j.RegisterMethod("ping", func(_ context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		return json.RawMessage(`"pong"`), jsonrpc.OK, nil
	}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(jsonrpcws.Handler(j))
	t.Cleanup(srv.Close)

	var mu sync.Mutex
	var count int
	done := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := jsonrpcws.DialClient(ctx, wsURL(srv),
		jsonrpcws.WithNotificationHandler(func(method string, _ json.RawMessage) {
			if method != "tick" {
				return
			}
			mu.Lock()
			count++
			if count == 3 {
				close(done)
			}
			mu.Unlock()
		}))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	if _, err := c.Call(ctx, "subscribe", nil); err != nil {
		t.Fatal(err)
	}

	// Deferred pushes (from the goroutine, with a canceled request ctx long
	// gone) must arrive.
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("expected 3 deferred pushes, got %d", count)
	}

	// The connection must still be alive for other calls.
	res, err := jsonrpc.CallResult[string](ctx, c, "ping", nil)
	if err != nil || res != "pong" {
		t.Fatalf("connection must survive background pushes; ping got %q, %v", res, err)
	}
}

// Without a registered handler, pushed notifications are simply dropped and
// must not disturb the call/response correlation.
func TestServerPushWithoutHandlerDropped(t *testing.T) {
	srv := pushServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := jsonrpcws.DialClient(ctx, wsURL(srv))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	res, err := jsonrpc.CallResult[string](ctx, c, "subscribe", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res != "subscribed" {
		t.Fatalf("call must succeed even with pushes dropped, got %q", res)
	}
}
