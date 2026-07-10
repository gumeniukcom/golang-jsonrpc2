package jsonrpcstdio_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpcstdio"
)

// ExampleServe is the shape of an MCP-style stdio server: NDJSON framing
// over the process's stdin/stdout.
//
// Deliberately compile-only (no Output comment): with one, `go test` would
// execute it and block reading the terminal's stdin.
func ExampleServe() {
	rpc := jsonrpc.New()
	_ = jsonrpc.RegisterTyped(rpc, "ping", func(_ context.Context, _ struct{}) (string, error) {
		return "pong", nil
	})

	// stdout carries the protocol; log to stderr only (slog's default).
	if err := jsonrpcstdio.Serve(context.Background(), rpc, jsonrpcstdio.FramingNDJSON, os.Stdin, os.Stdout); err != nil {
		os.Exit(1)
	}
}

// ExampleServe_contentLength is the shape of an LSP-style stdio server:
// Content-Length framing, larger frames allowed for document payloads.
//
// Deliberately compile-only (no Output comment): with one, `go test` would
// execute it and block reading the terminal's stdin.
func ExampleServe_contentLength() {
	rpc := jsonrpc.New()
	_ = jsonrpc.RegisterTyped(rpc, "initialize", func(_ context.Context, _ json.RawMessage) (map[string]any, error) {
		return map[string]any{"capabilities": map[string]any{}}, nil
	})

	err := jsonrpcstdio.Serve(context.Background(), rpc, jsonrpcstdio.FramingContentLength, os.Stdin, os.Stdout,
		jsonrpcstdio.WithMaxMessageSize(32<<20))
	if err != nil {
		os.Exit(1)
	}
}

// ExampleNewClient wires a client to a server over in-process pipes — the
// same way you would wire it to a child process's StdoutPipe/StdinPipe.
// This one runs under `go test` (loopback, no real stdin involved).
func ExampleNewClient() {
	rpc := jsonrpc.New()
	_ = jsonrpc.RegisterTyped(rpc, "sum", func(_ context.Context, nums []int) (int, error) {
		total := 0
		for _, n := range nums {
			total += n
		}
		return total, nil
	})

	cliR, srvW := io.Pipe() // server stdout → client
	srvR, cliW := io.Pipe() // client → server stdin
	done := make(chan error, 1)
	go func() {
		done <- jsonrpcstdio.Serve(context.Background(), rpc, jsonrpcstdio.FramingNDJSON, srvR, srvW)
	}()

	c, err := jsonrpcstdio.NewClient(jsonrpcstdio.FramingNDJSON, cliR, cliW)
	if err != nil {
		panic(err)
	}

	sum, err := jsonrpc.CallResult[int](context.Background(), c, "sum", []int{1, 2, 3})
	if err != nil {
		panic(err)
	}
	fmt.Println(sum)

	_ = c.Close()
	_ = cliW.Close() // close the server's stdin: orderly shutdown
	fmt.Println(<-done)
	_ = cliR.Close()
	// Output:
	// 6
	// <nil>
}
