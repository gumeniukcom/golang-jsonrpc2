package jsonrpcfiberv3_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpcfiberv3"
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
	app.Post("/rpc", jsonrpcfiberv3.Handler(j))
	return app
}

func do(t *testing.T, app *fiber.App, contentType, body string) (int, string, string) {
	t.Helper()
	req := httptest.NewRequest("POST", "/rpc", strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := app.Test(req, fiber.TestConfig{Timeout: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header.Get("Content-Type"), string(b)
}

func TestFiberV3Success(t *testing.T) {
	app := newApp(t)
	code, ct, body := do(t, app, "application/json",
		`{"jsonrpc":"2.0","id":1,"method":"echo","params":{"a":1}}`)
	if code != fiber.StatusOK {
		t.Fatalf("expected 200, got %d: %s", code, body)
	}
	if !strings.Contains(body, `"result":{"a":1}`) {
		t.Fatalf("unexpected body: %s", body)
	}
	if !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("expected application/json, got %q", ct)
	}
}

func TestFiberV3NotificationNoContent(t *testing.T) {
	app := newApp(t)
	code, _, body := do(t, app, "application/json", `{"jsonrpc":"2.0","method":"echo","params":1}`)
	if code != fiber.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", code, body)
	}
	if body != "" {
		t.Fatalf("notification body must be empty, got: %s", body)
	}
}

func TestFiberV3Batch(t *testing.T) {
	app := newApp(t)
	code, _, body := do(t, app, "application/json",
		`[{"jsonrpc":"2.0","id":1,"method":"echo"},{"jsonrpc":"2.0","method":"echo"}]`)
	if code != fiber.StatusOK {
		t.Fatalf("expected 200, got %d: %s", code, body)
	}
	var batch []json.RawMessage
	if err := json.Unmarshal([]byte(body), &batch); err != nil || len(batch) != 1 {
		t.Fatalf("expected 1-entry batch, got: %s", body)
	}
}

func TestFiberV3UnsupportedMediaType(t *testing.T) {
	app := newApp(t)
	code, _, _ := do(t, app, "text/plain", `{"jsonrpc":"2.0","id":1,"method":"echo"}`)
	if code != fiber.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", code)
	}
}

func TestFiberV3ContentTypeWithCharset(t *testing.T) {
	app := newApp(t)
	code, _, _ := do(t, app, "application/json; charset=utf-8", `{"jsonrpc":"2.0","id":1,"method":"echo"}`)
	if code != fiber.StatusOK {
		t.Fatalf("expected 200 for charset, got %d", code)
	}
}

func TestFiberV3ParseErrorStays200(t *testing.T) {
	app := newApp(t)
	code, _, body := do(t, app, "application/json", `{broken`)
	if code != fiber.StatusOK {
		t.Fatalf("expected 200 with error body, got %d", code)
	}
	if !strings.Contains(body, `-32700`) {
		t.Fatalf("expected parse error, got: %s", body)
	}
}

// A compressed body is rejected (decompression-bomb guard), not decompressed.
func TestFiberV3RejectsContentEncoding(t *testing.T) {
	app := newApp(t)
	req := httptest.NewRequest("POST", "/rpc",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"echo"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	resp, err := app.Test(req, fiber.TestConfig{Timeout: -1})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnsupportedMediaType {
		t.Fatalf("compressed body must be rejected with 415, got %d", resp.StatusCode)
	}
}

// Under enforced-timeout mode a slow handler outlives the request; the
// adapter must not pass the pooled Fiber Ctx into the dispatcher (it would
// race with Ctx reuse). Run many concurrent requests under -race; passing a
// stable context (c.Context()) keeps it clean.
func TestFiberV3EnforcedTimeoutNoCtxRace(t *testing.T) {
	j := jsonrpc.New()
	j.SetEnforcedTimeout(true)
	j.SetDefaultTimeOut(20 * time.Millisecond)
	if err := j.RegisterMethod("slow", func(ctx context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		time.Sleep(80 * time.Millisecond) // ignores ctx, outlives the deadline
		_ = ctx.Value("x")                // touches the ctx after the handler returned
		return json.RawMessage(`"late"`), jsonrpc.OK, nil
	}); err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Post("/rpc", jsonrpcfiberv3.Handler(j))

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("POST", "/rpc",
				strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"slow"}`))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req, fiber.TestConfig{Timeout: -1})
			if err == nil {
				_ = resp.Body.Close()
			}
		}()
	}
	wg.Wait()
	time.Sleep(150 * time.Millisecond) // let lingering handlers finish and touch ctx
}
