package jsonrpcfiber_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpcfiber"
)

func newApp(t *testing.T) *fiber.App {
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
	app := fiber.New()
	app.Post("/rpc", jsonrpcfiber.Handler(j))
	return app
}

func do(t *testing.T, app *fiber.App, contentType, body string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	req := httptest.NewRequest("POST", "/rpc", strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	rec := httptest.NewRecorder()
	rec.Code = resp.StatusCode
	rec.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	return rec, string(b)
}

func TestFiberSuccess(t *testing.T) {
	app := newApp(t)
	rec, body := do(t, app, "application/json",
		`{"jsonrpc":"2.0","id":1,"method":"echo","params":{"a":1}}`)
	if rec.Code != fiber.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, body)
	}
	if !strings.Contains(body, `"result":{"a":1}`) {
		t.Fatalf("unexpected body: %s", body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("expected application/json, got %q", ct)
	}
}

func TestFiberNotificationNoContent(t *testing.T) {
	app := newApp(t)
	rec, body := do(t, app, "application/json", `{"jsonrpc":"2.0","method":"echo","params":1}`)
	if rec.Code != fiber.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, body)
	}
	if body != "" {
		t.Fatalf("notification body must be empty, got: %s", body)
	}
}

func TestFiberBatch(t *testing.T) {
	app := newApp(t)
	rec, body := do(t, app, "application/json",
		`[{"jsonrpc":"2.0","id":1,"method":"echo"},{"jsonrpc":"2.0","method":"echo"}]`)
	if rec.Code != fiber.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, body)
	}
	var batch []json.RawMessage
	if err := json.Unmarshal([]byte(body), &batch); err != nil || len(batch) != 1 {
		t.Fatalf("expected 1-entry batch, got: %s", body)
	}
}

func TestFiberUnsupportedMediaType(t *testing.T) {
	app := newApp(t)
	rec, _ := do(t, app, "text/plain", `{"jsonrpc":"2.0","id":1,"method":"echo"}`)
	if rec.Code != fiber.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", rec.Code)
	}
}

func TestFiberContentTypeWithCharset(t *testing.T) {
	app := newApp(t)
	rec, _ := do(t, app, "application/json; charset=utf-8", `{"jsonrpc":"2.0","id":1,"method":"echo"}`)
	if rec.Code != fiber.StatusOK {
		t.Fatalf("expected 200 for charset, got %d", rec.Code)
	}
}

func TestFiberParseErrorStays200(t *testing.T) {
	app := newApp(t)
	rec, body := do(t, app, "application/json", `{broken`)
	if rec.Code != fiber.StatusOK {
		t.Fatalf("expected 200 with error body, got %d", rec.Code)
	}
	if !strings.Contains(body, `-32700`) {
		t.Fatalf("expected parse error, got: %s", body)
	}
}

// A compressed body is rejected (decompression-bomb guard), not decompressed.
func TestFiberRejectsContentEncoding(t *testing.T) {
	app := newApp(t)
	req := httptest.NewRequest("POST", "/rpc",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"echo"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnsupportedMediaType {
		t.Fatalf("compressed body must be rejected with 415, got %d", resp.StatusCode)
	}
}
