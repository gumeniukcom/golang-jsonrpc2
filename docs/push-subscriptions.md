# Server push and subscriptions

WebSocket and stdio connections are bidirectional: a handler can send
server-initiated **notifications** (id-less requests) to the peer. That is
how LSP publishes diagnostics, how MCP announces resource updates, and how
you build subscription feeds. Plain HTTP has no push channel —
`PusherFromContext` reports `ok == false` there, so shared handlers degrade
gracefully.

## The Pusher contract

The transport injects a `jsonrpc.Pusher` into every request context:

```go
serv.RegisterMethod("subscribe", func(ctx context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
	p, ok := jsonrpc.PusherFromContext(ctx)
	if !ok {
		return nil, jsonrpc.MethodNotFoundErrorCode, errors.New("push not supported on this transport")
	}
	_ = p.Notify(ctx, "tick", map[string]int{"n": 1})
	return json.RawMessage(`"ok"`), jsonrpc.OK, nil
})
```

Rules that keep long-lived subscriptions correct:

1. **The pusher outlives the handler.** It stays valid for the life of the
   connection, so a subscription goroutine may keep calling `Notify` after
   the handler returned.
2. **Don't reuse the request context in that goroutine.** It cancels when
   the handler returns; use `context.Background()` (or your own lifecycle
   context). The transports deliberately ignore the passed ctx for the
   write itself, but passing a live one keeps your own code honest.
3. **`Notify` returning an error means the connection is gone.** That is
   the subscription's stop signal — a push loop must check it and exit,
   or it leaks a goroutine per disconnected client.
4. **Pushes and responses share one serialized writer** — frames never
   interleave, and you may push concurrently from several goroutines.

A complete subscription loop:

```go
serv.RegisterMethod("watch", func(ctx context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
	p, ok := jsonrpc.PusherFromContext(ctx)
	if !ok {
		return nil, jsonrpc.MethodNotFoundErrorCode, errors.New("push not supported")
	}
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for range t.C {
			if err := p.Notify(context.Background(), "watch.event", nextEvent()); err != nil {
				return // connection closed — stop pushing
			}
		}
	}()
	return json.RawMessage(`"watching"`), jsonrpc.OK, nil
})
```

For explicit `unsubscribe`, keep the subscription's cancel func in a
registry keyed by a subscription id you return to the client; `Notify`'s
error remains the backstop when the client just disconnects.

## Receiving pushes on the client

WebSocket and stdio clients deliver id-less frames to
`WithNotificationHandler`:

```go
c, _ := jsonrpcws.DialClient(ctx, "ws://localhost:8088/ws",
	jsonrpcws.WithNotificationHandler(func(method string, params json.RawMessage) {
		// runs on the read loop — return promptly, offload slow work
		events <- pushEvent{method, params}
	}))
```

`method` and `params` come straight from the server: treat them as
untrusted before mapping to any privileged local action. The handler runs
on the client's single read loop, so a blocking handler stalls responses to
pending calls — hand the payload to a channel and return.

## Transport notes

- **WebSocket**: pushes ride the same time-bounded writer as responses
  (`WithWriteTimeout`, 10s default); a peer that stops reading tears the
  connection down rather than wedging the server.
- **stdio**: after `Serve` returns, the writer is latched — a late `Notify`
  from a leftover goroutine gets an error instead of writing to a dead
  stdout (which would kill the process with SIGPIPE).
- **What push is not**: server-initiated *requests* (with a response coming
  back) are not supported — pushes are fire-and-forget notifications.
