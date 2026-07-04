package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

func echoMethod(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
	return json.RawMessage(`"ok"`), OK, nil
}

func buildBatch(n int) string {
	reqs := make([]string, n)
	for i := range reqs {
		reqs[i] = fmt.Sprintf(`{"jsonrpc":"2.0","method":"echo","id":%d}`, i)
	}
	return "[" + strings.Join(reqs, ",") + "]"
}

func TestJSONRPC_MaxBatchSize(t *testing.T) {
	j := New()
	j.SetLogger(nil)
	j.SetMaxBatchSize(2)

	if err := j.RegisterMethod("echo", echoMethod); err != nil {
		t.Fatal(err)
	}

	t.Run("batch over the limit is rejected with a distinct message", func(t *testing.T) {
		res := j.HandleRPCJSONRawMessage(context.Background(), []byte(buildBatch(3)))
		if string(res) != string(errorBatchTooLarge()) {
			t.Errorf("expected batch_too_large, got %s", string(res))
		}
	})

	t.Run("batch at the limit is executed", func(t *testing.T) {
		res := j.HandleRPCJSONRawMessage(context.Background(), []byte(buildBatch(2)))
		var responses []structs.Response
		if err := json.Unmarshal(res, &responses); err != nil {
			t.Fatalf("expected batch response, got %s", string(res))
		}
		if len(responses) != 2 {
			t.Errorf("expected 2 responses, got %d", len(responses))
		}
	})

	t.Run("rejection happens before unmarshal", func(t *testing.T) {
		// Nested strings with brackets and escapes must not confuse the
		// pre-parse counter into rejecting a small batch.
		batch := `[{"jsonrpc":"2.0","method":"echo","id":"a[,]\"{"}, {"jsonrpc":"2.0","method":"echo","id":2}]`
		res := j.HandleRPCJSONRawMessage(context.Background(), []byte(batch))
		var responses []structs.Response
		if err := json.Unmarshal(res, &responses); err != nil {
			t.Fatalf("expected batch response, got %s", string(res))
		}
		if len(responses) != 2 {
			t.Errorf("expected 2 responses, got %d", len(responses))
		}
	})
}

func TestApproxBatchLen(t *testing.T) {
	tests := []struct {
		name  string
		input string
		limit int
		want  int
	}{
		{"empty array", `[]`, 10, 0},
		{"one element", `[{"a":1}]`, 10, 1},
		{"three elements with nesting", `[{"a":1},{"b":[1,2,3]},{"c":{"d":4}}]`, 10, 3},
		{"commas inside strings ignored", `[{"a":"x,y,[z]"},{"b":2}]`, 10, 2},
		{"escaped quotes", `[{"a":"say \",\" twice"},{"b":2}]`, 10, 2},
		{"early exit over limit", buildBatch(100), 5, 6},
		{"scalars", `[1, 2, 3]`, 10, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := approxBatchLen([]byte(tt.input), tt.limit); got != tt.want {
				t.Errorf("approxBatchLen(%s) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestJSONRPC_BatchConcurrency(t *testing.T) {
	j := New()
	j.SetLogger(nil)
	j.SetBatchConcurrency(2)

	var mu sync.Mutex
	var current, maxSeen int
	err := j.RegisterMethod("slow", func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		mu.Lock()
		current++
		if current > maxSeen {
			maxSeen = current
		}
		mu.Unlock()

		time.Sleep(20 * time.Millisecond)

		mu.Lock()
		current--
		mu.Unlock()
		return json.RawMessage(`"ok"`), OK, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	requests := make(structs.Requests, 10)
	for i := range requests {
		requests[i] = structs.Request{Version: Version, Method: "slow", ID: i}
	}

	resp := j.HandleBatchRPC(context.Background(), requests)
	if len(resp) != len(requests) {
		t.Fatalf("expected %d responses, got %d", len(requests), len(resp))
	}
	for i := range resp {
		if resp[i].Error != nil {
			t.Errorf("unexpected error for id=%v: %+v", resp[i].ID, resp[i].Error)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if maxSeen > 2 {
		t.Errorf("expected at most 2 concurrent executions, saw %d", maxSeen)
	}
}

func TestJSONRPC_BatchResponsesKeepRequestOrder(t *testing.T) {
	j := New()
	j.SetLogger(nil)
	j.SetBatchConcurrency(3)

	if err := j.RegisterMethod("echo", echoMethod); err != nil {
		t.Fatal(err)
	}

	requests := make(structs.Requests, 20)
	for i := range requests {
		requests[i] = structs.Request{Version: Version, Method: "echo", ID: i}
	}

	resp := j.HandleBatchRPC(context.Background(), requests)
	for i := range resp {
		if resp[i].ID != i {
			t.Fatalf("response %d has id %v, responses must keep request order", i, resp[i].ID)
		}
	}
}

func TestJSONRPC_BatchMarshalFailureKeepsGoodResponses(t *testing.T) {
	j := New()
	j.SetLogger(nil)

	const code = -5
	if err := j.RegisterError(code, "bad_data"); err != nil {
		t.Fatal(err)
	}
	if err := j.RegisterMethod("echo", echoMethod); err != nil {
		t.Fatal(err)
	}
	// NaN is not JSON-marshalable: this response entry cannot be serialized.
	err := j.RegisterMethod("nan", func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		return nil, code, NewRPCError(code, nil).WithData(math.NaN())
	})
	if err != nil {
		t.Fatal(err)
	}

	raw := j.HandleRPCJSONRawMessage(context.Background(), []byte(
		`[{"jsonrpc":"2.0","method":"echo","id":1},{"jsonrpc":"2.0","method":"nan","id":2}]`))

	var responses []structs.Response
	if err := json.Unmarshal(raw, &responses); err != nil {
		t.Fatalf("expected a batch response, got %s", string(raw))
	}
	if len(responses) != 2 {
		t.Fatalf("good responses must survive a sibling marshal failure, got %d entries: %s", len(responses), string(raw))
	}
	if responses[0].Error != nil || string(*responses[0].Result) != `"ok"` {
		t.Errorf("good entry corrupted: %s", string(raw))
	}
	if responses[1].Error == nil || responses[1].Error.Code != InternalErrorCode {
		t.Errorf("broken entry must degrade to internal_error with its id: %s", string(raw))
	}
	if responses[1].ID == nil {
		t.Errorf("broken entry must keep its request id: %s", string(raw))
	}
}
