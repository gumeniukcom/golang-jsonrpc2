package jsonrpcws_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpcws"
)

func newRPC(t *testing.T) *jsonrpc.JSONRPC {
	t.Helper()
	j := jsonrpc.New()
	if err := j.RegisterMethod("echo", func(_ context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		if data == nil {
			return json.RawMessage(`null`), jsonrpc.OK, nil
		}
		return data, jsonrpc.OK, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := j.RegisterMethod("slow", func(ctx context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		select {
		case <-ctx.Done():
			return nil, jsonrpc.InternalErrorCode, ctx.Err()
		case <-time.After(300 * time.Millisecond):
			return json.RawMessage(`"slow-done"`), jsonrpc.OK, nil
		}
	}); err != nil {
		t.Fatal(err)
	}
	return j
}

func dial(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.CloseNow() })
	return c
}

func send(t *testing.T, c *websocket.Conn, msg string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageText, []byte(msg)); err != nil {
		t.Fatal(err)
	}
}

func recv(t *testing.T, c *websocket.Conn) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestWSSingleRequest(t *testing.T) {
	srv := httptest.NewServer(jsonrpcws.Handler(newRPC(t)))
	t.Cleanup(srv.Close)
	c := dial(t, srv)

	send(t, c, `{"jsonrpc":"2.0","id":1,"method":"echo","params":{"a":1}}`)
	resp := recv(t, c)
	if !strings.Contains(resp, `"result":{"a":1}`) || !strings.Contains(resp, `"id":1`) {
		t.Fatalf("unexpected response: %s", resp)
	}
}

// A notification gets no frame back: the next frame the client sees must be
// the response to the follow-up request.
func TestWSNotificationProducesNoFrame(t *testing.T) {
	srv := httptest.NewServer(jsonrpcws.Handler(newRPC(t)))
	t.Cleanup(srv.Close)
	c := dial(t, srv)

	send(t, c, `{"jsonrpc":"2.0","method":"echo","params":"notify"}`)
	send(t, c, `{"jsonrpc":"2.0","id":2,"method":"echo","params":"follow-up"}`)
	resp := recv(t, c)
	if !strings.Contains(resp, `"id":2`) {
		t.Fatalf("expected only the follow-up response, got: %s", resp)
	}
}

func TestWSBatch(t *testing.T) {
	srv := httptest.NewServer(jsonrpcws.Handler(newRPC(t)))
	t.Cleanup(srv.Close)
	c := dial(t, srv)

	send(t, c, `[{"jsonrpc":"2.0","id":1,"method":"echo"},{"jsonrpc":"2.0","method":"echo"}]`)
	resp := recv(t, c)
	var batch []json.RawMessage
	if err := json.Unmarshal([]byte(resp), &batch); err != nil || len(batch) != 1 {
		t.Fatalf("expected 1-entry batch response, got: %s", resp)
	}
}

// Requests are processed concurrently: a fast request sent after a slow one
// must be answered first (responses correlate by id, order is not
// guaranteed).
func TestWSConcurrentRequestsInterleave(t *testing.T) {
	srv := httptest.NewServer(jsonrpcws.Handler(newRPC(t)))
	t.Cleanup(srv.Close)
	c := dial(t, srv)

	send(t, c, `{"jsonrpc":"2.0","id":"slow","method":"slow"}`)
	send(t, c, `{"jsonrpc":"2.0","id":"fast","method":"echo","params":1}`)

	first := recv(t, c)
	if !strings.Contains(first, `"id":"fast"`) {
		t.Fatalf("fast response must arrive before the slow one, got: %s", first)
	}
	second := recv(t, c)
	if !strings.Contains(second, `"id":"slow"`) {
		t.Fatalf("slow response must still arrive, got: %s", second)
	}
}

// Binary frames carry JSON just as well as text frames.
func TestWSBinaryFrameAccepted(t *testing.T) {
	srv := httptest.NewServer(jsonrpcws.Handler(newRPC(t)))
	t.Cleanup(srv.Close)
	c := dial(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageBinary,
		[]byte(`{"jsonrpc":"2.0","id":7,"method":"echo","params":true}`)); err != nil {
		t.Fatal(err)
	}
	resp := recv(t, c)
	if !strings.Contains(resp, `"id":7`) {
		t.Fatalf("binary frame must be dispatched, got: %s", resp)
	}
}

// Oversized messages must close the connection (read limit).
func TestWSMessageTooLargeClosesConnection(t *testing.T) {
	srv := httptest.NewServer(jsonrpcws.Handler(newRPC(t), jsonrpcws.WithMaxMessageSize(64)))
	t.Cleanup(srv.Close)
	c := dial(t, srv)

	send(t, c, `{"jsonrpc":"2.0","id":1,"method":"echo","params":"`+strings.Repeat("x", 256)+`"}`)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, err := c.Read(ctx)
	if err == nil {
		t.Fatal("read after oversized message must fail (connection closed)")
	}
	if websocket.CloseStatus(err) != websocket.StatusMessageTooBig {
		t.Fatalf("expected close status 1009 (message too big), got: %v", err)
	}
}

// An abrupt client disconnect must cancel in-flight handler contexts
// promptly — the connection manages its own lifecycle because r.Context()
// is not canceled for hijacked connections.
func TestWSClientDisconnectCancelsInflight(t *testing.T) {
	j := jsonrpc.New()
	canceled := make(chan struct{})
	if err := j.RegisterMethod("wait", func(ctx context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		select {
		case <-ctx.Done():
			close(canceled)
			return nil, jsonrpc.InternalErrorCode, ctx.Err()
		case <-time.After(10 * time.Second):
			return json.RawMessage(`"never"`), jsonrpc.OK, nil
		}
	}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(jsonrpcws.Handler(j))
	t.Cleanup(srv.Close)
	c := dial(t, srv)

	send(t, c, `{"jsonrpc":"2.0","id":1,"method":"wait"}`)
	time.Sleep(50 * time.Millisecond) // let the call start
	_ = c.CloseNow()                  // abrupt disconnect, no close handshake

	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("in-flight handler ctx must be canceled shortly after disconnect")
	}
}

// Cross-origin browser handshakes are rejected by default.
func TestWSOriginRejectedByDefault(t *testing.T) {
	srv := httptest.NewServer(jsonrpcws.Handler(newRPC(t)))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), &websocket.DialOptions{
		HTTPHeader: map[string][]string{"Origin": {"https://evil.example.com"}},
	})
	if err == nil {
		_ = c.CloseNow()
		t.Fatal("cross-origin handshake must be rejected by default")
	}
	if resp == nil || resp.StatusCode != 403 {
		t.Fatalf("expected 403 handshake rejection, got: %+v", resp)
	}
}

// WithOriginPatterns allows the listed origins.
func TestWSOriginPatternAllowed(t *testing.T) {
	srv := httptest.NewServer(jsonrpcws.Handler(newRPC(t),
		jsonrpcws.WithOriginPatterns("trusted.example.com")))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), &websocket.DialOptions{
		HTTPHeader: map[string][]string{"Origin": {"https://trusted.example.com"}},
	})
	if err != nil {
		t.Fatalf("allowed origin must connect: %v", err)
	}
	defer func() { _ = c.CloseNow() }()

	if err := c.Write(ctx, websocket.MessageText,
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"echo"}`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Read(ctx); err != nil {
		t.Fatalf("dispatch over allowed-origin connection must work: %v", err)
	}
}
