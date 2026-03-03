package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

type ctxKey string

func TestInterceptor_ContextChaining(t *testing.T) {
	j := New()

	// First interceptor sets a value in context
	j.RegisterGlobalInterceptorCall(func(ctx context.Context, methodName string, data json.RawMessage, id any) (context.Context, int, error) {
		return context.WithValue(ctx, ctxKey("step"), "first"), OK, nil
	})

	// Second interceptor reads the value set by first and adds its own
	j.RegisterGlobalInterceptorCall(func(ctx context.Context, methodName string, data json.RawMessage, id any) (context.Context, int, error) {
		val, ok := ctx.Value(ctxKey("step")).(string)
		if !ok || val != "first" {
			return ctx, InternalErrorCode, fmt.Errorf("expected context value 'first', got %q", val)
		}
		return context.WithValue(ctx, ctxKey("step"), "second"), OK, nil
	})

	// Method verifies it received the context from the second interceptor
	err := j.RegisterMethod("test", func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		val, ok := ctx.Value(ctxKey("step")).(string)
		if !ok || val != "second" {
			return nil, InternalErrorCode, fmt.Errorf("expected context value 'second', got %q", val)
		}
		return []byte(`"ok"`), OK, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	resp := j.HandleRPC(context.Background(), &structs.Request{
		Version: Version,
		Method:  "test",
		Params:  []byte("{}"),
		ID:      1,
	})

	if resp.Error != nil {
		t.Errorf("expected no error, got code=%d msg=%s data=%v", resp.Error.Code, resp.Error.Message, resp.Error.Data)
	}
}

func TestInterceptor_Abort(t *testing.T) {
	j := New()

	// Interceptor that always aborts
	j.RegisterGlobalInterceptorCall(func(ctx context.Context, methodName string, data json.RawMessage, id any) (context.Context, int, error) {
		return ctx, InvalidRequestErrorCode, fmt.Errorf("blocked by interceptor")
	})

	err := j.RegisterMethod("test", func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		t.Error("method should not have been called")
		return nil, OK, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	resp := j.HandleRPC(context.Background(), &structs.Request{
		Version: Version,
		Method:  "test",
		Params:  []byte("{}"),
		ID:      1,
	})

	if resp.Error == nil {
		t.Fatal("expected error response")
	}
	if resp.Error.Code != InvalidRequestErrorCode {
		t.Errorf("expected error code %d, got %d", InvalidRequestErrorCode, resp.Error.Code)
	}
}

func TestInterceptor_MultipleAbortOnFirst(t *testing.T) {
	j := New()
	secondCalled := false

	j.RegisterGlobalInterceptorCall(func(ctx context.Context, methodName string, data json.RawMessage, id any) (context.Context, int, error) {
		return ctx, InternalErrorCode, fmt.Errorf("first interceptor error")
	})

	j.RegisterGlobalInterceptorCall(func(ctx context.Context, methodName string, data json.RawMessage, id any) (context.Context, int, error) {
		secondCalled = true
		return ctx, OK, nil
	})

	err := j.RegisterMethod("test", func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		return []byte(`"ok"`), OK, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	resp := j.HandleRPC(context.Background(), &structs.Request{
		Version: Version,
		Method:  "test",
		Params:  []byte("{}"),
		ID:      1,
	})

	if resp.Error == nil {
		t.Fatal("expected error response")
	}
	if secondCalled {
		t.Error("second interceptor should not have been called")
	}
}
