package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

	j := New()

	err := j.RegisterMethod("sum", sumMethod)
	if err != nil {
		t.Fatal(err)
	}

	serv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}
		defer func() { _ = r.Body.Close() }()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(j.HandleRPCJSONRawMessage(r.Context(), body))
	}))
	defer serv.Close()

	client := serv.Client()

	tests := []struct {
		name     string
		request  string
		response string
	}{
		{
			name:     "empty body",
			request:  "",
			response: `{"jsonrpc":"2.0","error":{"code":-32600,"message":"invalid_request_not_conforming_to_spec"},"id":null}`,
		},
		{
			name:     "malformed JSON",
			request:  `{"jsonrpc":"2.0", "id"":1}`,
			response: `{"jsonrpc":"2.0","error":{"code":-32600,"message":"invalid_request_not_conforming_to_spec"},"id":null}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := client.Post(serv.URL, "application/json", strings.NewReader(tt.request))
			if err != nil {
				t.Fatal(err)
			}

			body, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatal(err)
			}

			resp := string(body)
			if resp != tt.response {
				t.Errorf("expected %q, got %q", tt.response, resp)
			}
		})
	}
}

func TestCallPanic(t *testing.T) {
	sumMethod := func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		panic("panic")
	}

	j := New()

	err := j.RegisterMethod("sum", sumMethod)
	if err != nil {
		t.Fatal(err)
	}

	serv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}
		defer func() { _ = r.Body.Close() }()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(j.HandleRPCJSONRawMessage(r.Context(), body))
	}))
	defer serv.Close()

	client := serv.Client()

	tests := []struct {
		name     string
		request  string
		response string
	}{
		{
			name:     "panic recovery",
			request:  `{"jsonrpc":"2.0", "id":1, "method":"sum", "params":{}}`,
			response: `{"jsonrpc":"2.0","error":{"code":-32603,"message":"internal_error","data":"panic"},"id":1}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := client.Post(serv.URL, "application/json", strings.NewReader(tt.request))
			if err != nil {
				t.Fatal(err)
			}

			body, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatal(err)
			}

			resp := string(body)
			if resp != tt.response {
				t.Errorf("expected %q, got %q", tt.response, resp)
			}
		})
	}
}
