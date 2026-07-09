package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
)

// Benchmarks for the dispatch hot path. Run with:
//
//	make bench
//
// CI compares these against master with benchstat on every PR.

func newBenchServer(b *testing.B) *JSONRPC {
	b.Helper()
	j := New()
	j.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := j.RegisterMethod("sum", func(_ context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		var p struct {
			A int `json:"a"`
			B int `json:"b"`
		}
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, InvalidParamsErrorCode, err
		}
		res, err := json.Marshal(p.A + p.B)
		if err != nil {
			return nil, InternalErrorCode, err
		}
		return res, OK, nil
	}); err != nil {
		b.Fatal(err)
	}
	if err := j.RegisterMethod("echo", func(_ context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		return data, OK, nil
	}); err != nil {
		b.Fatal(err)
	}
	if err := RegisterTyped(j, "sum_typed", func(_ context.Context, p struct {
		A int `json:"a"`
		B int `json:"b"`
	}) (int, error) {
		return p.A + p.B, nil
	}); err != nil {
		b.Fatal(err)
	}
	return j
}

var benchSingleReq = json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"sum","params":{"a":3,"b":4}}`)

func benchBatchReq(n int) json.RawMessage {
	var sb strings.Builder
	sb.WriteByte('[')
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, `{"jsonrpc":"2.0","id":%d,"method":"sum","params":{"a":%d,"b":4}}`, i+1, i)
	}
	sb.WriteByte(']')
	return json.RawMessage(sb.String())
}

func BenchmarkSingleRequest(b *testing.B) {
	j := newBenchServer(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if out := j.HandleRPCJSONRawMessage(ctx, benchSingleReq); len(out) == 0 {
			b.Fatal("empty response")
		}
	}
}

func BenchmarkSingleRequestParallel(b *testing.B) {
	j := newBenchServer(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if out := j.HandleRPCJSONRawMessage(ctx, benchSingleReq); len(out) == 0 {
				b.Fatal("empty response")
			}
		}
	})
}

func BenchmarkTypedRequest(b *testing.B) {
	j := newBenchServer(b)
	req := json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"sum_typed","params":{"a":3,"b":4}}`)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if out := j.HandleRPCJSONRawMessage(ctx, req); len(out) == 0 {
			b.Fatal("empty response")
		}
	}
}

func BenchmarkNotification(b *testing.B) {
	j := newBenchServer(b)
	req := json.RawMessage(`{"jsonrpc":"2.0","method":"sum","params":{"a":3,"b":4}}`)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if out := j.HandleRPCJSONRawMessage(ctx, req); out != nil {
			b.Fatal("notification must not produce a response")
		}
	}
}

func BenchmarkBatch10(b *testing.B)  { benchBatch(b, 10) }
func BenchmarkBatch100(b *testing.B) { benchBatch(b, 100) }

func benchBatch(b *testing.B, n int) {
	j := newBenchServer(b)
	req := benchBatchReq(n)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if out := j.HandleRPCJSONRawMessage(ctx, req); len(out) == 0 {
			b.Fatal("empty response")
		}
	}
}

func BenchmarkLargeParams10KB(b *testing.B) {
	j := newBenchServer(b)
	req := json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"echo","params":{"blob":"` +
		strings.Repeat("x", 10*1024) + `"}}`)
	ctx := context.Background()
	b.SetBytes(int64(len(req)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if out := j.HandleRPCJSONRawMessage(ctx, req); len(out) == 0 {
			b.Fatal("empty response")
		}
	}
}

func BenchmarkInvalidRequest(b *testing.B) {
	j := newBenchServer(b)
	req := json.RawMessage(`{"jsonrpc":"1.0","id":1,"method":"","params":{}}`)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if out := j.HandleRPCJSONRawMessage(ctx, req); len(out) == 0 {
			b.Fatal("empty response")
		}
	}
}
