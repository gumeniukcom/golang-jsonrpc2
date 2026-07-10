package jsonrpchttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpchttp"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

func newClientServer(t *testing.T) *httptest.Server {
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
		return nil, jsonrpc.InvalidParamsErrorCode, errors.New("kaboom")
	}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(jsonrpchttp.Handler(j))
	t.Cleanup(srv.Close)
	return srv
}

func TestHTTPClientCall(t *testing.T) {
	srv := newClientServer(t)
	c := jsonrpchttp.NewClient(srv.URL)

	res, err := jsonrpc.CallResult[int](context.Background(), c, "sum",
		map[string]int{"a": 3, "b": 4})
	if err != nil {
		t.Fatal(err)
	}
	if res != 7 {
		t.Fatalf("expected 7, got %d", res)
	}
}

func TestHTTPClientRPCError(t *testing.T) {
	srv := newClientServer(t)
	c := jsonrpchttp.NewClient(srv.URL)

	_, err := c.Call(context.Background(), "boom", nil)
	var rpcErr *structs.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.InvalidParamsErrorCode {
		t.Fatalf("expected *structs.Error(-32602), got: %v", err)
	}

	_, err = c.Call(context.Background(), "no_such_method", nil)
	if !errors.As(err, &rpcErr) || rpcErr.Code != -32601 {
		t.Fatalf("expected method-not-found, got: %v", err)
	}
}

func TestHTTPClientNotify(t *testing.T) {
	srv := newClientServer(t)
	c := jsonrpchttp.NewClient(srv.URL)

	if err := c.Notify(context.Background(), "sum", map[string]int{"a": 1, "b": 2}); err != nil {
		t.Fatalf("notify must succeed on 204, got: %v", err)
	}
}

func TestHTTPClientTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down for maintenance", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	c := jsonrpchttp.NewClient(srv.URL)

	_, err := c.Call(context.Background(), "sum", nil)
	if err == nil {
		t.Fatal("expected transport error for 503")
	}
	var rpcErr *structs.Error
	if errors.As(err, &rpcErr) {
		t.Fatalf("503 is a transport error, not an RPC error: %v", err)
	}
}

func TestHTTPClientContextCancellation(t *testing.T) {
	j := jsonrpc.New()
	if err := j.RegisterMethod("slow", func(ctx context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		select {
		case <-ctx.Done():
			return nil, jsonrpc.InternalErrorCode, ctx.Err()
		case <-time.After(5 * time.Second):
			return json.RawMessage(`1`), jsonrpc.OK, nil
		}
	}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(jsonrpchttp.Handler(j))
	t.Cleanup(srv.Close)
	c := jsonrpchttp.NewClient(srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.Call(ctx, "slow", nil)
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("call must fail promptly on ctx timeout, err=%v", err)
	}
}

func TestHTTPClientCustomHTTPClient(t *testing.T) {
	srv := newClientServer(t)
	used := false
	hc := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		used = true
		return http.DefaultTransport.RoundTrip(r)
	})}
	c := jsonrpchttp.NewClient(srv.URL, jsonrpchttp.WithHTTPClient(hc))

	if _, err := c.Call(context.Background(), "sum", map[string]int{"a": 1, "b": 1}); err != nil {
		t.Fatal(err)
	}
	if !used {
		t.Fatal("custom http.Client must be used")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
