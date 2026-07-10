package benchmarks_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"testing"

	crjrpc2 "github.com/creachadair/jrpc2"
	"github.com/creachadair/jrpc2/channel"
	"github.com/creachadair/jrpc2/handler"
	sgjrpc2 "github.com/sourcegraph/jsonrpc2"

	gjrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpcstdio"

	"github.com/gumeniukcom/golang-jsonrpc2/benchmarks/internal/arena"
)

// Every adapter returns the same shape: a call function that issues one
// "sum" request end-to-end and returns the numeric result, plus a cleanup.
// The adapters must stay thin — handler logic lives in internal/arena.
type pipeAdapter struct {
	name string
	// setup wires a server and client over net.Pipe and returns callSum.
	setup func(b *testing.B) (callSum func() (int, error), cleanup func())
}

// ourRPC builds our dispatcher with the shared arena handlers, logging off
// (every library runs silent — fairness rule #3).
func ourRPC(b *testing.B) *gjrpc.JSONRPC {
	b.Helper()
	rpc := gjrpc.New()
	rpc.SetLogger(nil)
	if err := gjrpc.RegisterTyped(rpc, "sum", func(_ context.Context, p arena.SumParams) (arena.SumResult, error) {
		return arena.Sum(p), nil
	}); err != nil {
		b.Fatal(err)
	}
	if err := gjrpc.RegisterTyped(rpc, "echo", func(_ context.Context, p arena.EchoParams) (arena.EchoResult, error) {
		return arena.Echo(p), nil
	}); err != nil {
		b.Fatal(err)
	}
	return rpc
}

// --- ours: jsonrpcstdio (NDJSON framing) over net.Pipe -------------------

func setupOurs(b *testing.B) (func() (int, error), func()) {
	b.Helper()
	srvConn, cliConn := net.Pipe()
	rpc := ourRPC(b)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = jsonrpcstdio.Serve(context.Background(), rpc, jsonrpcstdio.FramingNDJSON, srvConn, srvConn)
	}()
	client, err := jsonrpcstdio.NewClient(jsonrpcstdio.FramingNDJSON, cliConn, cliConn)
	if err != nil {
		b.Fatal(err)
	}

	call := func() (int, error) {
		res, err := gjrpc.CallResult[arena.SumResult](context.Background(), client, "sum", arena.SumParams{A: 3, B: 4})
		return res.Sum, err
	}
	cleanup := func() {
		_ = client.Close()
		_ = cliConn.Close()
		_ = srvConn.Close()
		<-done
	}
	return call, cleanup
}

// --- sourcegraph/jsonrpc2 (PlainObjectCodec) over net.Pipe ---------------

type sgHandler struct{}

func (sgHandler) Handle(ctx context.Context, conn *sgjrpc2.Conn, req *sgjrpc2.Request) {
	switch req.Method {
	case "sum":
		var p arena.SumParams
		if req.Params != nil {
			if err := json.Unmarshal(*req.Params, &p); err != nil {
				_ = conn.ReplyWithError(ctx, req.ID, &sgjrpc2.Error{Code: sgjrpc2.CodeInvalidParams, Message: err.Error()})
				return
			}
		}
		_ = conn.Reply(ctx, req.ID, arena.Sum(p))
	case "echo":
		var p arena.EchoParams
		if req.Params != nil {
			if err := json.Unmarshal(*req.Params, &p); err != nil {
				_ = conn.ReplyWithError(ctx, req.ID, &sgjrpc2.Error{Code: sgjrpc2.CodeInvalidParams, Message: err.Error()})
				return
			}
		}
		_ = conn.Reply(ctx, req.ID, arena.Echo(p))
	default:
		_ = conn.ReplyWithError(ctx, req.ID, &sgjrpc2.Error{Code: sgjrpc2.CodeMethodNotFound, Message: "not found"})
	}
}

// sgDiscardLogger silences sourcegraph's default stderr logger (fairness
// rule: no library logs during a run).
var sgDiscardLogger = sgjrpc2.SetLogger(log.New(io.Discard, "", 0))

func setupSourcegraph(b *testing.B) (func() (int, error), func()) {
	return setupSourcegraphHandler(b, sgHandler{})
}

// setupSourcegraphAsync wraps the handler in sourcegraph's AsyncHandler —
// its idiomatic concurrent-dispatch path, used in the parallel arena so the
// library is not unfairly capped at one in-flight handler.
func setupSourcegraphAsync(b *testing.B) (func() (int, error), func()) {
	return setupSourcegraphHandler(b, sgjrpc2.AsyncHandler(sgHandler{}))
}

func setupSourcegraphHandler(b *testing.B, h sgjrpc2.Handler) (func() (int, error), func()) {
	b.Helper()
	srvConn, cliConn := net.Pipe()
	ctx := context.Background()

	srv := sgjrpc2.NewConn(ctx, sgjrpc2.NewBufferedStream(srvConn, sgjrpc2.PlainObjectCodec{}), h, sgDiscardLogger)
	cli := sgjrpc2.NewConn(ctx, sgjrpc2.NewBufferedStream(cliConn, sgjrpc2.PlainObjectCodec{}), noopSGHandler{}, sgDiscardLogger)

	call := func() (int, error) {
		var res arena.SumResult
		if err := cli.Call(ctx, "sum", arena.SumParams{A: 3, B: 4}, &res); err != nil {
			return 0, err
		}
		return res.Sum, nil
	}
	cleanup := func() {
		_ = cli.Close()
		_ = srv.Close()
	}
	return call, cleanup
}

type noopSGHandler struct{}

func (noopSGHandler) Handle(context.Context, *sgjrpc2.Conn, *sgjrpc2.Request) {}

// --- creachadair/jrpc2 (channel.Line) over net.Pipe ----------------------

func setupCreachadair(b *testing.B) (func() (int, error), func()) {
	b.Helper()
	srvConn, cliConn := net.Pipe()

	srv := crjrpc2.NewServer(handler.Map{
		"sum": handler.New(func(_ context.Context, p arena.SumParams) (arena.SumResult, error) {
			return arena.Sum(p), nil
		}),
		"echo": handler.New(func(_ context.Context, p arena.EchoParams) (arena.EchoResult, error) {
			return arena.Echo(p), nil
		}),
	}, nil)
	srv.Start(channel.Line(srvConn, srvConn))
	cli := crjrpc2.NewClient(channel.Line(cliConn, cliConn), nil)

	call := func() (int, error) {
		var res arena.SumResult
		if err := cli.CallResult(context.Background(), "sum", arena.SumParams{A: 3, B: 4}, &res); err != nil {
			return 0, err
		}
		return res.Sum, nil
	}
	cleanup := func() {
		_ = cli.Close()
		srv.Stop()
	}
	return call, cleanup
}

// --- stdlib net/rpc + jsonrpc codec (JSON-RPC 1.0!) over net.Pipe --------

type NetRPCService struct{}

func (NetRPCService) Sum(p arena.SumParams, r *arena.SumResult) error {
	*r = arena.Sum(p)
	return nil
}

func (NetRPCService) Echo(p arena.EchoParams, r *arena.EchoResult) error {
	*r = arena.Echo(p)
	return nil
}

func setupNetRPC(b *testing.B) (func() (int, error), func()) {
	b.Helper()
	srvConn, cliConn := net.Pipe()

	srv := rpc.NewServer()
	if err := srv.RegisterName("arena", NetRPCService{}); err != nil {
		b.Fatal(err)
	}
	go srv.ServeCodec(jsonrpc.NewServerCodec(srvConn))
	cli := rpc.NewClientWithCodec(jsonrpc.NewClientCodec(cliConn))

	call := func() (int, error) {
		var res arena.SumResult
		if err := cli.Call("arena.Sum", arena.SumParams{A: 3, B: 4}, &res); err != nil {
			return 0, err
		}
		return res.Sum, nil
	}
	cleanup := func() {
		_ = cli.Close()
		_ = srvConn.Close()
	}
	return call, cleanup
}

// --- hand-rolled encoding/json dispatcher (the "no library" floor) -------

type rawRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      json.RawMessage `json:"id"`
}

type rawResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  any             `json:"result,omitempty"`
	Error   *rawError       `json:"error,omitempty"`
	ID      json.RawMessage `json:"id"`
}

type rawError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// baselineDispatch is the message-level floor: decode, switch, encode.
func baselineDispatch(data []byte) []byte {
	var req rawRequest
	if err := json.Unmarshal(data, &req); err != nil {
		out, _ := json.Marshal(rawResponse{JSONRPC: "2.0", Error: &rawError{Code: -32700, Message: "parse error"}, ID: json.RawMessage("null")})
		return out
	}
	var (
		result any
		rerr   *rawError
	)
	switch req.Method {
	case "sum":
		var p arena.SumParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			rerr = &rawError{Code: -32602, Message: "invalid params"}
		} else {
			result = arena.Sum(p)
		}
	case "echo":
		var p arena.EchoParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			rerr = &rawError{Code: -32602, Message: "invalid params"}
		} else {
			result = arena.Echo(p)
		}
	default:
		rerr = &rawError{Code: -32601, Message: "method not found"}
	}
	out, _ := json.Marshal(rawResponse{JSONRPC: "2.0", Result: result, Error: rerr, ID: req.ID})
	return out
}

func setupBaseline(b *testing.B) (func() (int, error), func()) {
	b.Helper()
	srvConn, cliConn := net.Pipe()

	go func() { // NDJSON server loop
		sc := bufio.NewScanner(srvConn)
		sc.Buffer(make([]byte, 64<<10), 16<<20)
		w := bufio.NewWriter(srvConn)
		for sc.Scan() {
			resp := baselineDispatch(sc.Bytes())
			_, _ = w.Write(resp)
			_ = w.WriteByte('\n')
			_ = w.Flush()
		}
	}()

	br := bufio.NewReader(cliConn)
	id := 0
	call := func() (int, error) {
		id++
		if _, err := cliConn.Write(append(arena.SumRequestJSON(id), '\n')); err != nil {
			return 0, err
		}
		line, err := br.ReadBytes('\n')
		if err != nil {
			return 0, err
		}
		var resp struct {
			Result arena.SumResult `json:"result"`
			Error  *rawError       `json:"error"`
		}
		if err := json.Unmarshal(line, &resp); err != nil {
			return 0, err
		}
		if resp.Error != nil {
			return 0, fmt.Errorf("error %d", resp.Error.Code)
		}
		return resp.Result.Sum, nil
	}
	cleanup := func() {
		_ = cliConn.Close()
		_ = srvConn.Close()
	}
	return call, cleanup
}

// pipeAdapters is the cross-library roster of the end-to-end arena.
var pipeAdapters = []pipeAdapter{
	{"ours", setupOurs},
	{"sourcegraph", setupSourcegraph},
	{"creachadair", setupCreachadair},
	{"netrpc10", setupNetRPC}, // JSON-RPC 1.0 — stdlib floor, not a 2.0 peer
	{"baseline", setupBaseline},
}
