package jsonrpc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func TestRPCError_Error(t *testing.T) {
	t.Run("with wrapped error", func(t *testing.T) {
		e := NewRPCError(-1, fmt.Errorf("boom"))
		if got := e.Error(); got != "jsonrpc error -1: boom" {
			t.Errorf("unexpected Error(): %q", got)
		}
	})

	t.Run("without wrapped error", func(t *testing.T) {
		e := &RPCError{Code: -2}
		if got := e.Error(); got != "jsonrpc error -2" {
			t.Errorf("unexpected Error(): %q", got)
		}
	})
}

func TestRPCError_Unwrap(t *testing.T) {
	sentinel := errors.New("sentinel")
	e := NewRPCError(-1, fmt.Errorf("wrap: %w", sentinel))

	if !errors.Is(e, sentinel) {
		t.Errorf("errors.Is should reach the wrapped sentinel")
	}

	wrapped := fmt.Errorf("outer: %w", e)
	var target *RPCError
	if !errors.As(wrapped, &target) {
		t.Fatalf("errors.As should find *RPCError through wrapping")
	}
	if target.Code != -1 {
		t.Errorf("expected code -1, got %d", target.Code)
	}
}

func TestRPCError_WithData(t *testing.T) {
	e := NewRPCError(-1, nil).WithData(map[string]string{"field": "email"})
	if e.Data == nil {
		t.Errorf("WithData should set Data")
	}
}

func TestRPCError_WithData_DoesNotMutateReceiver(t *testing.T) {
	// Package-level sentinel pattern: WithData must return a copy so shared
	// sentinels cannot race or leak data across concurrent requests.
	sentinel := NewRPCError(-1, nil)

	withData := sentinel.WithData("request-specific")
	if sentinel.Data != nil {
		t.Errorf("WithData mutated the shared sentinel: %v", sentinel.Data)
	}
	if withData.Data != "request-specific" {
		t.Errorf("copy should carry the data, got %v", withData.Data)
	}
	if withData == sentinel {
		t.Error("WithData must return a copy, not the receiver")
	}
}

func TestJSONRPC_Error_NoLeak(t *testing.T) {
	j := New()
	j.SetLogger(nil)

	t.Run("plain error text is not sent to the client", func(t *testing.T) {
		resp := j.Error(context.Background(), fmt.Errorf("pq: SELECT failed on host db-internal"), InternalErrorCode, 1)
		if resp.Error == nil {
			t.Fatal("expected error response")
		}
		if resp.Error.Data != nil {
			t.Errorf("plain error must not populate data, got %v", resp.Error.Data)
		}
	})

	t.Run("unknown code maps to internal without leaking", func(t *testing.T) {
		resp := j.Error(context.Background(), fmt.Errorf("boom"), -9999, 1)
		if resp.Error == nil {
			t.Fatal("expected error response")
		}
		if resp.Error.Code != InternalErrorCode {
			t.Errorf("expected code %d, got %d", InternalErrorCode, resp.Error.Code)
		}
		if resp.Error.Data != nil {
			t.Errorf("unknown code must not populate data, got %v", resp.Error.Data)
		}
	})

	t.Run("RPCError data is sent to the client", func(t *testing.T) {
		if err := j.RegisterError(-1, "validation_failed"); err != nil {
			t.Fatal(err)
		}
		appErr := NewRPCError(-1, fmt.Errorf("internal detail")).WithData("email is required")
		resp := j.Error(context.Background(), fmt.Errorf("handler: %w", appErr), -1, 1)
		if resp.Error == nil {
			t.Fatal("expected error response")
		}
		if resp.Error.Data != "email is required" {
			t.Errorf("expected data from RPCError, got %v", resp.Error.Data)
		}
	})
}

func TestJSONRPC_Error_RPCErrorCodeIsAuthoritative(t *testing.T) {
	j := New()
	j.SetLogger(nil)
	if err := j.RegisterError(-7, "app_error"); err != nil {
		t.Fatal(err)
	}

	// A raw handler may return a mismatching int code; the RPCError in the
	// chain wins, so code and data can never belong to different errors.
	resp := j.Error(context.Background(),
		NewRPCError(-7, fmt.Errorf("detail")).WithData("d"), InvalidParamsErrorCode, 1)
	if resp.Error.Code != -7 {
		t.Errorf("RPCError code must override the int code, got %d", resp.Error.Code)
	}
	if resp.Error.Message != "app_error" {
		t.Errorf("expected registered message, got %q", resp.Error.Message)
	}
	if resp.Error.Data != "d" {
		t.Errorf("expected data from the same RPCError, got %v", resp.Error.Data)
	}
}

func TestJSONRPC_Error_UnregisteredRPCErrorCode(t *testing.T) {
	j := New()
	var buf bytes.Buffer
	j.SetLogger(slog.New(slog.NewTextHandler(&buf, nil)))

	resp := j.Error(context.Background(),
		NewRPCError(-4242, fmt.Errorf("boom")).WithData(map[string]any{"limit": 5}), -4242, 1)

	if resp.Error.Code != InternalErrorCode {
		t.Errorf("unregistered code must degrade to internal_error, got %d", resp.Error.Code)
	}
	if resp.Error.Data != nil {
		t.Errorf("data must be dropped when the code is remapped, got %v", resp.Error.Data)
	}
	if !strings.Contains(buf.String(), "-4242") {
		t.Errorf("the original unregistered code must be logged, log: %q", buf.String())
	}
}

func TestJSONRPC_Error_TypedNilRPCError(t *testing.T) {
	j := New()
	var buf bytes.Buffer
	j.SetLogger(slog.New(slog.NewTextHandler(&buf, nil)))

	// Classic typed-nil mistake: `var e *RPCError; return e` yields a non-nil
	// error interface holding a nil pointer — must not panic anywhere,
	// including the logging path.
	var nilErr *RPCError
	resp := j.Error(context.Background(), nilErr, InternalErrorCode, 1)
	if resp.Error == nil || resp.Error.Code != InternalErrorCode {
		t.Errorf("expected internal_error response, got %+v", resp.Error)
	}
	if resp.Error != nil && resp.Error.Data != nil {
		t.Errorf("typed-nil RPCError must not attach data, got %v", resp.Error.Data)
	}
}

func TestJSONRPC_Error_ClientErrorsLogAtDebug(t *testing.T) {
	j := New()
	var buf bytes.Buffer
	// Default handler level is Info: Debug records must be suppressed.
	j.SetLogger(slog.New(slog.NewTextHandler(&buf, nil)))

	j.Error(context.Background(), fmt.Errorf("no such method"), MethodNotFoundErrorCode, 1)
	if buf.Len() != 0 {
		t.Errorf("client-caused errors must not log above Debug, log: %q", buf.String())
	}

	j.Error(context.Background(), fmt.Errorf("boom"), InternalErrorCode, 1)
	if !strings.Contains(buf.String(), "boom") {
		t.Errorf("internal errors must still log at Error, log: %q", buf.String())
	}
}

func TestJSONRPC_Error_LogsServerSide(t *testing.T) {
	j := New()
	var buf bytes.Buffer
	j.SetLogger(slog.New(slog.NewTextHandler(&buf, nil)))

	j.Error(context.Background(), fmt.Errorf("pq: connection refused"), InternalErrorCode, 1)

	logged := buf.String()
	if !strings.Contains(logged, "pq: connection refused") {
		t.Errorf("real error must be logged server-side, log: %q", logged)
	}
	if !strings.Contains(logged, "code=-32603") {
		t.Errorf("error code must be logged, log: %q", logged)
	}
}
