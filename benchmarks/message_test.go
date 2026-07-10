package benchmarks_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/gumeniukcom/golang-jsonrpc2/benchmarks/internal/arena"
)

// BenchmarkMessage is the in-process, message-level arena: raw request
// bytes in, raw response bytes out, no transport at all. Only our
// dispatcher and the hand-rolled encoding/json baseline have this entry
// point — the other libraries are coupled to their streams, and inventing
// one for them would not be a fair measurement.
func BenchmarkMessage(b *testing.B) {
	req := arena.SumRequestJSON(1)

	b.Run("ours", func(b *testing.B) {
		rpc := ourRPC(b)
		ctx := context.Background()
		if resp := rpc.HandleRPCJSONRawMessage(ctx, req); !bytes.Contains(resp, []byte(`"sum":7`)) {
			b.Fatalf("unexpected response: %s", resp)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if resp := rpc.HandleRPCJSONRawMessage(ctx, req); len(resp) == 0 {
				b.Fatal("empty response")
			}
		}
	})

	b.Run("baseline", func(b *testing.B) {
		if resp := baselineDispatch(req); !bytes.Contains(resp, []byte(`"sum":7`)) {
			b.Fatalf("unexpected response: %s", resp)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if resp := baselineDispatch(req); len(resp) == 0 {
				b.Fatal("empty response")
			}
		}
	})
}
