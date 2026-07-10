package jsonrpcstdio_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpcstdio"
)

// The pusher outlives the handler that grabbed it: a background subscription
// goroutine keeps notifying after the handler returned (connection-lifetime
// contract, same as jsonrpcws).
func TestPusherSurvivesHandlerReturn(t *testing.T) {
	j := jsonrpc.New()
	kick := make(chan struct{})
	if err := j.RegisterMethod("subscribe", func(ctx context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		p, ok := jsonrpc.PusherFromContext(ctx)
		if !ok {
			return nil, jsonrpc.InternalErrorCode, errors.New("no pusher")
		}
		go func() {
			<-kick // fires long after this handler returned
			_ = p.Notify(context.Background(), "tick", 1)
		}()
		return json.RawMessage(`"subscribed"`), jsonrpc.OK, nil
	}); err != nil {
		t.Fatal(err)
	}

	pushed := make(chan struct{})
	c := startLoopback(t, j, jsonrpcstdio.FramingNDJSON,
		jsonrpcstdio.WithNotificationHandler(func(method string, _ json.RawMessage) {
			if method == "tick" {
				close(pushed)
			}
		}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.Call(ctx, "subscribe", nil); err != nil {
		t.Fatal(err)
	}

	close(kick) // the handler is long gone; the background push must still work
	select {
	case <-pushed:
	case <-time.After(5 * time.Second):
		t.Fatal("background push never delivered")
	}
}

// After Serve returns, the writer is latched: a late Notify from a leftover
// subscription goroutine gets an error instead of writing to a dead stdout
// (which would kill a real process with SIGPIPE).
func TestPusherFailsAfterServeReturns(t *testing.T) {
	j := jsonrpc.New()
	grabbed := make(chan jsonrpc.Pusher, 1)
	if err := j.RegisterMethod("grab", func(ctx context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		p, ok := jsonrpc.PusherFromContext(ctx)
		if !ok {
			return nil, jsonrpc.InternalErrorCode, errors.New("no pusher")
		}
		grabbed <- p
		return json.RawMessage(`true`), jsonrpc.OK, nil
	}); err != nil {
		t.Fatal(err)
	}

	rpc := j
	c := startServe(t, context.Background(), rpc, jsonrpcstdio.FramingNDJSON)
	c.send(t, `{"jsonrpc":"2.0","method":"grab","id":1}`)
	_ = c.recv(t)
	pusher := <-grabbed

	_ = c.inW.Close() // orderly shutdown
	if err := c.wait(t); err != nil {
		t.Fatalf("clean EOF must return nil, got %v", err)
	}

	if err := pusher.Notify(context.Background(), "late", nil); err == nil {
		t.Fatal("Notify after Serve returned must fail, not write to a dead stream")
	}
}
