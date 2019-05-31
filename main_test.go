package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAll(t *testing.T) {

	type income struct {
		A int `json:"a"`
		B int `json:"bb"`
	}
	type outcome struct {
		C int `json:"c"`
	}
	sumMethod := func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
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

	sumMethodName := "sum"

	j := New()

	err := j.RegisterMethod(sumMethodName, sumMethod)
	if err != nil {
		t.Error(err)
		return
	}

	serv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}
		defer r.Body.Close()

		w.Header().Set("Content-Type", "applicaition/json")
		w.WriteHeader(http.StatusOK)
		w.Write(j.HandleRPCJsonRawMessage(ctx, body))
	}))
	defer serv.Close()

	client := serv.Client()

	type teststruct struct {
		Request  string
		Response string
		Idx      int
	}

	tests := []teststruct{
		{
			Request:  "",
			Response: "{\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32600,\"message\":\"invalid_request_not_conforming_to_spec\"}, \"id\":1}",
			Idx:      1,
		},
		{
			Request:  `{"jsonrpc":"2.0", "id"":1}`,
			Response: "{\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32600,\"message\":\"invalid_request_not_conforming_to_spec\"}, \"id\":1}",
			Idx:      2,
		},
	}

	for idx := range tests {
		res, err := client.Post(serv.URL, "application/json", strings.NewReader(tests[idx].Request))
		if err != nil {
			t.Error(err)
			return
		}

		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			panic(err)
		}

		resp := string(body)
		if resp != tests[idx].Response {
			t.Errorf("[%d] Expected '%s', got '%s'", tests[idx].Idx, tests[idx].Response, resp)
			return
		}
	}

}
