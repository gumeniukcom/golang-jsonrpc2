package jsonrpc

import (
	"testing"

	"github.com/gumeniukcom/golang-jsonrpc2/v2/internal/testutil"
)

func TestNewRequest_OK(t *testing.T) {
	requestParams := &testutil.RequestTestStruct{ID: 1}

	_, err := NewRequest("foobar", requestParams)
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewRequest_EmptyMethod(t *testing.T) {
	requestParams := &testutil.RequestTestStruct{ID: 1}

	_, err := NewRequest("", requestParams)
	if err == nil {
		t.Fatal("expected error for empty method")
	}
	if err.Error() != "method is required" {
		t.Errorf("expected %q, but got %v", "method is required", err)
	}
}
