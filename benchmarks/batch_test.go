package benchmarks_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"testing"

	crjrpc2 "github.com/creachadair/jrpc2"
	"github.com/creachadair/jrpc2/channel"
	"github.com/creachadair/jrpc2/handler"

	"github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpcstdio"

	"github.com/gumeniukcom/golang-jsonrpc2/benchmarks/internal/arena"
)

// BenchmarkBatch measures server-side batch processing: one raw JSON-RPC
// batch frame of N sum calls is written to the server, and the single
// array response is read back and its entry count validated. The frame
// bytes are identical for both libraries (built once by the arena).
//
// Roster: ours and creachadair/jrpc2 — the only two contenders with native
// batch support. sourcegraph/jsonrpc2 documents batches as unsupported and
// net/rpc's jsonrpc codec is JSON-RPC 1.0 (no batches); they are reported
// as N/A in docs/BENCHMARKS.md rather than benchmarked against a shim.
func BenchmarkBatch(b *testing.B) {
	for _, size := range []int{10, 100} {
		frame := append(arena.BatchRequestJSON(size), '\n')

		b.Run(sizeName("ours", size), func(b *testing.B) {
			srvConn, cliConn := net.Pipe()
			rpc := ourRPC(b)
			done := make(chan struct{})
			go func() {
				defer close(done)
				_ = jsonrpcstdio.Serve(context.Background(), rpc, jsonrpcstdio.FramingNDJSON, srvConn, srvConn)
			}()
			b.Cleanup(func() { _ = cliConn.Close(); _ = srvConn.Close(); <-done })

			br := bufio.NewReaderSize(cliConn, 1<<20)
			runRawBatch(b, cliConn, br, frame, size)
		})

		b.Run(sizeName("creachadair", size), func(b *testing.B) {
			srvConn, cliConn := net.Pipe()
			srv := crjrpc2.NewServer(handler.Map{
				"sum": handler.New(func(_ context.Context, p arena.SumParams) (arena.SumResult, error) {
					return arena.Sum(p), nil
				}),
			}, nil)
			srv.Start(channel.Line(srvConn, srvConn))
			b.Cleanup(func() { _ = cliConn.Close(); _ = srvConn.Close(); srv.Stop() })

			br := bufio.NewReaderSize(cliConn, 1<<20)
			runRawBatch(b, cliConn, br, frame, size)
		})
	}
}

func sizeName(lib string, n int) string {
	if n == 10 {
		return lib + "/10"
	}
	return lib + "/100"
}

// runRawBatch drives one raw newline-framed batch round-trip per iteration
// and validates the response entry count.
func runRawBatch(b *testing.B, w net.Conn, br *bufio.Reader, frame []byte, want int) {
	b.Helper()
	do := func() {
		if _, err := w.Write(frame); err != nil {
			b.Fatal(err)
		}
		line, err := br.ReadBytes('\n')
		if err != nil {
			b.Fatal(err)
		}
		var entries []json.RawMessage
		if err := json.Unmarshal(line, &entries); err != nil {
			b.Fatalf("batch response does not parse: %v (%.80s)", err, line)
		}
		if len(entries) != want {
			b.Fatalf("got %d entries, want %d", len(entries), want)
		}
	}
	do() // prove the adapter works before measuring
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		do()
	}
}
