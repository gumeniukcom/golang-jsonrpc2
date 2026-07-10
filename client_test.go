package jsonrpc

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

type fakeCaller struct {
	result json.RawMessage
	err    error
	method string
	params json.RawMessage
}

func (f *fakeCaller) Call(_ context.Context, method string, params any) (json.RawMessage, error) {
	f.method = method
	f.params, _ = MarshalParams(params)
	return f.result, f.err
}

func (f *fakeCaller) Notify(_ context.Context, method string, params any) error {
	f.method = method
	return f.err
}

func TestCallResultDecodesTypedResult(t *testing.T) {
	fc := &fakeCaller{result: json.RawMessage(`{"sum":8}`)}
	type out struct {
		Sum int `json:"sum"`
	}
	res, err := CallResult[out](context.Background(), fc, "sum", map[string]int{"a": 3, "b": 5})
	if err != nil {
		t.Fatal(err)
	}
	if res.Sum != 8 {
		t.Fatalf("expected 8, got %+v", res)
	}
	if fc.method != "sum" || string(fc.params) != `{"a":3,"b":5}` {
		t.Fatalf("unexpected call: %s %s", fc.method, fc.params)
	}
}

func TestCallResultPropagatesRPCError(t *testing.T) {
	fc := &fakeCaller{err: &structs.Error{Code: -32601, Message: "requested_method_not_found"}}
	_, err := CallResult[int](context.Background(), fc, "nope", nil)
	var rpcErr *structs.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != -32601 {
		t.Fatalf("expected *structs.Error(-32601), got: %v", err)
	}
	if rpcErr.Error() == "" {
		t.Fatal("structs.Error must implement error with a useful message")
	}
}

func TestMarshalParams(t *testing.T) {
	if p, err := MarshalParams(nil); err != nil || p != nil {
		t.Fatalf("nil params must stay nil, got %s, %v", p, err)
	}
	raw := json.RawMessage(`{"x":1}`)
	if p, _ := MarshalParams(raw); string(p) != `{"x":1}` {
		t.Fatalf("RawMessage must pass through, got %s", p)
	}
	if p, _ := MarshalParams(struct {
		A int `json:"a"`
	}{7}); string(p) != `{"a":7}` {
		t.Fatalf("struct params must marshal, got %s", p)
	}
	if _, err := MarshalParams(make(chan int)); err == nil {
		t.Fatal("unmarshalable params must error")
	}
}
