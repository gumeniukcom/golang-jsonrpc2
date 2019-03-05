package jsonrpc

import (
	"context"
	"fmt"
	"testing"
)

func TestJSONRPC_RegisterError(t *testing.T) {
	j := New()

	err := j.RegisterError(-1, "minus 1")
	if err != nil {
		t.Errorf("Should register error with code -1, but got \"%v\"", err)
		return
	}

	err = j.RegisterError(-1, "minus 1")
	if err == nil {
		t.Errorf("Should NOT register error with code -1")
		return
	}

	err = j.RegisterError(InternalErrorCode, "int")
	if err == nil {
		t.Errorf("Should NOT register error code \"%d\"", InternalErrorCode)
		return
	}
}

func TestJSONRPC_NewError(t *testing.T) {
	j := New()

	resp := j.NewError(context.Background(), fmt.Errorf("foobar"), InternalErrorCode, 1)

	if resp.Error == nil {
		t.Errorf("Should get with error code \"%d\"", InternalErrorCode)
		return
	}

	if resp.Error.Code != InternalErrorCode {
		t.Errorf("Should get with error code \"%d\", but got: %d", InternalErrorCode, resp.Error.Code)
		return
	}
}
