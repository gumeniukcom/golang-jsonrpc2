package jsonrpcws_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpcws"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

func newClientServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(jsonrpcws.Handler(newRPC(t)))
	t.Cleanup(srv.Close)
	return srv
}

func wsURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func dialClient(t *testing.T, srv *httptest.Server) *jsonrpcws.Client {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := jsonrpcws.DialClient(ctx, wsURL(srv))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestWSClientCall(t *testing.T) {
	c := dialClient(t, newClientServer(t))

	res, err := jsonrpc.CallResult[map[string]int](context.Background(), c, "echo",
		map[string]int{"a": 5})
	if err != nil {
		t.Fatal(err)
	}
	if res["a"] != 5 {
		t.Fatalf("expected echo of a=5, got %+v", res)
	}
}

func TestWSClientRPCError(t *testing.T) {
	c := dialClient(t, newClientServer(t))

	_, err := c.Call(context.Background(), "no_such_method", nil)
	var rpcErr *structs.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != -32601 {
		t.Fatalf("expected method-not-found, got: %v", err)
	}
}

// Concurrent calls over one connection must correlate by id even when
// responses arrive out of order.
func TestWSClientConcurrentCalls(t *testing.T) {
	c := dialClient(t, newClientServer(t))

	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			method := "echo"
			if i%3 == 0 {
				method = "slow" // 300ms — forces interleaving
			}
			raw, err := c.Call(context.Background(), method, i)
			if err != nil {
				errs <- err
				return
			}
			if method == "echo" && strings.TrimSpace(string(raw)) != json.Number(itoa(i)).String() {
				errs <- errors.New("wrong correlation: got " + string(raw) + " for " + itoa(i))
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func itoa(i int) string {
	return string(rune('0' + i))
}

// A method returning a JSON null result must correlate back to its call, not
// be misrouted to the push path (which would hang the call).
func TestWSClientNullResultCorrelates(t *testing.T) {
	j := newRPC(t)
	if err := j.RegisterMethod("void", func(_ context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		return json.RawMessage(`null`), jsonrpc.OK, nil
	}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(jsonrpcws.Handler(j))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, err := jsonrpcws.DialClient(ctx, wsURL(srv))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	// The call must correlate and return (not hang in the push path); a null
	// result surfaces as an empty raw result (void), which CallResult[R]
	// turns into R's zero value.
	raw, err := c.Call(ctx, "void", nil)
	if err != nil {
		t.Fatalf("null-result call must return, got: %v", err)
	}
	if s := strings.TrimSpace(string(raw)); s != "" && s != "null" {
		t.Fatalf("expected empty/null result, got: %q", raw)
	}
}

func TestWSClientNotify(t *testing.T) {
	c := dialClient(t, newClientServer(t))

	if err := c.Notify(context.Background(), "echo", "fire-and-forget"); err != nil {
		t.Fatal(err)
	}
	// The connection must remain usable after a notification.
	if _, err := c.Call(context.Background(), "echo", 1); err != nil {
		t.Fatal(err)
	}
}

func TestWSClientCallContextTimeout(t *testing.T) {
	c := dialClient(t, newClientServer(t))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := c.Call(ctx, "slow", nil) // slow takes 300ms
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got: %v", err)
	}
}

// Closing the client must fail pending calls instead of hanging them.
func TestWSClientCloseFailsPending(t *testing.T) {
	c := dialClient(t, newClientServer(t))

	done := make(chan error, 1)
	go func() {
		_, err := c.Call(context.Background(), "slow", nil)
		done <- err
	}()
	time.Sleep(50 * time.Millisecond) // let the call get registered
	_ = c.Close()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("pending call must fail on close")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pending call must not hang after close")
	}
}

// Calls after Close must fail immediately.
func TestWSClientCallAfterClose(t *testing.T) {
	c := dialClient(t, newClientServer(t))
	_ = c.Close()

	if _, err := c.Call(context.Background(), "echo", 1); err == nil {
		t.Fatal("call on closed client must fail")
	}
}
