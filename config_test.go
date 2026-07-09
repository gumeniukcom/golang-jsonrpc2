package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

func registerSleeper(t *testing.T, j *JSONRPC, name string, d time.Duration, respectCtx bool) {
	t.Helper()
	err := j.RegisterMethod(name, func(ctx context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		if respectCtx {
			select {
			case <-ctx.Done():
				return nil, InternalErrorCode, ctx.Err()
			case <-time.After(d):
			}
		} else {
			time.Sleep(d)
		}
		return json.RawMessage(`"done"`), OK, nil
	})
	if err != nil {
		t.Fatalf("register %q: %v", name, err)
	}
}

func buildFastBatch(n int) json.RawMessage {
	var sb strings.Builder
	sb.WriteByte('[')
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, `{"jsonrpc":"2.0","id":%d,"method":"fast","params":null}`, i+1)
	}
	sb.WriteByte(']')
	return json.RawMessage(sb.String())
}

func registerFast(t *testing.T, j *JSONRPC) {
	t.Helper()
	if err := j.RegisterMethod("fast", func(_ context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		return json.RawMessage(`"ok"`), OK, nil
	}); err != nil {
		t.Fatalf("register fast: %v", err)
	}
}

// New() must ship with DoS-safe defaults: a batch limit and a bounded batch
// concurrency, so an unconfigured server cannot be goroutine-amplified.
func TestDefaultBatchLimitRejectsOversizedBatch(t *testing.T) {
	j := New()
	registerFast(t, j)

	resp := j.HandleRPCJSONRawMessage(context.Background(), buildFastBatch(DefaultMaxBatchSize+1))
	if !strings.Contains(string(resp), "batch_too_large") {
		t.Fatalf("expected batch_too_large for %d requests, got: %s", DefaultMaxBatchSize+1, resp)
	}

	resp = j.HandleRPCJSONRawMessage(context.Background(), buildFastBatch(DefaultMaxBatchSize))
	if strings.Contains(string(resp), "batch_too_large") {
		t.Fatalf("batch of exactly %d must pass, got: %s", DefaultMaxBatchSize, resp)
	}
}

// SetMaxBatchSize(0) must restore the old unlimited behavior.
func TestBatchLimitZeroDisables(t *testing.T) {
	j := New()
	registerFast(t, j)
	j.SetMaxBatchSize(0)

	resp := j.HandleRPCJSONRawMessage(context.Background(), buildFastBatch(DefaultMaxBatchSize+50))
	if strings.Contains(string(resp), "batch_too_large") {
		t.Fatalf("unlimited batch must pass, got: %s", resp)
	}
	var batch []structs.Response
	if err := json.Unmarshal(resp, &batch); err != nil {
		t.Fatalf("unmarshal batch response: %v", err)
	}
	if len(batch) != DefaultMaxBatchSize+50 {
		t.Fatalf("expected %d responses, got %d", DefaultMaxBatchSize+50, len(batch))
	}
}

// A message above SetMaxMessageSize must be rejected before any parsing.
func TestMaxMessageSize(t *testing.T) {
	j := New()
	registerFast(t, j)
	j.SetMaxMessageSize(64)

	small := json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"fast"}`)
	if resp := j.HandleRPCJSONRawMessage(context.Background(), small); strings.Contains(string(resp), "request_too_large") {
		t.Fatalf("small request must pass, got: %s", resp)
	}

	big := json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"fast","params":"` + strings.Repeat("x", 128) + `"}`)
	resp := j.HandleRPCJSONRawMessage(context.Background(), big)
	if !strings.Contains(string(resp), "request_too_large") {
		t.Fatalf("expected request_too_large, got: %s", resp)
	}
}

// By default a message of any size is accepted (limit disabled).
func TestMaxMessageSizeDisabledByDefault(t *testing.T) {
	j := New()
	registerFast(t, j)

	big := json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"fast","params":"` + strings.Repeat("x", 1<<16) + `"}`)
	resp := j.HandleRPCJSONRawMessage(context.Background(), big)
	if strings.Contains(string(resp), "request_too_large") {
		t.Fatalf("size limit must be off by default, got: %s", resp)
	}
}

// Default (inline) mode: a handler that respects ctx returns promptly with a
// time-limit error once the deadline expires.
func TestInlineTimeoutHandlerRespectsContext(t *testing.T) {
	j := New()
	j.SetDefaultTimeOut(50 * time.Millisecond)
	registerSleeper(t, j, "slow", 5*time.Second, true)

	start := time.Now()
	resp := j.HandleRPC(context.Background(), &structs.Request{Version: Version, Method: "slow", ID: structs.ID("1")})
	elapsed := time.Since(start)

	if resp.Error == nil || resp.Error.Code != RequestTimeLimit {
		t.Fatalf("expected time-limit error, got: %+v", resp)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("ctx-respecting handler must return near the deadline, took %v", elapsed)
	}
}

// Default (inline) mode: a handler that ignores ctx delays the response, but
// the response is still a time-limit error because the deadline passed.
func TestInlineTimeoutHandlerIgnoresContext(t *testing.T) {
	j := New()
	j.SetDefaultTimeOut(20 * time.Millisecond)
	registerSleeper(t, j, "stubborn", 200*time.Millisecond, false)

	resp := j.HandleRPC(context.Background(), &structs.Request{Version: Version, Method: "stubborn", ID: structs.ID("1")})
	if resp.Error == nil || resp.Error.Code != RequestTimeLimit {
		t.Fatalf("expected time-limit error after late return, got: %+v", resp)
	}
}

// Enforced mode (opt-in): the time-limit response arrives at the deadline even
// when the handler ignores ctx and keeps running in the background.
func TestEnforcedTimeoutRespondsAtDeadline(t *testing.T) {
	j := New()
	j.SetDefaultTimeOut(50 * time.Millisecond)
	j.SetEnforcedTimeout(true)
	registerSleeper(t, j, "stubborn", 3*time.Second, false)

	start := time.Now()
	resp := j.HandleRPC(context.Background(), &structs.Request{Version: Version, Method: "stubborn", ID: structs.ID("1")})
	elapsed := time.Since(start)

	if resp.Error == nil || resp.Error.Code != RequestTimeLimit {
		t.Fatalf("expected time-limit error, got: %+v", resp)
	}
	if elapsed > 1*time.Second {
		t.Fatalf("enforced mode must respond at the deadline, took %v", elapsed)
	}
}

// Cancellation of the PARENT context must not be misreported as a time
// limit: a handler that completes its work keeps its successful response
// even if the caller's context was canceled mid-flight (e.g. client
// disconnect), and only a genuine deadline expiry produces RequestTimeLimit.
func TestParentCancelDoesNotBecomeTimeLimit(t *testing.T) {
	j := New()
	j.SetDefaultTimeOut(30 * time.Second)
	registerSleeper(t, j, "work", 100*time.Millisecond, false) // ignores ctx

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel() // client goes away while the handler is running
	}()

	resp := j.HandleRPC(ctx, &structs.Request{Version: Version, Method: "work", ID: structs.ID("1")})
	if resp.Error != nil {
		t.Fatalf("completed handler must keep its response on parent cancel, got error: %+v", resp.Error)
	}
	if resp.Result == nil || string(*resp.Result) != `"done"` {
		t.Fatalf("expected result \"done\", got: %+v", resp)
	}
}

// Config changes made while requests are in flight must be race-free and
// visible to subsequent requests (atomic snapshot semantics).
func TestConcurrentConfigMutation(t *testing.T) {
	j := New()
	registerFast(t, j)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			j.SetDefaultTimeOut(time.Duration(i+1) * time.Millisecond)
			j.SetMaxBatchSize(i + 1)
			_ = j.RegisterMethod(fmt.Sprintf("m%d", i), func(_ context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
				return json.RawMessage(`1`), OK, nil
			})
		}
	}()

	req := json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"fast"}`)
	for i := 0; i < 500; i++ {
		resp := j.HandleRPCJSONRawMessage(context.Background(), req)
		if !strings.Contains(string(resp), `"result"`) {
			t.Fatalf("request %d failed: %s", i, resp)
		}
	}
	<-done

	found := false
	for _, mi := range j.Methods() {
		if mi.Name == "m199" {
			found = true
		}
	}
	if !found {
		t.Fatal("method registered during traffic must be visible afterwards")
	}
}
