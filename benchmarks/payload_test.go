package benchmarks_test

import (
	"context"
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

// BenchmarkPayload10KiB echoes a 10 KiB string through each library over
// net.Pipe — the large-payload arena. b.SetBytes reports throughput; the
// response length is validated every iteration.
func BenchmarkPayload10KiB(b *testing.B) {
	payload := arena.LargePayload()
	params := arena.EchoParams{Data: payload}

	type echoCall func() (int, error) // returns len(echoed data)

	adapters := []struct {
		name  string
		setup func(b *testing.B) (echoCall, func())
	}{
		{"ours", func(b *testing.B) (echoCall, func()) {
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
			return func() (int, error) {
					res, err := gjrpc.CallResult[arena.EchoResult](context.Background(), client, "echo", params)
					return len(res.Data), err
				}, func() {
					_ = client.Close()
					_ = cliConn.Close()
					_ = srvConn.Close()
					<-done
				}
		}},
		{"sourcegraph", func(b *testing.B) (echoCall, func()) {
			srvConn, cliConn := net.Pipe()
			ctx := context.Background()
			srv := sgjrpc2.NewConn(ctx, sgjrpc2.NewBufferedStream(srvConn, sgjrpc2.PlainObjectCodec{}), sgHandler{}, sgDiscardLogger)
			cli := sgjrpc2.NewConn(ctx, sgjrpc2.NewBufferedStream(cliConn, sgjrpc2.PlainObjectCodec{}), noopSGHandler{}, sgDiscardLogger)
			return func() (int, error) {
					var res arena.EchoResult
					if err := cli.Call(ctx, "echo", params, &res); err != nil {
						return 0, err
					}
					return len(res.Data), nil
				}, func() {
					_ = cli.Close()
					_ = srv.Close()
				}
		}},
		{"creachadair", func(b *testing.B) (echoCall, func()) {
			srvConn, cliConn := net.Pipe()
			srv := crjrpc2.NewServer(handler.Map{
				"echo": handler.New(func(_ context.Context, p arena.EchoParams) (arena.EchoResult, error) {
					return arena.Echo(p), nil
				}),
			}, nil)
			srv.Start(channel.Line(srvConn, srvConn))
			cli := crjrpc2.NewClient(channel.Line(cliConn, cliConn), nil)
			return func() (int, error) {
					var res arena.EchoResult
					if err := cli.CallResult(context.Background(), "echo", params, &res); err != nil {
						return 0, err
					}
					return len(res.Data), nil
				}, func() {
					_ = cli.Close()
					srv.Stop()
				}
		}},
		{"netrpc10", func(b *testing.B) (echoCall, func()) {
			srvConn, cliConn := net.Pipe()
			srv := rpc.NewServer()
			if err := srv.RegisterName("arena", NetRPCService{}); err != nil {
				b.Fatal(err)
			}
			go srv.ServeCodec(jsonrpc.NewServerCodec(srvConn))
			cli := rpc.NewClientWithCodec(jsonrpc.NewClientCodec(cliConn))
			return func() (int, error) {
					var res arena.EchoResult
					if err := cli.Call("arena.Echo", params, &res); err != nil {
						return 0, err
					}
					return len(res.Data), nil
				}, func() {
					_ = cli.Close()
					_ = srvConn.Close()
				}
		}},
	}

	// The request and response each carry the payload once; SetBytes counts
	// the round-trip payload volume so ns/op converts to MB/s.
	wireBytes := int64(2 * len(payload))

	for _, ad := range adapters {
		b.Run(ad.name, func(b *testing.B) {
			call, cleanup := ad.setup(b)
			b.Cleanup(cleanup)

			if n, err := call(); err != nil {
				b.Fatal(err)
			} else if n != len(payload) {
				b.Fatalf("echoed %d bytes, want %d", n, len(payload))
			}

			b.SetBytes(wireBytes)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				n, err := call()
				if err != nil {
					b.Fatal(err)
				}
				if n != len(payload) {
					b.Fatalf("echoed %d bytes, want %d", n, len(payload))
				}
			}
		})
	}
}
