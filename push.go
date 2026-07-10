package jsonrpc

import "context"

// Pusher lets a handler send server-initiated notifications to the client
// over a bidirectional transport (e.g. WebSocket). It is transport-provided:
// the transport injects one into each request context with ContextWithPusher,
// and handlers retrieve it with PusherFromContext. Notify sends a
// notification — no id, no response — so it never blocks on a reply.
//
// A Pusher is valid for the life of the underlying connection, not just the
// request that produced it: a handler may keep it and call Notify from a
// background goroutine to drive a long-lived subscription after it returned.
// Notify returns an error once the connection is gone, which is the signal to
// stop the subscription. A push loop MUST stop on that error — ignoring it
// spins after the peer is gone, and a loop running inside the RPC method's
// goroutine would also hold a dispatch slot and block connection shutdown.
// (Do not, however, retain the handler's ctx for that goroutine — it is
// request-scoped and cancels when the handler returns; use a fresh context.)
//
// Not every transport supports push (plain HTTP request/response does not);
// handlers must treat PusherFromContext's second return as "push available"
// and degrade gracefully when it is false.
type Pusher interface {
	Notify(ctx context.Context, method string, params any) error
}

type pusherKey struct{}

// ContextWithPusher returns a context carrying p, for a transport to install
// before dispatching a request.
func ContextWithPusher(ctx context.Context, p Pusher) context.Context {
	return context.WithValue(ctx, pusherKey{}, p)
}

// PusherFromContext returns the Pusher a transport installed, if any. The
// second result is false when the transport does not support server push.
func PusherFromContext(ctx context.Context) (Pusher, bool) {
	p, ok := ctx.Value(pusherKey{}).(Pusher)
	return p, ok && p != nil
}
