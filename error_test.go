package jsonrpc

import (
	"context"
	"fmt"
	"testing"
)

func TestJSONRPC_RegisterError(t *testing.T) {
	j := New()

	t.Run("register custom error", func(t *testing.T) {
		err := j.RegisterError(-1, "minus 1")
		if err != nil {
			t.Errorf("should register error with code -1, but got %q", err)
		}
	})

	t.Run("duplicate error code", func(t *testing.T) {
		err := j.RegisterError(-1, "minus 1")
		if err == nil {
			t.Errorf("should NOT register duplicate error with code -1")
		}
	})

	t.Run("dispatcher pre-defined code", func(t *testing.T) {
		err := j.RegisterError(InternalErrorCode, "int")
		if err == nil {
			t.Errorf("should NOT register dispatcher pre-defined error code %d", InternalErrorCode)
		}
	})

	t.Run("implementation-defined server error range", func(t *testing.T) {
		tests := []struct {
			name string
			code int
		}{
			{"server error -32001", -32001},
			{"server error range edge -32099", -32099},
			{"LSP RequestCancelled -32800", -32800},
			{"LSP RequestFailed -32803", -32803},
			{"deep reserved -32150", -32150},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				if err := j.RegisterError(tc.code, "custom"); err != nil {
					t.Errorf("should register error code %d in reserved range, but got %q", tc.code, err)
				}
			})
		}
	})

	t.Run("reserved-range code round-trips to the client", func(t *testing.T) {
		resp := j.Error(context.Background(), fmt.Errorf("detail"), -32001, 1)
		if resp.Error == nil || resp.Error.Code != -32001 {
			t.Fatalf("should get error code -32001, got %+v", resp.Error)
		}
		if resp.Error.Message != "custom" {
			t.Errorf("should get registered message, got %q", resp.Error.Message)
		}
	})
}

func TestJSONRPC_NewError(t *testing.T) {
	j := New()

	resp := j.Error(context.Background(), fmt.Errorf("foobar"), InternalErrorCode, 1)

	if resp.Error == nil {
		t.Errorf("should get response with error code %d", InternalErrorCode)
		return
	}

	if resp.Error.Code != InternalErrorCode {
		t.Errorf("should get error code %d, but got: %d", InternalErrorCode, resp.Error.Code)
	}
}
