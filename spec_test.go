package jsonrpc

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

func newSpecServer(t *testing.T) (*JSONRPC, *atomic.Int64) {
	t.Helper()
	j := New()
	calls := &atomic.Int64{}
	if err := j.RegisterMethod("echo", func(_ context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		calls.Add(1)
		if data == nil {
			return json.RawMessage(`null`), OK, nil
		}
		return data, OK, nil
	}); err != nil {
		t.Fatal(err)
	}
	return j, calls
}

// A request without an "id" member is a notification: it must be executed
// and MUST NOT produce a response.
func TestNotificationExecutedWithoutResponse(t *testing.T) {
	j, calls := newSpecServer(t)

	resp := j.HandleRPCJSONRawMessage(context.Background(),
		json.RawMessage(`{"jsonrpc":"2.0","method":"echo","params":{"a":1}}`))
	if len(resp) != 0 {
		t.Fatalf("notification must produce no response, got: %s", resp)
	}
	if calls.Load() != 1 {
		t.Fatalf("notification must still execute the handler, calls=%d", calls.Load())
	}
}

// Errors hit by a notification (e.g. method not found) must also be
// suppressed — the server MUST NOT reply to a notification.
func TestNotificationErrorSuppressed(t *testing.T) {
	j, _ := newSpecServer(t)

	resp := j.HandleRPCJSONRawMessage(context.Background(),
		json.RawMessage(`{"jsonrpc":"2.0","method":"no_such_method"}`))
	if len(resp) != 0 {
		t.Fatalf("notification error must be suppressed, got: %s", resp)
	}
}

// "id":null is NOT a notification: the id member is present, so the request
// gets a response whose id is null.
func TestNullIDIsNotNotification(t *testing.T) {
	j, _ := newSpecServer(t)

	resp := j.HandleRPCJSONRawMessage(context.Background(),
		json.RawMessage(`{"jsonrpc":"2.0","id":null,"method":"echo","params":1}`))
	if len(resp) == 0 {
		t.Fatal("id:null request must get a response")
	}
	if !strings.Contains(string(resp), `"id":null`) {
		t.Fatalf("response id must be null, got: %s", resp)
	}
}

// In a batch, notification entries produce no response entries; the
// remaining responses keep request order.
func TestBatchFiltersNotifications(t *testing.T) {
	j, calls := newSpecServer(t)

	resp := j.HandleRPCJSONRawMessage(context.Background(), json.RawMessage(`[
		{"jsonrpc":"2.0","id":1,"method":"echo","params":1},
		{"jsonrpc":"2.0","method":"echo","params":2},
		{"jsonrpc":"2.0","id":3,"method":"echo","params":3}
	]`))

	var batch []structs.Response
	if err := json.Unmarshal(resp, &batch); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, resp)
	}
	if len(batch) != 2 {
		t.Fatalf("expected 2 responses for 3 requests with 1 notification, got %d: %s", len(batch), resp)
	}
	if string(batch[0].ID) != "1" || string(batch[1].ID) != "3" {
		t.Fatalf("responses must keep order (ids 1,3), got: %s", resp)
	}
	if calls.Load() != 3 {
		t.Fatalf("all 3 requests incl. notification must execute, calls=%d", calls.Load())
	}
}

// A batch consisting only of notifications must return nothing at all.
func TestAllNotificationBatchReturnsNothing(t *testing.T) {
	j, calls := newSpecServer(t)

	resp := j.HandleRPCJSONRawMessage(context.Background(), json.RawMessage(`[
		{"jsonrpc":"2.0","method":"echo","params":1},
		{"jsonrpc":"2.0","method":"echo","params":2}
	]`))
	if len(resp) != 0 {
		t.Fatalf("all-notification batch must return nothing, got: %s", resp)
	}
	if calls.Load() != 2 {
		t.Fatalf("notifications must execute, calls=%d", calls.Load())
	}
}

// Malformed JSON must yield ParseError (-32700), not InvalidRequest.
func TestParseErrorForMalformedJSON(t *testing.T) {
	j, _ := newSpecServer(t)

	for _, in := range []string{`{"jsonrpc":"2.0",`, `[{"jsonrpc":"2.0"}`, `garbage`, ``} {
		resp := j.HandleRPCJSONRawMessage(context.Background(), json.RawMessage(in))
		if !strings.Contains(string(resp), `-32700`) {
			t.Fatalf("input %q: expected -32700, got: %s", in, resp)
		}
	}
}

// Structurally valid JSON that is not a request object stays InvalidRequest
// (-32600).
func TestInvalidRequestForNonObjectJSON(t *testing.T) {
	j, _ := newSpecServer(t)

	for _, in := range []string{`1`, `"str"`, `true`, `{"jsonrpc":"1.0","id":1,"method":"echo"}`} {
		resp := j.HandleRPCJSONRawMessage(context.Background(), json.RawMessage(in))
		if !strings.Contains(string(resp), `-32600`) {
			t.Fatalf("input %q: expected -32600, got: %s", in, resp)
		}
	}
}

// Leading/trailing whitespace around a valid message must be accepted.
func TestWhitespaceAroundMessageAccepted(t *testing.T) {
	j, _ := newSpecServer(t)

	resp := j.HandleRPCJSONRawMessage(context.Background(),
		json.RawMessage(" \n\t {\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"echo\",\"params\":5} \r\n "))
	if !strings.Contains(string(resp), `"result"`) {
		t.Fatalf("whitespace-wrapped request must succeed, got: %s", resp)
	}
}

// A large integer id must be echoed back byte-exact (no float64 round-trip).
func TestLargeIntegerIDPrecision(t *testing.T) {
	j, _ := newSpecServer(t)

	resp := j.HandleRPCJSONRawMessage(context.Background(),
		json.RawMessage(`{"jsonrpc":"2.0","id":9007199254740993,"method":"echo","params":1}`))
	if !strings.Contains(string(resp), `"id":9007199254740993`) {
		t.Fatalf("id must be echoed exactly, got: %s", resp)
	}
}

// id may only be a string, number, or null.
func TestNonScalarIDRejected(t *testing.T) {
	j, _ := newSpecServer(t)

	for _, in := range []string{
		`{"jsonrpc":"2.0","id":{"a":1},"method":"echo"}`,
		`{"jsonrpc":"2.0","id":[1],"method":"echo"}`,
		`{"jsonrpc":"2.0","id":true,"method":"echo"}`,
	} {
		resp := j.HandleRPCJSONRawMessage(context.Background(), json.RawMessage(in))
		if !strings.Contains(string(resp), `-32600`) {
			t.Fatalf("input %q: expected -32600 for non-scalar id, got: %s", in, resp)
		}
	}
}

// A batch with some undecodable entries must still answer the valid ones;
// broken entries get individual -32600 errors with id:null (spec §6 example).
func TestBatchInvalidEntriesGetIndividualErrors(t *testing.T) {
	j, _ := newSpecServer(t)

	resp := j.HandleRPCJSONRawMessage(context.Background(), json.RawMessage(`[
		{"jsonrpc":"2.0","id":1,"method":"echo","params":1},
		2,
		{"jsonrpc":"2.0","id":3,"method":"echo","params":3}
	]`))
	if !json.Valid(resp) {
		t.Fatalf("batch response must be valid JSON, got: %s", resp)
	}
	var batch []structs.Response
	if err := json.Unmarshal(resp, &batch); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, resp)
	}
	if len(batch) != 3 {
		t.Fatalf("expected 3 entries, got %d: %s", len(batch), resp)
	}
	if batch[0].Error != nil || string(batch[0].ID) != "1" {
		t.Fatalf("entry 0 must succeed with id 1, got: %s", resp)
	}
	// Decoding a Response with encoding/json leaves a null id as nil.
	if batch[1].Error == nil || batch[1].Error.Code != InvalidRequestErrorCode ||
		(len(batch[1].ID) > 0 && string(batch[1].ID) != "null") {
		t.Fatalf("entry 1 must be -32600 with id null, got: %s", resp)
	}
	if batch[2].Error != nil || string(batch[2].ID) != "3" {
		t.Fatalf("entry 2 must succeed with id 3, got: %s", resp)
	}
}

// Spec §6: [1,2,3] → three individual -32600 error entries.
func TestBatchOfPrimitivesPerSpec(t *testing.T) {
	j, _ := newSpecServer(t)

	resp := j.HandleRPCJSONRawMessage(context.Background(), json.RawMessage(`[1,2,3]`))
	var batch []structs.Response
	if err := json.Unmarshal(resp, &batch); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, resp)
	}
	if len(batch) != 3 {
		t.Fatalf("expected 3 error entries, got %d: %s", len(batch), resp)
	}
	for i, r := range batch {
		if r.Error == nil || r.Error.Code != InvalidRequestErrorCode {
			t.Fatalf("entry %d must be -32600, got: %s", i, resp)
		}
	}
}

// The lazy lexer lets syntactically broken scalar ids (1e, -, 1.) through;
// they must be rejected, not echoed byte-exact into (and corrupting) the
// response.
func TestBrokenScalarIDNotEchoed(t *testing.T) {
	j, _ := newSpecServer(t)

	for _, in := range []string{
		`{"jsonrpc":"2.0","id":1e,"method":"echo"}`,
		`{"jsonrpc":"2.0","id":-,"method":"echo"}`,
		`{"jsonrpc":"2.0","id":1.,"method":"echo"}`,
	} {
		resp := j.HandleRPCJSONRawMessage(context.Background(), json.RawMessage(in))
		if !json.Valid(resp) {
			t.Fatalf("input %q: response must be valid JSON, got: %s", in, resp)
		}
		if !strings.Contains(string(resp), `"id":null`) {
			t.Fatalf("input %q: broken id must not be echoed, got: %s", in, resp)
		}
	}
}

// A broken id inside a batch must not corrupt the sibling responses.
func TestBrokenIDInBatchKeepsSiblings(t *testing.T) {
	j, _ := newSpecServer(t)

	resp := j.HandleRPCJSONRawMessage(context.Background(), json.RawMessage(
		`[{"jsonrpc":"2.0","id":1,"method":"echo"},{"jsonrpc":"2.0","id":1e,"method":"echo"},{"jsonrpc":"2.0","id":2,"method":"echo"}]`))
	if !json.Valid(resp) {
		t.Fatalf("batch response must be valid JSON, got: %s", resp)
	}
	var batch []structs.Response
	if err := json.Unmarshal(resp, &batch); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(batch) != 3 || batch[0].Error != nil || batch[2].Error != nil {
		t.Fatalf("valid siblings must succeed, got: %s", resp)
	}
}

// NaN/Inf convenience ids must not produce invalid JSON.
func TestNaNIDMarshalsAsNull(t *testing.T) {
	resp := NewResponse(math.NaN(), nil, &structs.Error{Code: -32603, Message: "x"})
	raw, err := resp.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		t.Fatalf("NaN id must not corrupt JSON, got: %s", raw)
	}
}

// Decoded Params and ID must not alias the caller's input buffer: reusing
// the buffer after UnmarshalJSON must not corrupt the request.
func TestRequestDecodeCopiesBuffer(t *testing.T) {
	buf := []byte(`{"jsonrpc":"2.0","id":42,"method":"echo","params":{"a":1}}`)
	var req structs.Request
	if err := req.UnmarshalJSON(buf); err != nil {
		t.Fatal(err)
	}
	for i := range buf {
		buf[i] = 'X'
	}
	if string(req.ID) != "42" {
		t.Fatalf("ID must survive buffer reuse, got: %q", req.ID)
	}
	if string(req.Params) != `{"a":1}` {
		t.Fatalf("Params must survive buffer reuse, got: %q", req.Params)
	}
}

// Method names with the reserved "rpc." prefix must be rejected at
// registration time — with the single sanctioned exception of
// "rpc.discover", the OpenRPC discovery extension (see methods.go).
func TestRPCPrefixRegistrationRejected(t *testing.T) {
	j := New()
	err := j.RegisterMethod("rpc.internal", func(_ context.Context, _ json.RawMessage) (json.RawMessage, int, error) {
		return nil, OK, nil
	})
	if err == nil {
		t.Fatal("rpc.* method registration must be rejected")
	}
}
