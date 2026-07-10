// Package arena defines the shared workload every benchmarked library must
// serve: identical handler logic, identical payloads, identical validation.
// Per-library code is limited to thin transport adapters in the _test.go
// files — anything that differs between libraries beyond wiring would make
// the comparison unfair.
package arena

import (
	"fmt"
	"strconv"
	"strings"
)

// SumParams / SumResult are the request/response shapes of the "sum"
// method, the small-call workload.
type SumParams struct {
	A int `json:"a"`
	B int `json:"b"`
}

type SumResult struct {
	Sum int `json:"sum"`
}

// Sum is the single source of the handler logic.
func Sum(p SumParams) SumResult { return SumResult{Sum: p.A + p.B} }

// EchoParams / EchoResult carry the large-payload workload.
type EchoParams struct {
	Data string `json:"data"`
}

type EchoResult struct {
	Data string `json:"data"`
}

// Echo is the single source of the echo handler logic.
func Echo(p EchoParams) EchoResult { return EchoResult(p) }

// LargePayload is the 10 KiB echo payload (ASCII, so JSON encoding adds no
// escaping and every library moves the same number of bytes).
func LargePayload() string { return strings.Repeat("x", 10<<10) }

// SumRequestJSON is a complete JSON-RPC 2.0 sum request frame with the
// given id, for message-level and raw-frame benchmarks.
func SumRequestJSON(id int) []byte {
	return []byte(`{"jsonrpc":"2.0","method":"sum","params":{"a":3,"b":4},"id":` + strconv.Itoa(id) + `}`)
}

// BatchRequestJSON is a JSON-RPC 2.0 batch of n sum requests (ids 1..n) as
// one frame, newline-free so every framing can carry it.
func BatchRequestJSON(n int) []byte {
	var sb strings.Builder
	sb.WriteByte('[')
	for i := 1; i <= n; i++ {
		if i > 1 {
			sb.WriteByte(',')
		}
		sb.Write(SumRequestJSON(i))
	}
	sb.WriteByte(']')
	return []byte(sb.String())
}

// WantSum is the expected result of the shared sum request (3+4).
const WantSum = 7

// CheckSum fails fast on a wrong answer — every benchmark validates every
// response, so dead-code elimination cannot fake a fast library and a
// broken adapter cannot ship a number.
func CheckSum(got int) error {
	if got != WantSum {
		return fmt.Errorf("sum = %d, want %d", got, WantSum)
	}
	return nil
}
