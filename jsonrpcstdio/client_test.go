package jsonrpcstdio_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpcstdio"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

// startLoopback wires our client to our Serve over two io.Pipes — the
// strongest hermetic interop test: both ends must agree on the framing.
func startLoopback(t *testing.T, rpc *jsonrpc.JSONRPC, framing jsonrpcstdio.Framing, copts ...jsonrpcstdio.ClientOption) *jsonrpcstdio.Client {
	t.Helper()
	cliR, srvW := io.Pipe() // server stdout → client
	srvR, cliW := io.Pipe() // client → server stdin
	done := make(chan error, 1)
	go func() {
		done <- jsonrpcstdio.Serve(context.Background(), rpc, framing, srvR, srvW)
	}()
	c, err := jsonrpcstdio.NewClient(framing, cliR, cliW, copts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = c.Close()
		_ = cliW.Close() // EOF the server's stdin: orderly shutdown
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Serve returned %v on orderly shutdown", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Serve did not return")
		}
		_ = cliR.Close() // release the client's read loop
	})
	return c
}

// fakePeer lets a test script the exact frames the client sees (NDJSON).
type fakePeer struct {
	in  *bufio.Reader  // client → peer
	out *io.PipeWriter // peer → client
}

func newFakePeer(t *testing.T) (*jsonrpcstdio.Client, *fakePeer) {
	return newFakePeerOpts(t, nil)
}

func newFakePeerWithHandler(t *testing.T, h jsonrpcstdio.NotificationHandler) (*jsonrpcstdio.Client, *fakePeer) {
	return newFakePeerOpts(t, h)
}

func newFakePeerOpts(t *testing.T, h jsonrpcstdio.NotificationHandler, extra ...jsonrpcstdio.ClientOption) (*jsonrpcstdio.Client, *fakePeer) {
	t.Helper()
	cliR, peerW := io.Pipe()
	peerR, cliW := io.Pipe()
	opts := extra
	if h != nil {
		opts = append(opts, jsonrpcstdio.WithNotificationHandler(h))
	}
	c, err := jsonrpcstdio.NewClient(jsonrpcstdio.FramingNDJSON, cliR, cliW, opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = c.Close()
		_ = peerW.Close()
		_ = cliW.Close()
		_ = cliR.Close()
		_ = peerR.Close()
	})
	return c, &fakePeer{in: bufio.NewReader(peerR), out: peerW}
}

// expectRequest reads one request line from the client.
func (p *fakePeer) expectRequest(t *testing.T) structs.Request {
	t.Helper()
	line, err := p.in.ReadString('\n')
	if err != nil {
		t.Fatalf("fake peer read: %v", err)
	}
	var req structs.Request
	if err := req.UnmarshalJSON([]byte(strings.TrimRight(line, "\n"))); err != nil {
		t.Fatalf("client sent unparsable frame %q: %v", line, err)
	}
	return req
}

// sendLine writes one NDJSON frame to the client.
func (p *fakePeer) sendLine(t *testing.T, s string) {
	t.Helper()
	if _, err := io.WriteString(p.out, s+"\n"); err != nil {
		t.Fatalf("fake peer write: %v", err)
	}
}

// Loopback: Call, Notify, and server push all work over both framings with
// our own Serve on the other end.
func TestClientLoopback(t *testing.T) {
	j := jsonrpc.New()
	if err := j.RegisterMethod("sum", func(_ context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		var nums []int
		if err := json.Unmarshal(data, &nums); err != nil {
			return nil, jsonrpc.InvalidParamsErrorCode, err
		}
		total := 0
		for _, n := range nums {
			total += n
		}
		raw, _ := json.Marshal(total)
		return raw, jsonrpc.OK, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := j.RegisterMethod("announce", func(ctx context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		p, ok := jsonrpc.PusherFromContext(ctx)
		if !ok {
			return nil, jsonrpc.InternalErrorCode, errors.New("no pusher")
		}
		if err := p.Notify(ctx, "announced", "hello"); err != nil {
			return nil, jsonrpc.InternalErrorCode, err
		}
		return json.RawMessage(`true`), jsonrpc.OK, nil
	}); err != nil {
		t.Fatal(err)
	}

	bothFramings(t, func(t *testing.T, framing jsonrpcstdio.Framing) {
		pushed := make(chan string, 1)
		c := startLoopback(t, j, framing,
			jsonrpcstdio.WithNotificationHandler(func(method string, params json.RawMessage) {
				pushed <- method + ":" + string(params)
			}))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		got, err := jsonrpc.CallResult[int](ctx, c, "sum", []int{1, 2, 3})
		if err != nil {
			t.Fatal(err)
		}
		if got != 6 {
			t.Fatalf("sum = %d, want 6", got)
		}

		// Notifications reach the server (no response expected): prove it by
		// the follow-up call still working.
		if err := c.Notify(ctx, "sum", []int{1}); err != nil {
			t.Fatal(err)
		}

		if _, err := c.Call(ctx, "announce", nil); err != nil {
			t.Fatal(err)
		}
		select {
		case msg := <-pushed:
			if msg != `announced:"hello"` {
				t.Fatalf("unexpected push %q", msg)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("push never delivered")
		}

		// A JSON-RPC error response surfaces as *structs.Error.
		_, err = c.Call(ctx, "nope", nil)
		var rpcErr *structs.Error
		if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.MethodNotFoundErrorCode {
			t.Fatalf("want method-not-found *structs.Error, got %v", err)
		}
	})
}

// Responses delivered out of order still reach the right callers: JSON-RPC
// correlates by id, not arrival order.
func TestClientOutOfOrderCorrelation(t *testing.T) {
	c, peer := newFakePeer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	type res struct {
		val json.RawMessage
		err error
	}
	first := make(chan res, 1)
	go func() {
		v, err := c.Call(ctx, "a", nil)
		first <- res{v, err}
	}()
	reqA := peer.expectRequest(t)

	second := make(chan res, 1)
	go func() {
		v, err := c.Call(ctx, "b", nil)
		second <- res{v, err}
	}()
	reqB := peer.expectRequest(t)

	// Answer B first, then A.
	peer.sendLine(t, `{"jsonrpc":"2.0","result":"for-b","id":`+string(reqB.ID)+`}`)
	peer.sendLine(t, `{"jsonrpc":"2.0","result":"for-a","id":`+string(reqA.ID)+`}`)

	if r := <-second; r.err != nil || string(r.val) != `"for-b"` {
		t.Fatalf("call b got (%s, %v)", r.val, r.err)
	}
	if r := <-first; r.err != nil || string(r.val) != `"for-a"` {
		t.Fatalf("call a got (%s, %v)", r.val, r.err)
	}
}

// Frames with unknown ids and array frames are dropped without disturbing
// the pending call.
func TestClientUnknownFramesDropped(t *testing.T) {
	c, peer := newFakePeer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		v, err := c.Call(ctx, "x", nil)
		if err == nil && string(v) != `"real"` {
			err = errors.New("wrong result " + string(v))
		}
		done <- err
	}()
	req := peer.expectRequest(t)

	peer.sendLine(t, `{"jsonrpc":"2.0","result":"bogus","id":424242}`) // unknown id
	peer.sendLine(t, `[{"jsonrpc":"2.0","result":"array","id":1}]`)    // batch frame: we sent no batch
	peer.sendLine(t, `{"jsonrpc":"2.0","result":"real","id":`+string(req.ID)+`}`)

	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// A top-level id:null error cannot be correlated, so it fails every pending
// call with the *structs.Error instead of letting them hang.
func TestClientNullIDErrorFailsPending(t *testing.T) {
	c, peer := newFakePeer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := c.Call(ctx, "x", nil)
		done <- err
	}()
	peer.expectRequest(t)
	peer.sendLine(t, `{"jsonrpc":"2.0","error":{"code":-32700,"message":"parse_error"},"id":null}`)

	err := <-done
	var rpcErr *structs.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.ParseErrorCode {
		t.Fatalf("want the id:null *structs.Error, got %v", err)
	}
}

// Close fails the pending call promptly and later calls are rejected; the
// read-loop goroutine is released by closing the stream, per the documented
// contract.
func TestClientCloseFailsPending(t *testing.T) {
	c, peer := newFakePeer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := c.Call(ctx, "x", nil)
		done <- err
	}()
	peer.expectRequest(t) // the call is on the wire and pending
	_ = c.Close()

	if err := <-done; !errors.Is(err, jsonrpcstdio.ErrClientClosed) {
		t.Fatalf("pending call must fail with ErrClientClosed, got %v", err)
	}
	if _, err := c.Call(ctx, "y", nil); !errors.Is(err, jsonrpcstdio.ErrClientClosed) {
		t.Fatalf("call after Close must fail with ErrClientClosed, got %v", err)
	}
	if err := c.Notify(ctx, "y", nil); !errors.Is(err, jsonrpcstdio.ErrClientClosed) {
		t.Fatalf("notify after Close must fail with ErrClientClosed, got %v", err)
	}
}

// Stream EOF (the peer died) closes the client and fails pending calls.
func TestClientStreamEOFClosesClient(t *testing.T) {
	c, peer := newFakePeer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := c.Call(ctx, "x", nil)
		done <- err
	}()
	peer.expectRequest(t)
	_ = peer.out.Close() // peer's stdout dies → client read loop EOFs

	if err := <-done; !errors.Is(err, jsonrpcstdio.ErrClientClosed) {
		t.Fatalf("want ErrClientClosed after stream EOF, got %v", err)
	}
}

// ctx cancellation abandons the wait; the late response is dropped by the
// read loop without disturbing anything.
func TestClientCallContextCanceled(t *testing.T) {
	c, peer := newFakePeer(t)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := c.Call(ctx, "x", nil)
		done <- err
	}()
	req := peer.expectRequest(t)
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	// The late response must be dropped silently (id already unregistered).
	peer.sendLine(t, `{"jsonrpc":"2.0","result":"late","id":`+string(req.ID)+`}`)

	// The client is still usable afterwards.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	ok := make(chan error, 1)
	go func() {
		v, err := c.Call(ctx2, "y", nil)
		if err == nil && string(v) != `"fine"` {
			err = errors.New("wrong result")
		}
		ok <- err
	}()
	req2 := peer.expectRequest(t)
	peer.sendLine(t, `{"jsonrpc":"2.0","result":"fine","id":`+string(req2.ID)+`}`)
	if err := <-ok; err != nil {
		t.Fatal(err)
	}
}

// NewClient validates its arguments.
func TestNewClientArgumentValidation(t *testing.T) {
	r, w := io.Pipe()
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })

	tests := []struct {
		name    string
		framing jsonrpcstdio.Framing
		r       io.Reader
		w       io.Writer
		wantMsg string
	}{
		{"zero framing", jsonrpcstdio.Framing(0), r, w, "FramingContentLength or FramingNDJSON"},
		{"unknown framing", jsonrpcstdio.Framing(9), r, w, "FramingContentLength or FramingNDJSON"},
		{"nil reader", jsonrpcstdio.FramingNDJSON, nil, w, "must not be nil"},
		{"nil writer", jsonrpcstdio.FramingNDJSON, r, nil, "must not be nil"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := jsonrpcstdio.NewClient(tt.framing, tt.r, tt.w)
			if err == nil || !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("want error containing %q, got %v", tt.wantMsg, err)
			}
		})
	}
}

// Regression: a mid-frame write failure permanently desynchronizes the
// outbound stream, so it must latch the client closed — later calls fail
// fast instead of appending frames after an orphaned header.
func TestClientWriteFailureLatches(t *testing.T) {
	cliR, _ := io.Pipe()
	t.Cleanup(func() { _ = cliR.Close() })
	// Fails on the second Write call: the Content-Length header lands, the
	// body does not — exactly the desync scenario.
	fw := &failingWriter{failAt: 2}
	c, err := jsonrpcstdio.NewClient(jsonrpcstdio.FramingContentLength, cliR, fw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := c.Call(ctx, "x", nil); err == nil {
		t.Fatal("the failing write must surface an error")
	}
	// The client must now be closed, with the write failure as the cause.
	_, err = c.Call(ctx, "y", nil)
	if !errors.Is(err, jsonrpcstdio.ErrClientClosed) {
		t.Fatalf("want ErrClientClosed after a mid-frame write failure, got %v", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("the close error must carry the write-failure cause, got %v", err)
	}
}

type failingWriter struct {
	n      int
	failAt int
}

func (w *failingWriter) Write(p []byte) (int, error) {
	w.n++
	if w.n >= w.failAt {
		return 0, errors.New("boom")
	}
	return len(p), nil
}

// Regression: after Close a still-live stream must not keep feeding the
// notification handler.
func TestClientCloseStopsNotificationDelivery(t *testing.T) {
	delivered := make(chan string, 16)
	c, peer := newFakePeerWithHandler(t, func(method string, _ json.RawMessage) {
		delivered <- method
	})

	peer.sendLine(t, `{"jsonrpc":"2.0","method":"before"}`)
	select {
	case m := <-delivered:
		if m != "before" {
			t.Fatalf("got %q", m)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first notification never delivered")
	}

	_ = c.Close()
	peer.sendLine(t, `{"jsonrpc":"2.0","method":"after"}`)
	// The frame may still be read from the pipe, but must not be delivered.
	select {
	case m := <-delivered:
		t.Fatalf("notification %q delivered after Close", m)
	case <-time.After(200 * time.Millisecond):
	}
}

// Regression: a server-initiated REQUEST whose id collides with one of our
// in-flight calls must not be delivered as a fabricated empty response.
func TestClientServerRequestFrameNotMisdelivered(t *testing.T) {
	c, peer := newFakePeer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		v, err := c.Call(ctx, "x", nil)
		if err == nil && string(v) != `"real"` {
			err = errors.New("wrong result " + string(v))
		}
		done <- err
	}()
	req := peer.expectRequest(t)

	// Reverse call from the peer, reusing OUR id — decodes as a response
	// with neither result nor error; must be dropped, not delivered.
	peer.sendLine(t, `{"jsonrpc":"2.0","id":`+string(req.ID)+`,"method":"roots/list"}`)
	// A null result is still a legitimate response shape and must pass the
	// same probe — prove it is not over-filtered by answering for real.
	peer.sendLine(t, `{"jsonrpc":"2.0","result":"real","id":`+string(req.ID)+`}`)

	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// Regression: a fatal inbound framing error (oversized frame) must be
// observable through the failed calls, not collapse into a bare "closed".
func TestClientFatalReadErrorSurfacesCause(t *testing.T) {
	c, peer := newFakePeerOpts(t, nil, jsonrpcstdio.WithClientMaxMessageSize(64))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := c.Call(ctx, "x", nil)
		done <- err
	}()
	peer.expectRequest(t)
	peer.sendLine(t, `{"jsonrpc":"2.0","result":"`+strings.Repeat("a", 256)+`","id":1}`)

	err := <-done
	if !errors.Is(err, jsonrpcstdio.ErrClientClosed) {
		t.Fatalf("want ErrClientClosed, got %v", err)
	}
	if !strings.Contains(err.Error(), "WithClientMaxMessageSize") {
		t.Fatalf("the cause (limit hint) must be observable, got %v", err)
	}
}

// Concurrent calls over one client stay correlated under -race.
func TestClientConcurrentCallsRace(t *testing.T) {
	j := jsonrpc.New()
	if err := j.RegisterMethod("echo", func(_ context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		return data, jsonrpc.OK, nil
	}); err != nil {
		t.Fatal(err)
	}
	c := startLoopback(t, j, jsonrpcstdio.FramingNDJSON)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			want, _ := json.Marshal(n)
			got, err := c.Call(ctx, "echo", n)
			if err != nil {
				t.Errorf("call %d: %v", n, err)
				return
			}
			if string(got) != string(want) {
				t.Errorf("call %d: got %s", n, got)
			}
		}(i)
	}
	wg.Wait()
}
