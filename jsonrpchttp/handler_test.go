package jsonrpchttp_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpchttp"
)

func newServer(t *testing.T, opts ...jsonrpchttp.Option) *httptest.Server {
	t.Helper()
	j := jsonrpc.New()
	if err := j.RegisterMethod("echo", func(_ context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		if data == nil {
			return json.RawMessage(`null`), jsonrpc.OK, nil
		}
		return data, jsonrpc.OK, nil
	}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(jsonrpchttp.Handler(j, opts...))
	t.Cleanup(srv.Close)
	return srv
}

func post(t *testing.T, url, contentType, body string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Post(url, contentType, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(b)
}

func TestHandlerSuccess(t *testing.T) {
	srv := newServer(t)
	resp, body := post(t, srv.URL, "application/json",
		`{"jsonrpc":"2.0","id":1,"method":"echo","params":{"a":1}}`)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %q", ct)
	}
	if !strings.Contains(body, `"result":{"a":1}`) {
		t.Fatalf("unexpected body: %s", body)
	}
}

// A notification must produce 204 No Content with an empty body.
func TestHandlerNotificationNoContent(t *testing.T) {
	srv := newServer(t)
	resp, body := post(t, srv.URL, "application/json",
		`{"jsonrpc":"2.0","method":"echo","params":1}`)

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 for notification, got %d: %s", resp.StatusCode, body)
	}
	if body != "" {
		t.Fatalf("notification response body must be empty, got: %s", body)
	}
}

func TestHandlerBatch(t *testing.T) {
	srv := newServer(t)
	resp, body := post(t, srv.URL, "application/json",
		`[{"jsonrpc":"2.0","id":1,"method":"echo"},{"jsonrpc":"2.0","method":"echo"}]`)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var batch []json.RawMessage
	if err := json.Unmarshal([]byte(body), &batch); err != nil || len(batch) != 1 {
		t.Fatalf("expected 1-entry batch (notification filtered), got: %s", body)
	}
}

func TestHandlerMethodNotAllowed(t *testing.T) {
	srv := newServer(t)
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET, got %d", resp.StatusCode)
	}
	if allow := resp.Header.Get("Allow"); allow != "POST" {
		t.Fatalf("expected Allow: POST, got %q", allow)
	}
}

// Every non-POST method is rejected with 405.
func TestHandlerNonPostMethods(t *testing.T) {
	srv := newServer(t)
	for _, m := range []string{http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodOptions} {
		req, err := http.NewRequest(m, srv.URL, strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("%s: expected 405, got %d", m, resp.StatusCode)
		}
	}
}

// An empty body is a JSON-RPC parse error: HTTP 200, -32700 in the body.
func TestHandlerEmptyBody(t *testing.T) {
	srv := newServer(t)
	resp, body := post(t, srv.URL, "application/json", "")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `-32700`) {
		t.Fatalf("expected 200 with -32700 for empty body, got %d: %s", resp.StatusCode, body)
	}
}

// A syntactically broken Content-Type is rejected with 415.
func TestHandlerBrokenContentType(t *testing.T) {
	srv := newServer(t)
	resp, _ := post(t, srv.URL, ";;not-a-mime", `{"jsonrpc":"2.0","id":1,"method":"echo"}`)
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415 for broken Content-Type, got %d", resp.StatusCode)
	}
}

// An all-notification batch yields 204 No Content.
func TestHandlerAllNotificationBatch(t *testing.T) {
	srv := newServer(t)
	resp, body := post(t, srv.URL, "application/json",
		`[{"jsonrpc":"2.0","method":"echo"},{"jsonrpc":"2.0","method":"echo"}]`)
	if resp.StatusCode != http.StatusNoContent || body != "" {
		t.Fatalf("expected empty 204, got %d: %s", resp.StatusCode, body)
	}
}

func TestHandlerUnsupportedMediaType(t *testing.T) {
	srv := newServer(t)
	resp, _ := post(t, srv.URL, "text/plain", `{"jsonrpc":"2.0","id":1,"method":"echo"}`)
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415 for text/plain, got %d", resp.StatusCode)
	}
}

// application/json with a charset parameter must be accepted.
func TestHandlerContentTypeWithCharset(t *testing.T) {
	srv := newServer(t)
	resp, _ := post(t, srv.URL, "application/json; charset=utf-8",
		`{"jsonrpc":"2.0","id":1,"method":"echo"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for application/json;charset=utf-8, got %d", resp.StatusCode)
	}
}

func TestHandlerBodyTooLarge(t *testing.T) {
	srv := newServer(t, jsonrpchttp.WithMaxBodySize(64))
	resp, _ := post(t, srv.URL, "application/json",
		`{"jsonrpc":"2.0","id":1,"method":"echo","params":"`+strings.Repeat("x", 256)+`"}`)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", resp.StatusCode)
	}
}

// Malformed JSON is a JSON-RPC-level error: HTTP 200 with -32700 in the body.
func TestHandlerParseErrorStays200(t *testing.T) {
	srv := newServer(t)
	resp, body := post(t, srv.URL, "application/json", `{broken`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with JSON-RPC error body, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, `-32700`) {
		t.Fatalf("expected parse error in body, got: %s", body)
	}
}

// The request context must reach the handler (client disconnect propagates).
func TestHandlerPropagatesContext(t *testing.T) {
	j := jsonrpc.New()
	got := make(chan bool, 1)
	if err := j.RegisterMethod("check", func(ctx context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		got <- ctx.Done() != nil
		return json.RawMessage(`1`), jsonrpc.OK, nil
	}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(jsonrpchttp.Handler(j))
	t.Cleanup(srv.Close)

	_, _ = post(t, srv.URL, "application/json", `{"jsonrpc":"2.0","id":1,"method":"check"}`)
	if !<-got {
		t.Fatal("handler must receive a cancellable request context")
	}
}
