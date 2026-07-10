// Package jsonrpcws adapts a jsonrpc.JSONRPC dispatcher to WebSocket
// (github.com/coder/websocket): each text or binary frame is one JSON-RPC
// message (single or batch), each non-notification message gets exactly one
// response frame. Messages are dispatched concurrently (WithMaxConcurrentCalls
// bounds the fan-out), so responses may arrive in any order — JSON-RPC
// correlates by id. Notifications produce no frame at all.
//
// The connection is bidirectional: a handler can push server-initiated
// notifications to the client by retrieving the jsonrpc.Pusher injected into
// its request context (jsonrpc.PusherFromContext). Pushes share the
// connection's serialized, time-bounded writer with responses. The client
// (DialClient) receives them via WithNotificationHandler.
//
// Browser handshakes are same-origin by default; allow specific origins with
// WithOriginPatterns. The handler does not implement authentication or
// per-client connection limits — both are application/load-balancer policy
// (wrap the handler, or authenticate the HTTP request before the upgrade).
//
// The WebSocket upgrade hijacks the connection: http.Server read/write
// timeouts no longer apply, and r.Context() is NOT canceled when the client
// drops (net/http cancels it only after the handler returns). The handler
// therefore manages the connection lifecycle itself: when the read side ends
// — client close, drop, or an oversized frame (WithMaxMessageSize, 1 MiB by
// default, close status 1009) — in-flight calls have their context canceled.
// Every response write runs under WithWriteTimeout (10s by default); a write
// that cannot complete (slow reader) tears the connection down instead of
// wedging it. Idle-connection policy (pings, deadlines) remains the
// application's responsibility.
package jsonrpcws

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
)

// DefaultMaxMessageSize bounds a single incoming frame unless overridden
// with WithMaxMessageSize.
const DefaultMaxMessageSize = 1 << 20 // 1 MiB

// DefaultMaxConcurrentCalls bounds how many messages of one connection are
// dispatched concurrently unless overridden with WithMaxConcurrentCalls.
const DefaultMaxConcurrentCalls = 16

// DefaultWriteTimeout bounds one response write unless overridden with
// WithWriteTimeout.
const DefaultWriteTimeout = 10 * time.Second

type handler struct {
	rpc            *jsonrpc.JSONRPC
	maxMessageSize int64
	maxCalls       int
	writeTimeout   time.Duration
	originPatterns []string
}

// Option configures the Handler.
type Option func(*handler)

// WithMaxMessageSize caps a single incoming frame in bytes; larger frames
// close the connection with status 1009 (message too big). Non-positive
// values keep the default.
func WithMaxMessageSize(n int64) Option {
	return func(h *handler) {
		if n > 0 {
			h.maxMessageSize = n
		}
	}
}

// WithMaxConcurrentCalls caps how many in-flight messages a single
// connection may have; further frames wait for a slot. Note the slot unit is
// a message, and one message may be a batch of up to the dispatcher's
// SetMaxBatchSize requests — lower this bound for batch-heavy workloads.
// With the dispatcher's SetEnforcedTimeout(true) the bound counts started
// requests: a handler that ignores cancellation keeps running after its
// slot is released, so keep the default inline timeout mode for connections
// exposed to untrusted peers. Non-positive values keep the default.
func WithMaxConcurrentCalls(n int) Option {
	return func(h *handler) {
		if n > 0 {
			h.maxCalls = n
		}
	}
}

// WithWriteTimeout bounds a single response write. A write that cannot
// finish in time (the client stopped reading) closes the connection —
// without this bound a slow reader would hold the write mutex and the
// concurrency slots forever. Non-positive values keep the default.
func WithWriteTimeout(d time.Duration) Option {
	return func(h *handler) {
		if d > 0 {
			h.writeTimeout = d
		}
	}
}

// WithOriginPatterns authorizes browser handshakes from the given host
// patterns (see websocket.AcceptOptions.OriginPatterns; "*" allows all —
// understand the CSRF implications before using it). Without this option
// only same-host handshakes are accepted.
func WithOriginPatterns(patterns ...string) Option {
	return func(h *handler) {
		h.originPatterns = append(h.originPatterns, patterns...)
	}
}

// Handler upgrades requests to WebSocket and serves JSON-RPC 2.0 frames.
func Handler(rpc *jsonrpc.JSONRPC, opts ...Option) http.Handler {
	h := &handler{
		rpc:            rpc,
		maxMessageSize: DefaultMaxMessageSize,
		maxCalls:       DefaultMaxConcurrentCalls,
		writeTimeout:   DefaultWriteTimeout,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: h.originPatterns,
	})
	if err != nil {
		// Accept has already written the handshake error response.
		return
	}
	defer conn.CloseNow() //nolint:errcheck // final cleanup, error is moot

	conn.SetReadLimit(h.maxMessageSize)

	// r.Context() is NOT canceled when a hijacked connection drops — net/http
	// cancels it only after ServeHTTP returns (and coder/websocket's own
	// deadline machinery hangs off the ctx we pass it). So the connection
	// runs its own lifecycle: r.Context() stays the parent for
	// request-scoped values, and cancel() fires as soon as the read side
	// ends, canceling in-flight handlers and unblocking writers.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// One writer for the connection: response frames and server-pushed
	// notifications both go through it, serialized and time-bounded. A write
	// failure tears the connection down so no goroutine wedges behind the
	// write mutex.
	cw := &connWriter{
		conn:    conn,
		baseCtx: ctx,
		timeout: h.writeTimeout,
		onError: func() { cancel(); _ = conn.CloseNow() },
	}
	pusher := &connPusher{w: cw}

	var wg sync.WaitGroup
	slots := make(chan struct{}, h.maxCalls)

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			// Client closed, dropped, read limit exceeded (1009 already
			// sent), or the connection was torn down by a failed write.
			break
		}

		slots <- struct{}{}
		wg.Add(1)
		go func(data []byte) {
			defer wg.Done()
			defer func() { <-slots }()

			// Inject the pusher so handlers can send server-initiated
			// notifications over this connection while they run.
			reqCtx := jsonrpc.ContextWithPusher(ctx, pusher)
			resp := h.rpc.HandleRPCJSONRawMessage(reqCtx, data)
			if len(resp) == 0 {
				return // notification: no response frame
			}
			_ = cw.write(resp)
		}(data)
	}

	// The read side ending — however it ended — closes the session:
	// in-flight calls are canceled rather than raced against an unreliable
	// delivery guarantee.
	cancel()
	wg.Wait()
	_ = conn.Close(websocket.StatusNormalClosure, "")
}

// interface guard
var _ http.Handler = (*handler)(nil)
