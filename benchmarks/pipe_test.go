package benchmarks_test

import (
	"testing"

	"github.com/gumeniukcom/golang-jsonrpc2/benchmarks/internal/arena"
)

// BenchmarkPipe is the headline cross-library arena: one complete JSON-RPC
// call — client encodes and writes, server reads, dispatches, and writes
// back, client reads and decodes — over an in-process net.Pipe, so no
// network stack or TLS distorts the comparison. Every response is
// validated (fairness rule: no library gets to skip work).
//
// netrpc10 is stdlib net/rpc with its jsonrpc codec — JSON-RPC 1.0, not a
// 2.0 peer; it is included as the "what does stdlib cost" floor, and
// baseline is a hand-rolled encoding/json dispatcher — the "no library at
// all" floor.
func BenchmarkPipe(b *testing.B) {
	for _, ad := range pipeAdapters {
		b.Run(ad.name, func(b *testing.B) {
			call, cleanup := ad.setup(b)
			b.Cleanup(cleanup)

			// One warm-up call proves the adapter works before measuring.
			got, err := call()
			if err != nil {
				b.Fatal(err)
			}
			if err := arena.CheckSum(got); err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				got, err := call()
				if err != nil {
					b.Fatal(err)
				}
				if err := arena.CheckSum(got); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkPipeParallel runs concurrent callers against one connection for
// the libraries whose clients multiplex (ours, sourcegraph, creachadair,
// net/rpc). The baseline's trivial client is single-caller by design and
// is excluded.
func BenchmarkPipeParallel(b *testing.B) {
	// sourcegraph dispatches its handler synchronously on the read loop; the
	// parallel arena uses its AsyncHandler wrapper (the library's idiomatic
	// concurrent path) so it is not unfairly capped at one in-flight call.
	parallelAdapters := []pipeAdapter{
		{"ours", setupOurs},
		{"sourcegraph", setupSourcegraphAsync},
		{"creachadair", setupCreachadair},
		{"netrpc10", setupNetRPC},
	}
	for _, ad := range parallelAdapters {
		b.Run(ad.name, func(b *testing.B) {
			call, cleanup := ad.setup(b)
			b.Cleanup(cleanup)

			if got, err := call(); err != nil {
				b.Fatal(err)
			} else if err := arena.CheckSum(got); err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					got, err := call()
					if err != nil {
						// b.Fatal must not be called from RunParallel worker
						// goroutines; Error+return is the documented-safe way.
						b.Error(err)
						return
					}
					if err := arena.CheckSum(got); err != nil {
						b.Error(err)
						return
					}
				}
			})
		})
	}
}
