package jsonrpc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

type callLog struct {
	mu      sync.Mutex
	entries []string
}

func (l *callLog) add(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, s)
}

func (l *callLog) list() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.entries...)
}

func logging(l *callLog, tag string) Middleware {
	return func(methodName string, next RPCMethod) RPCMethod {
		return func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
			l.add(tag + ":before:" + methodName)
			res, code, err := next(ctx, data)
			l.add(tag + ":after:" + methodName)
			return res, code, err
		}
	}
}

// Middleware must wrap the handler with post-call capability, in
// first-registered-outermost order, and receive the method name.
func TestMiddlewareOrderAndPostCall(t *testing.T) {
	j := New()
	l := &callLog{}
	j.Use(logging(l, "a"))
	j.Use(logging(l, "b"))
	registerFast(t, j)

	resp := j.HandleRPCJSONRawMessage(context.Background(),
		json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"fast"}`))
	if !strings.Contains(string(resp), `"result"`) {
		t.Fatalf("expected success, got: %s", resp)
	}

	want := []string{"a:before:fast", "b:before:fast", "b:after:fast", "a:after:fast"}
	got := l.list()
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

// Middleware registered BEFORE a method must still apply to methods
// registered later, and vice versa.
func TestMiddlewareAppliesRegardlessOfRegistrationOrder(t *testing.T) {
	j := New()
	l := &callLog{}
	registerFast(t, j) // method first
	j.Use(logging(l, "mw"))

	if err := j.RegisterMethod("later", func(_ context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		return json.RawMessage(`1`), OK, nil
	}); err != nil {
		t.Fatal(err)
	}

	for _, m := range []string{"fast", "later"} {
		l.entries = nil
		j.HandleRPCJSONRawMessage(context.Background(),
			json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"`+m+`"}`))
		if len(l.list()) != 2 {
			t.Fatalf("middleware must wrap %q, log: %v", m, l.list())
		}
	}
}

// Middleware can short-circuit with an error that maps to an error response.
func TestMiddlewareShortCircuit(t *testing.T) {
	j := New()
	j.Use(func(_ string, _ RPCMethod) RPCMethod {
		return func(_ context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
			return nil, InvalidParamsErrorCode, NewRPCError(InvalidParamsErrorCode, errors.New("denied"))
		}
	})
	registerFast(t, j)

	resp := j.HandleRPCJSONRawMessage(context.Background(),
		json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"fast"}`))
	if !strings.Contains(string(resp), `"error"`) {
		t.Fatalf("middleware must be able to short-circuit, got: %s", resp)
	}
}

// Middleware can rewrite the successful result.
func TestMiddlewareRewritesResult(t *testing.T) {
	j := New()
	j.Use(func(_ string, next RPCMethod) RPCMethod {
		return func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
			res, code, err := next(ctx, data)
			if err == nil {
				res = json.RawMessage(`"rewritten"`)
			}
			return res, code, err
		}
	})
	registerFast(t, j)

	resp := j.HandleRPCJSONRawMessage(context.Background(),
		json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"fast"}`))
	if !strings.Contains(string(resp), `"rewritten"`) {
		t.Fatalf("expected rewritten result, got: %s", resp)
	}
}

// Middleware factories must re-run only when the registry (methods or
// middleware) changes — plain setters like SetLogger must not recompose.
func TestSettersDoNotRerunMiddlewareFactories(t *testing.T) {
	j := New()
	var factoryRuns int
	j.Use(func(_ string, next RPCMethod) RPCMethod {
		factoryRuns++
		return next
	})
	registerFast(t, j) // recompose: 1 run

	before := factoryRuns
	j.SetDefaultTimeOut(10 * time.Second)
	j.SetMaxBatchSize(50)
	j.SetLogger(nil)
	if factoryRuns != before {
		t.Fatalf("setters must not re-run middleware factories: %d -> %d", before, factoryRuns)
	}

	if err := j.RegisterMethod("second", func(_ context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		return json.RawMessage(`1`), OK, nil
	}); err != nil {
		t.Fatal(err)
	}
	if factoryRuns <= before {
		t.Fatal("registering a method must recompose")
	}

	// Dispatch still works after non-recomposing setters.
	resp := j.HandleRPCJSONRawMessage(context.Background(),
		json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"fast"}`))
	if !strings.Contains(string(resp), `"result"`) {
		t.Fatalf("dispatch broken after setters: %s", resp)
	}
}

// WithTimeout overrides the server default for one method only.
func TestPerMethodTimeoutOverridesDefault(t *testing.T) {
	j := New()
	j.SetDefaultTimeOut(30 * time.Second)

	err := RegisterTyped(j, "quick_limit", func(ctx context.Context, _ struct{}) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
			return "done", nil
		}
	}, WithTimeout(50*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	resp := j.HandleRPC(context.Background(),
		&structs.Request{Version: Version, Method: "quick_limit", ID: structs.ID("1")})
	if resp.Error == nil || resp.Error.Code != RequestTimeLimit {
		t.Fatalf("expected time-limit from per-method timeout, got: %+v", resp)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("per-method timeout must fire at ~50ms, took %v", elapsed)
	}
}

// A generous per-method timeout must survive a small server default.
func TestPerMethodTimeoutExtendsDefault(t *testing.T) {
	j := New()
	j.SetDefaultTimeOut(20 * time.Millisecond)

	err := RegisterTyped(j, "patient", func(ctx context.Context, _ struct{}) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(100 * time.Millisecond):
			return "done", nil
		}
	}, WithTimeout(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}

	resp := j.HandleRPC(context.Background(),
		&structs.Request{Version: Version, Method: "patient", ID: structs.ID("1")})
	if resp.Error != nil {
		t.Fatalf("per-method timeout must extend the default, got error: %+v", resp.Error)
	}
}

// Methods() must expose the configured per-method timeout.
func TestMethodsExposeTimeout(t *testing.T) {
	j := New()
	err := RegisterTyped(j, "m", func(_ context.Context, _ struct{}) (int, error) { return 0, nil },
		WithTimeout(7*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	mi := j.Methods()
	if len(mi) != 1 || mi[0].Timeout != 7*time.Second {
		t.Fatalf("expected Timeout=7s in MethodInfo, got: %+v", mi)
	}
}
