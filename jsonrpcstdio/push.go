package jsonrpcstdio

import (
	"context"
	"errors"
	"fmt"
	"sync"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

// errWriterClosed is returned by pushes attempted after Serve returned.
var errWriterClosed = errors.New("jsonrpcstdio: connection closed")

// connWriter serializes all writes to the stream (response frames and pushed
// notifications) under a mutex. A write failure latches the writer closed
// and reports the error through fail, which cancels the connection — so no
// goroutine keeps queueing behind a dead stream. Serve latches the writer on
// exit for the same reason: a background subscription goroutine calling
// Notify after shutdown must get an error, not a write to a closed stdout
// (which would kill the process with SIGPIPE instead of returning an error).
type connWriter struct {
	fr   framer
	fail func(error)

	mu     sync.Mutex
	closed bool
}

func (w *connWriter) write(frame []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errWriterClosed
	}
	if err := w.fr.WriteFrame(frame); err != nil {
		if errors.Is(err, errUnframeable) {
			// Nothing reached the wire: only this message is lost, the
			// stream is still synchronized — do not kill the connection.
			return err
		}
		// A partial frame may be on the wire; the stream is desynchronized
		// beyond repair. Latch and fail the connection.
		w.closed = true
		w.fail(err)
		return err
	}
	return nil
}

// close latches the writer; subsequent writes return errWriterClosed.
// Acquiring the mutex also guarantees no fail() call is still in flight
// when close returns, so the caller may read the recorded error after it.
func (w *connWriter) close() {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
}

// stdioPusher is the per-connection jsonrpc.Pusher handlers receive via the
// request context. Notify sends a JSON-RPC notification (no id) to the peer
// through the connection's serialized writer.
type stdioPusher struct {
	w *connWriter
}

// Notify sends a server-initiated notification to the peer. It is safe for
// concurrent use and valid for the whole life of the connection — a handler
// may keep the pusher and call Notify from a background goroutine (a
// long-lived subscription) long after it returned. The ctx is accepted for
// jsonrpc.Pusher symmetry but ignored by this transport: a pipe write cannot
// be bounded by a context, and pushing with an already-canceled request
// context must not fail the write. Once the stream is gone Notify returns an
// error, letting the subscription stop.
func (p *stdioPusher) Notify(_ context.Context, method string, params any) error {
	rawParams, err := jsonrpc.MarshalParams(params)
	if err != nil {
		return fmt.Errorf("jsonrpcstdio: marshal params: %w", err)
	}
	// A notification is a request with no id member.
	frame, err := structs.Request{
		Version: jsonrpc.Version,
		Method:  method,
		Params:  rawParams,
	}.MarshalJSON()
	if err != nil {
		return fmt.Errorf("jsonrpcstdio: marshal notification: %w", err)
	}
	return p.w.write(frame)
}

// interface guard
var _ jsonrpc.Pusher = (*stdioPusher)(nil)
