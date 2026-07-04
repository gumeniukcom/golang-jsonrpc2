package jsonrpc

import (
	"context"
	"fmt"
	"testing"

	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

type sumParams struct {
	A int `json:"a"`
	B int `json:"b"`
}

type sumResult struct {
	C int `json:"c"`
}

func TestTyped_Success(t *testing.T) {
	j := New()
	j.SetLogger(nil)

	err := RegisterTyped(j, "sum", func(ctx context.Context, p sumParams) (sumResult, error) {
		return sumResult{C: p.A + p.B}, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	resp := j.HandleRPC(context.Background(), &structs.Request{
		Version: Version,
		Method:  "sum",
		Params:  []byte(`{"a":3,"b":5}`),
		ID:      1,
	})
	if resp.Error != nil {
		t.Fatalf("expected no error, got %+v", resp.Error)
	}
	if string(*resp.Result) != `{"c":8}` {
		t.Errorf("expected {\"c\":8}, got %s", string(*resp.Result))
	}
}

func TestTyped_NilParams(t *testing.T) {
	j := New()
	j.SetLogger(nil)

	err := RegisterTyped(j, "zero", func(ctx context.Context, p sumParams) (sumResult, error) {
		return sumResult{C: p.A + p.B}, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	resp := j.HandleRPC(context.Background(), &structs.Request{
		Version: Version,
		Method:  "zero",
		ID:      1,
	})
	if resp.Error != nil {
		t.Fatalf("absent params should yield zero value, got error %+v", resp.Error)
	}
	if string(*resp.Result) != `{"c":0}` {
		t.Errorf("expected {\"c\":0}, got %s", string(*resp.Result))
	}
}

func TestTyped_InvalidParams(t *testing.T) {
	j := New()
	j.SetLogger(nil)

	err := RegisterTyped(j, "sum", func(ctx context.Context, p sumParams) (sumResult, error) {
		return sumResult{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	resp := j.HandleRPC(context.Background(), &structs.Request{
		Version: Version,
		Method:  "sum",
		Params:  []byte(`{"a":"not-a-number"}`),
		ID:      1,
	})
	if resp.Error == nil {
		t.Fatal("expected error response")
	}
	if resp.Error.Code != InvalidParamsErrorCode {
		t.Errorf("expected code %d, got %d", InvalidParamsErrorCode, resp.Error.Code)
	}
}

func TestTyped_RPCErrorCodeAndData(t *testing.T) {
	j := New()
	j.SetLogger(nil)

	const cropLimitCode = 4001
	if err := j.RegisterError(cropLimitCode, "custom_crop_limit_exceeded"); err != nil {
		t.Fatal(err)
	}

	err := RegisterTyped(j, "crop.add", func(ctx context.Context, p sumParams) (sumResult, error) {
		return sumResult{}, NewRPCError(cropLimitCode, fmt.Errorf("user has 5 crops")).
			WithData(map[string]any{"limit": 5})
	})
	if err != nil {
		t.Fatal(err)
	}

	resp := j.HandleRPC(context.Background(), &structs.Request{
		Version: Version,
		Method:  "crop.add",
		Params:  []byte(`{"a":1,"b":2}`),
		ID:      1,
	})
	if resp.Error == nil {
		t.Fatal("expected error response")
	}
	if resp.Error.Code != cropLimitCode {
		t.Errorf("expected code %d from RPCError, got %d", cropLimitCode, resp.Error.Code)
	}
	if resp.Error.Message != "custom_crop_limit_exceeded" {
		t.Errorf("expected registered message, got %q", resp.Error.Message)
	}
	data, ok := resp.Error.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected map data, got %T", resp.Error.Data)
	}
	if data["limit"] != 5 {
		t.Errorf("expected limit 5 in data, got %v", data["limit"])
	}
}

func TestTyped_PlainErrorMapsToInternal(t *testing.T) {
	j := New()
	j.SetLogger(nil)

	err := RegisterTyped(j, "boom", func(ctx context.Context, p sumParams) (sumResult, error) {
		return sumResult{}, fmt.Errorf("pq: something internal")
	})
	if err != nil {
		t.Fatal(err)
	}

	resp := j.HandleRPC(context.Background(), &structs.Request{
		Version: Version,
		Method:  "boom",
		Params:  []byte(`{}`),
		ID:      1,
	})
	if resp.Error == nil {
		t.Fatal("expected error response")
	}
	if resp.Error.Code != InternalErrorCode {
		t.Errorf("expected code %d, got %d", InternalErrorCode, resp.Error.Code)
	}
	if resp.Error.Data != nil {
		t.Errorf("plain error must not leak into data, got %v", resp.Error.Data)
	}
}

func TestRegisterTyped_Duplicate(t *testing.T) {
	j := New()

	fn := func(ctx context.Context, p sumParams) (sumResult, error) { return sumResult{}, nil }
	if err := RegisterTyped(j, "dup", fn); err != nil {
		t.Fatal(err)
	}
	if err := RegisterTyped(j, "dup", fn); err == nil {
		t.Error("duplicate registration must fail")
	}
}
