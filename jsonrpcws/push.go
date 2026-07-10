package jsonrpcws

import (
	"context"
	"sync"
	"time"

	"github.com/coder/websocket"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

// connWriter serializes all writes to one connection (response frames and
// pushed notifications) under a mutex and bounds each with a timeout. Writes
// are timed off the connection's own context (baseCtx), not any caller's
// request context — so a handler that pushes with an already-canceled
// request context cannot fail the write and tear the connection down. A
// genuine write failure (dead/slow peer, or the connection's own context
// ending) invokes onError, which closes the connection so callers stop
// queueing behind the mutex.
type connWriter struct {
	conn    *websocket.Conn
	baseCtx context.Context //nolint:containedctx // connection-lifetime ctx, deliberately held
	timeout time.Duration
	onError func()

	mu sync.Mutex
}

func (w *connWriter) write(frame []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// WithTimeout arms its own timer, so the bound holds even though baseCtx
	// (a hijacked connection's context) cannot be canceled from outside.
	wctx, cancel := context.WithTimeout(w.baseCtx, w.timeout)
	defer cancel()

	if err := w.conn.Write(wctx, websocket.MessageText, frame); err != nil {
		w.onError()
		return err
	}
	return nil
}

// connPusher is the per-connection jsonrpc.Pusher handlers receive via the
// request context. Notify sends a JSON-RPC notification (no id) to the client
// through the shared connWriter.
type connPusher struct {
	w *connWriter
}

// Notify sends a server-initiated notification to the client. It is safe for
// concurrent use and valid for the whole life of the connection — a handler
// may keep the pusher and call Notify from a background goroutine (a
// long-lived subscription) long after it returned. The ctx is accepted for
// jsonrpc.Pusher symmetry but ignored by this transport: the write is bounded
// by the connection's own context, not the caller's, so pushing with an
// already-canceled request context is safe. Once the connection is gone
// Notify returns an error, letting the subscription stop.
func (p *connPusher) Notify(_ context.Context, method string, params any) error {
	rawParams, err := jsonrpc.MarshalParams(params)
	if err != nil {
		return err
	}
	// A notification is a request with no id member.
	frame, err := structs.Request{
		Version: jsonrpc.Version,
		Method:  method,
		Params:  rawParams,
	}.MarshalJSON()
	if err != nil {
		return err
	}
	return p.w.write(frame)
}

// interface guard
var _ jsonrpc.Pusher = (*connPusher)(nil)
