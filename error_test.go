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

	t.Run("reserved error code", func(t *testing.T) {
		err := j.RegisterError(InternalErrorCode, "int")
		if err == nil {
			t.Errorf("should NOT register reserved error code %d", InternalErrorCode)
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
