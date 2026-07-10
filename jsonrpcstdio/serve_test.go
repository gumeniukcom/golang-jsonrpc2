package jsonrpcstdio_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpcstdio"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

// bothFramings runs a subtest per framing so every server behavior is
// verified on both wire formats.
func bothFramings(t *testing.T, run func(t *testing.T, framing jsonrpcstdio.Framing)) {
	t.Helper()
	for _, framing := range []jsonrpcstdio.Framing{jsonrpcstdio.FramingContentLength, jsonrpcstdio.FramingNDJSON} {
		t.Run(framing.String(), func(t *testing.T) { run(t, framing) })
	}
}

// slowGate coordinates the "slow" test method: started signals the handler
// is running; closing release lets it finish.
type slowGate struct {
	started chan struct{}
	release chan struct{}
}

// echoRPC returns a dispatcher with an "echo" method that returns its params
// verbatim, plus a "slow" method gated on slowGate for ordering tests.
func echoRPC(t *testing.T) (*jsonrpc.JSONRPC, *slowGate) {
	t.Helper()
	j := jsonrpc.New()
	if err := j.RegisterMethod("echo", func(_ context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		if len(data) == 0 {
			data = json.RawMessage("null")
		}
		return data, jsonrpc.OK, nil
	}); err != nil {
		t.Fatal(err)
	}
	gate := &slowGate{started: make(chan struct{}, 1), release: make(chan struct{})}
	if err := j.RegisterMethod("slow", func(ctx context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		select {
		case gate.started <- struct{}{}:
		default:
		}
		select {
		case <-gate.release:
			return json.RawMessage(`"slow done"`), jsonrpc.OK, nil
		case <-ctx.Done():
			return nil, jsonrpc.InternalErrorCode, ctx.Err()
		}
	}); err != nil {
		t.Fatal(err)
	}
	return j, gate
}

// serveConn is one end-to-end server under test: the test talks to it over
// io.Pipe exactly like a peer process over stdin/stdout would.
type serveConn struct {
	framing jsonrpcstdio.Framing
	inW     *io.PipeWriter // test → server stdin
	outR    *bufio.Reader  // server stdout → test
	done    chan error

	waitOnce sync.Once
	waitErr  error
	waitOK   bool
}

// wait returns Serve's result, waiting for it once and memoizing it so the
// test body and t.Cleanup can both ask.
func (c *serveConn) wait(t *testing.T) error {
	t.Helper()
	c.waitOnce.Do(func() {
		select {
		case c.waitErr = <-c.done:
			c.waitOK = true
		case <-time.After(5 * time.Second):
		}
	})
	if !c.waitOK {
		t.Fatal("Serve did not return")
	}
	return c.waitErr
}

func startServe(t *testing.T, ctx context.Context, rpc *jsonrpc.JSONRPC, framing jsonrpcstdio.Framing, opts ...jsonrpcstdio.Option) *serveConn {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- jsonrpcstdio.Serve(ctx, rpc, framing, inR, outW, opts...)
	}()
	c := &serveConn{
		framing: framing,
		inW:     inW,
		outR:    bufio.NewReader(outR),
		done:    done,
	}
	t.Cleanup(func() {
		_ = inW.Close()  // EOF the server's stdin so the read loop ends
		_ = outR.Close() // release any response write still in flight
		_ = c.wait(t)    // memoized: the body may have consumed it already
	})
	return c
}

// send writes one framed message to the server's stdin.
func (c *serveConn) send(t *testing.T, msg string) {
	t.Helper()
	var frame string
	switch c.framing {
	case jsonrpcstdio.FramingContentLength:
		frame = "Content-Length: " + strconv.Itoa(len(msg)) + "\r\n\r\n" + msg
	case jsonrpcstdio.FramingNDJSON:
		frame = msg + "\n"
	}
	if _, err := io.WriteString(c.inW, frame); err != nil {
		t.Fatalf("send: %v", err)
	}
}

// recv reads one framed message from the server's stdout.
func (c *serveConn) recv(t *testing.T) string {
	t.Helper()
	type result struct {
		msg string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		msg, err := readWireFrame(c.outR, c.framing)
		ch <- result{msg, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("recv: %v", r.err)
		}
		return r.msg
	case <-time.After(5 * time.Second):
		t.Fatal("recv: timed out waiting for a frame")
		return ""
	}
}

// readWireFrame is a deliberately independent (test-local) frame reader, so
// server tests do not depend on the package's own client framing code.
func readWireFrame(br *bufio.Reader, framing jsonrpcstdio.Framing) (string, error) {
	if framing == jsonrpcstdio.FramingNDJSON {
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return "", err
			}
			line = strings.TrimRight(line, "\r\n")
			if line != "" {
				return line, nil
			}
		}
	}
	var contentLen = -1
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return "", err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if v, ok := strings.CutPrefix(line, "Content-Length: "); ok {
			if contentLen, err = strconv.Atoi(v); err != nil {
				return "", fmt.Errorf("bad Content-Length %q", v)
			}
		}
	}
	if contentLen < 0 {
		return "", errors.New("missing Content-Length in server output")
	}
	body := make([]byte, contentLen)
	if _, err := io.ReadFull(br, body); err != nil {
		return "", err
	}
	return string(body), nil
}

func mustUnmarshalResponse(t *testing.T, msg string) structs.Response {
	t.Helper()
	var resp structs.Response
	if err := resp.UnmarshalJSON([]byte(msg)); err != nil {
		t.Fatalf("response %q does not parse: %v", msg, err)
	}
	return resp
}

// A call gets exactly one correctly framed response with the same id.
func TestServeEcho(t *testing.T) {
	bothFramings(t, func(t *testing.T, framing jsonrpcstdio.Framing) {
		rpc, _ := echoRPC(t)
		c := startServe(t, context.Background(), rpc, framing)

		c.send(t, `{"jsonrpc":"2.0","method":"echo","params":{"a":1},"id":1}`)
		resp := mustUnmarshalResponse(t, c.recv(t))
		if string(resp.ID) != "1" || resp.Error != nil {
			t.Fatalf("unexpected response: id=%s err=%v", resp.ID, resp.Error)
		}
		if string(*resp.Result) != `{"a":1}` {
			t.Fatalf("echo result mismatch: %s", *resp.Result)
		}
	})
}

// A notification produces no bytes at all: the next frame on the wire is the
// follow-up call's response (spec: no response to id-less requests).
func TestServeNotificationWritesNothing(t *testing.T) {
	bothFramings(t, func(t *testing.T, framing jsonrpcstdio.Framing) {
		rpc, _ := echoRPC(t)
		c := startServe(t, context.Background(), rpc, framing)

		c.send(t, `{"jsonrpc":"2.0","method":"echo","params":"notify"}`)
		c.send(t, `{"jsonrpc":"2.0","method":"echo","params":"call","id":2}`)
		resp := mustUnmarshalResponse(t, c.recv(t))
		if string(resp.ID) != "2" {
			t.Fatalf("first wire frame must answer the call, got id=%s", resp.ID)
		}
	})
}

// A batch answers as a single array frame; notification entries are filtered
// out of it, and an all-notification batch writes nothing (spec §6).
func TestServeBatch(t *testing.T) {
	bothFramings(t, func(t *testing.T, framing jsonrpcstdio.Framing) {
		rpc, _ := echoRPC(t)
		c := startServe(t, context.Background(), rpc, framing)

		c.send(t, `[{"jsonrpc":"2.0","method":"echo","params":1,"id":1},{"jsonrpc":"2.0","method":"echo","params":2},{"jsonrpc":"2.0","method":"echo","params":3,"id":3}]`)
		frame := c.recv(t)
		var batch []structs.Response
		if err := json.Unmarshal([]byte(frame), &batch); err != nil {
			t.Fatalf("batch response must be one array frame, got %q: %v", frame, err)
		}
		if len(batch) != 2 {
			t.Fatalf("want 2 entries (notification filtered), got %d", len(batch))
		}

		// All-notification batch: nothing on the wire, next call answers first.
		c.send(t, `[{"jsonrpc":"2.0","method":"echo","params":1},{"jsonrpc":"2.0","method":"echo","params":2}]`)
		c.send(t, `{"jsonrpc":"2.0","method":"echo","id":9}`)
		resp := mustUnmarshalResponse(t, c.recv(t))
		if string(resp.ID) != "9" {
			t.Fatalf("all-notification batch must write nothing, got id=%s", resp.ID)
		}
	})
}

// Well-framed garbage draws -32700 and the stream SURVIVES: framing
// violations are fatal, protocol violations are not (design §2.4).
func TestServeMalformedJSONSurvives(t *testing.T) {
	bothFramings(t, func(t *testing.T, framing jsonrpcstdio.Framing) {
		rpc, _ := echoRPC(t)
		c := startServe(t, context.Background(), rpc, framing)

		c.send(t, `{"jsonrpc":]`)
		resp := mustUnmarshalResponse(t, c.recv(t))
		if resp.Error == nil || resp.Error.Code != jsonrpc.ParseErrorCode {
			t.Fatalf("want -32700, got %+v", resp)
		}

		c.send(t, `{"jsonrpc":"2.0","method":"echo","id":2}`)
		resp = mustUnmarshalResponse(t, c.recv(t))
		if string(resp.ID) != "2" || resp.Error != nil {
			t.Fatalf("stream must survive a parse error, got %+v", resp)
		}
	})
}

// An inbound response-shaped frame draws a -32600 reply — pinned behavior,
// not a silent drop (a v1 server never sends requests, so no response can
// be legitimate).
func TestServeResponseToNobody(t *testing.T) {
	bothFramings(t, func(t *testing.T, framing jsonrpcstdio.Framing) {
		rpc, _ := echoRPC(t)
		c := startServe(t, context.Background(), rpc, framing)

		c.send(t, `{"jsonrpc":"2.0","result":5,"id":7}`)
		resp := mustUnmarshalResponse(t, c.recv(t))
		if resp.Error == nil || resp.Error.Code != jsonrpc.InvalidRequestErrorCode {
			t.Fatalf("response-shaped frame must draw -32600, got %+v", resp)
		}
	})
}

// A corrupt Content-Length header block is fatal: Serve returns an error.
func TestServeCorruptHeaderFatal(t *testing.T) {
	rpc, _ := echoRPC(t)
	c := startServe(t, context.Background(), rpc, jsonrpcstdio.FramingContentLength)

	if _, err := io.WriteString(c.inW, "Not-A-Header-Line\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	if err := c.wait(t); err == nil || !strings.Contains(err.Error(), "invalid header line") {
		t.Fatalf("want fatal header error, got %v", err)
	}

	// A header block that ends without ever naming Content-Length is fatal too.
	rpc2, _ := echoRPC(t)
	c2 := startServe(t, context.Background(), rpc2, jsonrpcstdio.FramingContentLength)
	if _, err := io.WriteString(c2.inW, "Content-Type: application/json\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	if err := c2.wait(t); err == nil || !strings.Contains(err.Error(), "missing Content-Length") {
		t.Fatalf("want missing Content-Length error, got %v", err)
	}
}

// An oversized frame is fatal and the error names the limit and the option.
func TestServeOversizedFrameFatal(t *testing.T) {
	bothFramings(t, func(t *testing.T, framing jsonrpcstdio.Framing) {
		rpc, _ := echoRPC(t)
		c := startServe(t, context.Background(), rpc, framing, jsonrpcstdio.WithMaxMessageSize(64))

		big := `{"jsonrpc":"2.0","method":"echo","params":"` + strings.Repeat("a", 128) + `","id":1}`
		c.send(t, big)
		if err := c.wait(t); err == nil || !strings.Contains(err.Error(), "WithMaxMessageSize") {
			t.Fatalf("oversize error must name the option, got %v", err)
		}
	})
}

// Feeding an LSP stream to an NDJSON server fails fast with a framing hint
// instead of half-working (UX finding: silent cross-framing corruption).
func TestServeWrongFramingHint(t *testing.T) {
	rpc, _ := echoRPC(t)
	c := startServe(t, context.Background(), rpc, jsonrpcstdio.FramingNDJSON)

	if _, err := io.WriteString(c.inW, "Content-Length: 2\r\n\r\n{}"); err != nil {
		t.Fatal(err)
	}
	if err := c.wait(t); err == nil || !strings.Contains(err.Error(), "FramingContentLength") {
		t.Fatalf("want a framing hint, got %v", err)
	}
}

// Clean stdin EOF returns nil AFTER draining the in-flight handler and
// writing its response (graceful shutdown contract).
func TestServeCleanEOFDrainsInFlight(t *testing.T) {
	bothFramings(t, func(t *testing.T, framing jsonrpcstdio.Framing) {
		rpc, gate := echoRPC(t)
		c := startServe(t, context.Background(), rpc, framing, jsonrpcstdio.WithMaxConcurrentCalls(2))

		c.send(t, `{"jsonrpc":"2.0","method":"slow","id":1}`)
		<-gate.started    // the handler is running
		_ = c.inW.Close() // EOF while slow is in flight
		close(gate.release)

		resp := mustUnmarshalResponse(t, c.recv(t))
		if string(resp.ID) != "1" || resp.Error != nil {
			t.Fatalf("in-flight response must be written before Serve returns, got %+v", resp)
		}
		if err := c.wait(t); err != nil {
			t.Fatalf("clean EOF must return nil, got %v", err)
		}
	})
}

// Canceling ctx (then closing the reader, per the documented escape hatch)
// makes Serve return ctx.Err(), matchable with errors.Is.
func TestServeContextCancel(t *testing.T) {
	rpc, _ := echoRPC(t)
	ctx, cancel := context.WithCancel(context.Background())
	c := startServe(t, ctx, rpc, jsonrpcstdio.FramingNDJSON)

	cancel()
	_ = c.inW.Close() // unblock the pipe read
	if err := c.wait(t); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

// Under the default sequential dispatch, responses keep request order even
// when the first request is slower (LSP ordering rule).
func TestServeSequentialOrdering(t *testing.T) {
	rpc, gate := echoRPC(t)
	c := startServe(t, context.Background(), rpc, jsonrpcstdio.FramingNDJSON)

	c.send(t, `{"jsonrpc":"2.0","method":"slow","id":1}`)
	<-gate.started
	// The dispatcher is inline in the read loop, so it is not reading while
	// slow runs — the second send must come from a goroutine (io.Pipe has no
	// buffer) and cannot even be READ, let alone answered, before slow ends.
	go func() { _, _ = io.WriteString(c.inW, `{"jsonrpc":"2.0","method":"echo","id":2}`+"\n") }()
	close(gate.release)

	if id := string(mustUnmarshalResponse(t, c.recv(t)).ID); id != "1" {
		t.Fatalf("sequential dispatch must answer in request order, got id=%s first", id)
	}
	if id := string(mustUnmarshalResponse(t, c.recv(t)).ID); id != "2" {
		t.Fatalf("want id=2 second, got %s", id)
	}
}

// With WithMaxConcurrentCalls(2) a fast request overtakes a slow one:
// responses may reorder, correlation is by id.
func TestServeConcurrentReordering(t *testing.T) {
	rpc, gate := echoRPC(t)
	c := startServe(t, context.Background(), rpc, jsonrpcstdio.FramingNDJSON, jsonrpcstdio.WithMaxConcurrentCalls(2))

	c.send(t, `{"jsonrpc":"2.0","method":"slow","id":1}`)
	<-gate.started
	c.send(t, `{"jsonrpc":"2.0","method":"echo","id":2}`)

	// echo answers while slow is still blocked — deterministic proof of
	// reordering, no sleeps involved.
	if id := string(mustUnmarshalResponse(t, c.recv(t)).ID); id != "2" {
		t.Fatalf("fast call must overtake the blocked one, got id=%s", id)
	}
	close(gate.release)
	if id := string(mustUnmarshalResponse(t, c.recv(t)).ID); id != "1" {
		t.Fatalf("want id=1 second, got %s", id)
	}
}

// Serve validates its arguments instead of guessing.
func TestServeArgumentValidation(t *testing.T) {
	rpc := jsonrpc.New()
	r, w := io.Pipe()
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })

	tests := []struct {
		name    string
		rpc     *jsonrpc.JSONRPC
		framing jsonrpcstdio.Framing
		r       io.Reader
		w       io.Writer
		wantMsg string
	}{
		{"nil rpc", nil, jsonrpcstdio.FramingNDJSON, r, w, "rpc must not be nil"},
		{"zero framing", rpc, jsonrpcstdio.Framing(0), r, w, "FramingContentLength or FramingNDJSON"},
		{"unknown framing", rpc, jsonrpcstdio.Framing(9), r, w, "FramingContentLength or FramingNDJSON"},
		{"nil reader", rpc, jsonrpcstdio.FramingNDJSON, nil, w, "must not be nil"},
		{"nil writer", rpc, jsonrpcstdio.FramingNDJSON, r, nil, "must not be nil"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := jsonrpcstdio.Serve(context.Background(), tt.rpc, tt.framing, tt.r, tt.w)
			if err == nil || !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("want error containing %q, got %v", tt.wantMsg, err)
			}
		})
	}
}

// Regression: a handler smuggling pretty-printed JSON into its RawMessage
// result must still get its response delivered over NDJSON (compacted), not
// kill the whole session.
func TestServeIndentedResultSurvivesNDJSON(t *testing.T) {
	j := jsonrpc.New()
	if err := j.RegisterMethod("pretty", func(_ context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		return json.RawMessage("{\n  \"a\": 1\n}"), jsonrpc.OK, nil
	}); err != nil {
		t.Fatal(err)
	}
	c := startServe(t, context.Background(), j, jsonrpcstdio.FramingNDJSON)

	c.send(t, `{"jsonrpc":"2.0","method":"pretty","id":1}`)
	resp := mustUnmarshalResponse(t, c.recv(t))
	if resp.Error != nil || string(*resp.Result) != `{"a":1}` {
		t.Fatalf("indented result must arrive compacted, got %+v", resp)
	}

	// The stream must still be alive afterwards.
	c.send(t, `{"jsonrpc":"2.0","method":"pretty","id":2}`)
	if id := string(mustUnmarshalResponse(t, c.recv(t)).ID); id != "2" {
		t.Fatalf("stream must survive, got id=%s", id)
	}
}

// Concurrent handlers responding while others push notifications never
// interleave frames — the -race payoff test for the serialized writer.
func TestServeConcurrentPushesAndResponsesRace(t *testing.T) {
	const calls = 8
	j := jsonrpc.New()
	if err := j.RegisterMethod("chatty", func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		p, ok := jsonrpc.PusherFromContext(ctx)
		if !ok {
			return nil, jsonrpc.InternalErrorCode, errors.New("no pusher")
		}
		for i := 0; i < 3; i++ {
			if err := p.Notify(ctx, "tick", map[string]int{"n": i}); err != nil {
				return nil, jsonrpc.InternalErrorCode, err
			}
		}
		return data, jsonrpc.OK, nil
	}); err != nil {
		t.Fatal(err)
	}

	bothFramings(t, func(t *testing.T, framing jsonrpcstdio.Framing) {
		c := startServe(t, context.Background(), j, framing, jsonrpcstdio.WithMaxConcurrentCalls(4))

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < calls; i++ {
				c.send(t, `{"jsonrpc":"2.0","method":"chatty","params":`+strconv.Itoa(i)+`,"id":`+strconv.Itoa(i)+`}`)
			}
		}()

		responses, ticks := 0, 0
		for i := 0; i < calls*4; i++ {
			frame := c.recv(t)
			// Every frame must parse cleanly: interleaved writes would break this.
			var probe struct {
				Method string          `json:"method"`
				ID     json.RawMessage `json:"id"`
			}
			if err := json.Unmarshal([]byte(frame), &probe); err != nil {
				t.Fatalf("frame %d corrupted (%q): %v", i, frame, err)
			}
			if probe.Method == "tick" {
				ticks++
			} else {
				responses++
			}
		}
		wg.Wait()
		if responses != calls || ticks != calls*3 {
			t.Fatalf("want %d responses and %d ticks, got %d and %d", calls, calls*3, responses, ticks)
		}
	})
}
