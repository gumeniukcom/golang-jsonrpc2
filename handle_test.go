package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gumeniukcom/golang-jsonrpc2/structs"
	"testing"
	"time"
)

func TestJSONRPC_HandleRPCJsonRawMessage(t *testing.T) {
	j := New()
	ctx := context.Background()
	i := 0

	res := j.HandleRPCJsonRawMessage(ctx, []byte(""))
	if string(res) != string(errorInvalidRequest()) {
		t.Errorf("[%d] Expected invalid request, but got \"%s\"", i, string(res))
	}
	i++

	res = j.HandleRPCJsonRawMessage(ctx, []byte("["))
	if string(res) != string(errorInvalidRequest()) {
		t.Errorf("[%d]Expected invalid request, but got \"%s\"", i, string(res))
	}
	i++

	res = j.HandleRPCJsonRawMessage(ctx, []byte("[}"))
	if string(res) != string(errorInvalidRequest()) {
		t.Errorf("[%d]Expected invalid request, but got \"%s\"", i, string(res))
	}
	i++

	res = j.HandleRPCJsonRawMessage(ctx, []byte("[foo}"))
	if string(res) != string(errorInvalidRequest()) {
		t.Errorf("[%d]Expected invalid request, but got \"%s\"", i, string(res))
	}
	i++

	res = j.HandleRPCJsonRawMessage(ctx, []byte("{foo}"))
	if string(res) != string(errorInvalidRequest()) {
		t.Errorf("[%d]Expected invalid request, but got \"%s\"", i, string(res))
	}
	i++

	res = j.HandleRPCJsonRawMessage(ctx, []byte("{\"jsonrpc\":\"2.0\", \"method\":\"foo\", \"params\":{}, \"id\":2}"))
	if len(res) == 0 {
		t.Errorf("[%d] Empty response", i)
	}
	if string(res) != "{\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32601,\"message\":\"requested_method_not_found\",\"data\":\"\"},\"id\":2}" {
		t.Errorf("[%d]Expected method not found, but got \"%s\"", i, string(res))
	}
	//i++
	//t.Logf("%#v", string(res))
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
	methodName := "sum"
	//callID := 1
	err := j.RegisterMethod(methodName, m)

	if err != nil {
		t.Errorf("should register , but got %v", err)
		return
	}

	ctx := context.Background()

	sendData := "{\"a\":3, \"bb\":5}"
	//resp := j.call(ctx, methodName, []byte(sendData), callID)
	resp := j.HandleRPC(ctx, &structs.Request{
		Version: JSONRPCVersion,
		Method:  "sum",
		Params:  []byte(sendData),
		ID:      23,
	})
	if resp.Error != nil {
		t.Errorf("Expected empty error, but got code=%v", resp.Error.Code)
		return
	}
	if string(*resp.Result) != "{\"c\":8}" {
		t.Errorf("Expected %#v, but got %#v", "{\"c\":8}", string(*resp.Result))
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
	methodName := "sum"
	//callID := 1
	err := j.RegisterMethod(methodName, m)
	if err != nil {
		t.Error(err)
		return
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
	methodName2 := "sum2"

	err = j.RegisterMethod(methodName2, mm)
	if err != nil {
		t.Error(err)
		return
	}

	if err != nil {
		t.Errorf("should register , but got %v", err)
		return
	}

	ctx := context.Background()

	sendData := "{\"a\":3, \"bb\":5}"
	sendData2 := "{\"a\":3, \"bb\":8}"
	//resp := j.call(ctx, methodName, []byte(sendData), callID)

	requests := structs.Requests{}

	requests = append(requests, structs.Request{
		Version: JSONRPCVersion,
		Method:  "sum2",
		Params:  []byte(sendData2),
		ID:      24,
	})
	requests = append(requests, structs.Request{
		Version: JSONRPCVersion,
		Method:  "sum",
		Params:  []byte(sendData),
		ID:      23,
	})
	resp := j.HandleBatchRPC(ctx, requests)
	if len(resp) == 0 {
		t.Errorf("Expected %v responses, but got %v", len(requests), len(resp))
		return
	}
	t.Logf("%v", resp)

	for idx := range resp {
		//if resp[idx].ID == 24&& resp.re
		t.Logf("%#v", resp[idx].ID)
		if resp[idx].Result != nil {

			t.Logf("%#v", string(*resp[idx].Result))
		}
		if resp[idx].Error != nil {
			t.Errorf("expected nil error, got '%v'", *resp[idx].Error)
		}
	}

}

func TestJSONRPC_HandleBatchRPCWithTimeOut(t *testing.T) {
	j := New()

	j.SetDefaultTimeOut(2)
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
	//callID := 1
	err := j.RegisterMethod(methodName, m)
	if err != nil {
		t.Error(err)
		return
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
	methodName2 := "sum2"

	err = j.RegisterMethod(methodName2, mm)
	if err != nil {
		t.Error(err)
		return
	}

	if err != nil {
		t.Errorf("should register , but got %v", err)
		return
	}

	ctx := context.Background()

	sendData := "{\"a\":3, \"bb\":5}"
	sendData2 := "{\"a\":3, \"bb\":8}"
	//resp := j.call(ctx, methodName, []byte(sendData), callID)

	requests := structs.Requests{}

	requests = append(requests, structs.Request{
		Version: JSONRPCVersion,
		Method:  "sum2",
		Params:  []byte(sendData2),
		ID:      24,
	})
	requests = append(requests, structs.Request{
		Version: JSONRPCVersion,
		Method:  "sum",
		Params:  []byte(sendData),
		ID:      23,
	})
	resp := j.HandleBatchRPC(ctx, requests)
	if len(resp) == 0 {
		t.Errorf("Expected %v responses, but got %v", len(requests), len(resp))
		return
	}
	t.Logf("%v", resp)

	for idx := range resp {
		//if resp[idx].ID == 24&& resp.re
		t.Logf("%#v", resp[idx].ID)
		if resp[idx].Result != nil {

			t.Logf("%#v", string(*resp[idx].Result))
		}
		if resp[idx].Error != nil {

			t.Logf("%#v", *resp[idx].Error)
		}
		if resp[idx].ID == 24 && resp[idx].Error == nil {
			t.Errorf("expected timeout error for resp id %d", 24)
		}
	}
}
