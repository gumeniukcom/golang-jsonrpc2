package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

func TestJSONRPC_HandleRPCJSONRawMessage(t *testing.T) {
	j := New()
	ctx := context.Background()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty input", "", string(errorInvalidRequest())},
		{"open bracket only", "[", string(errorInvalidRequest())},
		{"mismatched brackets", "[}", string(errorInvalidRequest())},
		{"invalid batch", "[foo}", string(errorInvalidRequest())},
		{"invalid object", "{foo}", string(errorInvalidRequest())},
		{
			"method not found",
			`{"jsonrpc":"2.0", "method":"foo", "params":{}, "id":2}`,
			`{"jsonrpc":"2.0","error":{"code":-32601,"message":"requested_method_not_found","data":""},"id":2}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := j.HandleRPCJSONRawMessage(ctx, []byte(tt.input))
			if string(res) != tt.expected {
				t.Errorf("expected %s, but got %s", tt.expected, string(res))
			}
		})
	}
}

func TestJSONRPC_HandleRPCJSONRawMessage_EmptyBatch(t *testing.T) {
	j := New()
	ctx := context.Background()

	res := j.HandleRPCJSONRawMessage(ctx, []byte("[]"))
	if string(res) != string(errorInvalidRequest()) {
		t.Errorf("expected invalid request for empty batch, but got %s", string(res))
	}
}

func TestJSONRPC_HandleRPC(t *testing.T) {
	j := New()

	type income struct {
		A int `json:"a"`
		B int `json:"bb"`
	}
	type outcome struct {
		C int `json:"c"`
	}
	m := func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		if data == nil {
			return nil, InvalidRequestErrorCode, fmt.Errorf("empty request")
		}
		inc := &income{}
		err := json.Unmarshal(data, inc)
		if err != nil {
			return nil, InvalidRequestErrorCode, err
		}

		C := outcome{
			C: inc.A + inc.B,
		}

		mdata, err := json.Marshal(C)
		if err != nil {
			return nil, InternalErrorCode, err
		}
		return mdata, 0, nil
	}

	err := j.RegisterMethod("sum", m)
	if err != nil {
		t.Fatalf("should register, but got %v", err)
	}

	ctx := context.Background()
	sendData := `{"a":3, "bb":5}`
	resp := j.HandleRPC(ctx, &structs.Request{
		Version: Version,
		Method:  "sum",
		Params:  []byte(sendData),
		ID:      23,
	})
	if resp.Error != nil {
		t.Errorf("expected no error, but got code=%v", resp.Error.Code)
		return
	}
	if string(*resp.Result) != `{"c":8}` {
		t.Errorf("expected %q, but got %q", `{"c":8}`, string(*resp.Result))
	}
}

func TestJSONRPC_HandleBatchRPC(t *testing.T) {
	j := New()

	type income struct {
		A int `json:"a"`
		B int `json:"bb"`
	}
	type outcome struct {
		C int `json:"c"`
	}
	m := func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		if data == nil {
			return nil, InvalidRequestErrorCode, fmt.Errorf("empty request")
		}
		inc := &income{}
		err := json.Unmarshal(data, inc)
		if err != nil {
			return nil, InvalidRequestErrorCode, err
		}

		C := outcome{
			C: inc.A + inc.B,
		}

		mdata, err := json.Marshal(C)
		if err != nil {
			return nil, InternalErrorCode, err
		}
		return mdata, 0, nil
	}

	err := j.RegisterMethod("sum", m)
	if err != nil {
		t.Fatalf("should register, but got %v", err)
	}

	mm := func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		if data == nil {
			return nil, InvalidRequestErrorCode, fmt.Errorf("empty request")
		}
		inc := &income{}
		err := json.Unmarshal(data, inc)
		if err != nil {
			return nil, InvalidRequestErrorCode, err
		}

		C := outcome{
			C: inc.A + inc.B,
		}

		mdata, err := json.Marshal(C)
		if err != nil {
			return nil, InternalErrorCode, err
		}

		time.Sleep(1 * time.Second)
		return mdata, 0, nil
	}

	err = j.RegisterMethod("sum2", mm)
	if err != nil {
		t.Fatalf("should register, but got %v", err)
	}

	ctx := context.Background()

	requests := structs.Requests{
		structs.Request{Version: Version, Method: "sum2", Params: []byte(`{"a":3, "bb":8}`), ID: 24},
		structs.Request{Version: Version, Method: "sum", Params: []byte(`{"a":3, "bb":5}`), ID: 23},
	}

	resp := j.HandleBatchRPC(ctx, requests)
	if len(resp) != len(requests) {
		t.Errorf("expected %v responses, but got %v", len(requests), len(resp))
		return
	}

	for idx := range resp {
		if resp[idx].Error != nil {
			t.Errorf("expected nil error for id=%v, got %v", resp[idx].ID, *resp[idx].Error)
		}
	}
}

func TestJSONRPC_HandleBatchRPCWithTimeOut(t *testing.T) {
	j := New()
	j.SetDefaultTimeOut(2 * time.Second)

	type income struct {
		A int `json:"a"`
		B int `json:"bb"`
	}
	type outcome struct {
		C int `json:"c"`
	}
	m := func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		if data == nil {
			return nil, InvalidRequestErrorCode, fmt.Errorf("empty request")
		}
		inc := &income{}
		err := json.Unmarshal(data, inc)
		if err != nil {
			return nil, InvalidRequestErrorCode, err
		}

		C := outcome{
			C: inc.A + inc.B,
		}

		mdata, err := json.Marshal(C)
		if err != nil {
			return nil, InternalErrorCode, err
		}
		return mdata, 0, nil
	}

	err := j.RegisterMethod("sum", m)
	if err != nil {
		t.Fatalf("should register, but got %v", err)
	}

	mm := func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		if data == nil {
			return nil, InvalidRequestErrorCode, fmt.Errorf("empty request")
		}
		inc := &income{}
		err := json.Unmarshal(data, inc)
		if err != nil {
			return nil, InvalidRequestErrorCode, err
		}

		C := outcome{
			C: inc.A + inc.B,
		}

		mdata, err := json.Marshal(C)
		if err != nil {
			return nil, InternalErrorCode, err
		}

		time.Sleep(3 * time.Second)
		return mdata, 0, nil
	}

	err = j.RegisterMethod("sum2", mm)
	if err != nil {
		t.Fatalf("should register, but got %v", err)
	}

	ctx := context.Background()

	requests := structs.Requests{
		structs.Request{Version: Version, Method: "sum2", Params: []byte(`{"a":3, "bb":8}`), ID: float64(24)},
		structs.Request{Version: Version, Method: "sum", Params: []byte(`{"a":3, "bb":5}`), ID: float64(23)},
	}

	resp := j.HandleBatchRPC(ctx, requests)
	if len(resp) != len(requests) {
		t.Errorf("expected %v responses, but got %v", len(requests), len(resp))
		return
	}

	for idx := range resp {
		if resp[idx].ID == float64(24) && resp[idx].Error == nil {
			t.Errorf("expected timeout error for resp id 24")
		}
	}
}
