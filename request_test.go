package jsonrpc

import (
	"context"
	"github.com/gumeniukcom/golang-jsonrpc2/mockstruct"
	"testing"
)

func TestRequest_OK(t *testing.T) {
	requestParams := &mockstruct.RequestTestStruct{ID: 1}

	_, err := Request(context.Background(), "foobar", requestParams)
	if err != nil {
		t.Error(err)
		return
	}
}

func TestRequest_EmptyMethod(t *testing.T) {
	requestParams := &mockstruct.RequestTestStruct{ID: 1}

	_, err := Request(context.Background(), "", requestParams)
	if err.Error() != "method is required" {
		t.Errorf("Expected %s, but got %v", "method is required", err)
		return
	}
}
