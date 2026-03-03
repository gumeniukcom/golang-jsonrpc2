package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestJSONRPC_RegisterMethod(t *testing.T) {
	j := New()

	m := func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		return nil, 0, nil
	}
	mm := func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		return nil, 0, nil
	}

	t.Run("register new method", func(t *testing.T) {
		err := j.RegisterMethod("simple", m)
		if err != nil {
			t.Errorf("should register, but got %v", err)
		}
	})

	t.Run("duplicate method", func(t *testing.T) {
		err := j.RegisterMethod("simple", mm)
		if err == nil {
			t.Errorf("should not register duplicate method")
		}
	})
}

func TestJSONRPC_CallEmpty(t *testing.T) {
	j := New()

	m := func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		return nil, 0, nil
	}
	methodName := "simple"
	callID := 1
	err := j.RegisterMethod(methodName, m)
	if err != nil {
		t.Fatalf("should register, but got %v", err)
	}

	ctx := context.Background()
	resp := j.call(ctx, methodName, nil, callID)

	if resp.ID != callID {
		t.Errorf("should get ID=%v, but got %v", callID, resp.ID)
	}
}

func TestJSONRPC_CallSum(t *testing.T) {
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
	methodName := "sum"
	callID := 1
	err := j.RegisterMethod(methodName, m)
	if err != nil {
		t.Fatalf("should register, but got %v", err)
	}

	ctx := context.Background()
	sendData := `{"a":3, "bb":5}`
	resp := j.call(ctx, methodName, []byte(sendData), callID)

	if resp.ID != callID {
		t.Errorf("should get ID=%v, but got %v", callID, resp.ID)
		return
	}

	if resp.Error != nil {
		t.Errorf("should get no error, but got %v", resp.Error)
		return
	}

	if resp.Result == nil {
		t.Errorf("should get result, but got nil")
		return
	}

	recData := resp.Result
	checkdata := outcome{}
	err = json.Unmarshal(*recData, &checkdata)
	if err != nil {
		t.Errorf("error on unmarshal response: %v", err)
		return
	}
	if checkdata.C != 8 {
		t.Errorf("should be %v, but got %v", 8, checkdata.C)
	}
}
