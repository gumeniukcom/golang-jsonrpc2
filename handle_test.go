package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
		{"empty input", "", string(errorParse())},
		{"open bracket only", "[", string(errorParse())},
		{"mismatched brackets", "[}", string(errorParse())},
		{"invalid batch", "[foo}", string(errorParse())},
		{"invalid object", "{foo}", string(errorParse())},
		{
			"method not found",
			`{"jsonrpc":"2.0", "method":"foo", "params":{}, "id":2}`,
			`{"jsonrpc":"2.0","error":{"code":-32601,"message":"requested_method_not_found"},"id":2}`,
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
		ID:      structs.ID("23"),
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
		structs.Request{Version: Version, Method: "sum2", Params: []byte(`{"a":3, "bb":8}`), ID: structs.ID("24")},
		structs.Request{Version: Version, Method: "sum", Params: []byte(`{"a":3, "bb":5}`), ID: structs.ID("23")},
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
		structs.Request{Version: Version, Method: "sum2", Params: []byte(`{"a":3, "bb":8}`), ID: structs.ID("24")},
		structs.Request{Version: Version, Method: "sum", Params: []byte(`{"a":3, "bb":5}`), ID: structs.ID("23")},
	}

	resp := j.HandleBatchRPC(ctx, requests)
	if len(resp) != len(requests) {
		t.Errorf("expected %v responses, but got %v", len(requests), len(resp))
		return
	}

	for idx := range resp {
		if string(resp[idx].ID) == "24" && resp[idx].Error == nil {
			t.Errorf("expected timeout error for resp id 24")
		}
	}
}

// Spec §5/§7: an INVALID request draws an id:null -32600 error even when it
// carries no id — only a syntactically valid request without an id is a
// notification and earns silence.
func TestJSONRPC_InvalidIDLessRequestIsAnswered(t *testing.T) {
	j := New()
	ctx := context.Background()

	tests := []struct {
		name  string
		input string
	}{
		{"wrong version, no id", `{"jsonrpc":"1.0","method":"x"}`},
		{"missing method, no id", `{"jsonrpc":"2.0"}`},
		{"response-shaped, no id", `{"jsonrpc":"2.0","result":5}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := j.HandleRPCJSONRawMessage(ctx, []byte(tt.input))
			if len(res) == 0 {
				t.Fatal("invalid id-less request must be answered, got silence")
			}
			var resp structs.Response
			if err := resp.UnmarshalJSON(res); err != nil {
				t.Fatalf("unparsable response %s: %v", res, err)
			}
			if resp.Error == nil || resp.Error.Code != InvalidRequestErrorCode {
				t.Fatalf("want -32600, got %s", res)
			}
			if string(resp.ID) != "null" && len(resp.ID) != 0 {
				t.Fatalf("id must be null, got %q", resp.ID)
			}
		})
	}
}

// A VALID notification must stay silent even when its method does not exist
// or the handler fails — the fix above must not regress notification silence.
func TestJSONRPC_ValidNotificationStaysSilent(t *testing.T) {
	j := New()
	ctx := context.Background()

	for _, input := range []string{
		`{"jsonrpc":"2.0","method":"no_such_method"}`,
		`{"jsonrpc":"2.0","method":"no_such_method","params":{"a":1}}`,
	} {
		if res := j.HandleRPCJSONRawMessage(ctx, []byte(input)); len(res) != 0 {
			t.Fatalf("notification must not be answered, got %s", res)
		}
	}
}

// A batch mixing a valid notification with an invalid id-less entry answers
// only for the invalid entry (id:null), spec §6 examples.
func TestJSONRPC_BatchInvalidIDLessEntryIsAnswered(t *testing.T) {
	j := New()
	ctx := context.Background()

	res := j.HandleRPCJSONRawMessage(ctx,
		[]byte(`[{"jsonrpc":"2.0","method":"note"},{"jsonrpc":"1.0","method":"bad"}]`))
	var batch []structs.Response
	if err := json.Unmarshal(res, &batch); err != nil {
		t.Fatalf("want a batch response, got %s: %v", res, err)
	}
	if len(batch) != 1 || batch[0].Error == nil || batch[0].Error.Code != InvalidRequestErrorCode {
		t.Fatalf("want exactly one -32600 entry, got %s", res)
	}
}

// An oversized payload that verifiably carries no id (a notification or an
// all-notification batch) is rejected SILENTLY; one with an id — or one
// whose id-lessness cannot be proven — keeps the id:null rejection.
func TestJSONRPC_OversizedNotificationRejectedSilently(t *testing.T) {
	j := New()
	j.SetMaxMessageSize(96)
	ctx := context.Background()
	pad := `"` + strings.Repeat("a", 128) + `"`

	silent := []string{
		`{"jsonrpc":"2.0","method":"m","params":` + pad + `}`,
		`[{"jsonrpc":"2.0","method":"m","params":` + pad + `}, {"jsonrpc":"2.0","method":"n","params":` + pad + `}]`,
	}
	for _, input := range silent {
		if res := j.HandleRPCJSONRawMessage(ctx, []byte(input)); len(res) != 0 {
			t.Fatalf("oversized notification must be rejected silently, got %s", res)
		}
	}

	answered := []string{
		`{"jsonrpc":"2.0","method":"m","id":1,"params":` + pad + `}`,      // has id
		`[{"jsonrpc":"2.0","method":"m","params":` + pad + `},{"id":2}]`,  // batch with an id
		`{"jsonrpc":"2.0","method":"m","params":` + pad,                   // malformed: unprovable
		strings.Repeat("x", 128),                                          // garbage
		`{"\u0069d":3,"jsonrpc":"2.0","method":"m","params":` + pad + `}`, // escaped "id" key: unprovable
		`[` + strings.Repeat(`1,`, 100) + `1]`,                            // non-object batch elements
	}
	for _, input := range answered {
		res := j.HandleRPCJSONRawMessage(ctx, []byte(input))
		if string(res) != string(errorRequestTooLarge()) {
			t.Fatalf("input %.40q must draw the id:null rejection, got %s", input, res)
		}
	}
}

// The same silence rule holds for the batch-count cap.
func TestJSONRPC_BatchTooLargeAllNotificationsSilent(t *testing.T) {
	j := New()
	j.SetMaxBatchSize(2)
	ctx := context.Background()

	notifications := `[{"jsonrpc":"2.0","method":"a"},{"jsonrpc":"2.0","method":"b"},{"jsonrpc":"2.0","method":"c"}]`
	if res := j.HandleRPCJSONRawMessage(ctx, []byte(notifications)); len(res) != 0 {
		t.Fatalf("all-notification oversized batch must be silent, got %s", res)
	}

	withID := `[{"jsonrpc":"2.0","method":"a"},{"jsonrpc":"2.0","method":"b"},{"jsonrpc":"2.0","method":"c","id":1}]`
	if res := j.HandleRPCJSONRawMessage(ctx, []byte(withID)); string(res) != string(errorBatchTooLarge()) {
		t.Fatalf("batch with an id must keep the id:null rejection, got %s", res)
	}
}

// lacksResponseID unit coverage: the scanner must only report true when the
// absence of an id is provable.
func TestLacksResponseID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"notification", `{"jsonrpc":"2.0","method":"m"}`, true},
		{"empty object", `{}`, true},
		{"id present", `{"id":1,"method":"m"}`, false},
		{"id null still correlates", `{"jsonrpc":"2.0","method":"m","id":null}`, false},
		{"nested id ignored", `{"method":"m","params":{"id":5}}`, true},
		{"id as string value ignored", `{"method":"id"}`, true},
		{"id key anywhere top-level", `{"method":"m","id":7}`, false},
		{"fully escaped id key unprovable", `{"\u0069d":1}`, false},
		{"partially escaped id key unprovable", `{"i\u0064":1}`, false},
		{"escaped non-id key still unprovable", `{"me\u0074hod":"m"}`, false},
		{"batch of notifications", `[{"method":"a"},{"method":"b"}]`, true},
		{"batch with one id", `[{"method":"a"},{"method":"b","id":1}]`, false},
		{"batch nested ids ignored", `[{"params":{"id":1},"method":"a"}]`, true},
		{"batch bare elements", `[1,2,3]`, false},
		{"batch nested array element", `[[{"method":"a"}]]`, false},
		{"empty batch", `[]`, false},
		{"empty batch with space", `[  ]`, false},
		{"trailing garbage", `{"method":"m"}{}`, false},
		{"unbalanced", `{"method":"m"`, false},
		{"scalar", `42`, false},
		{"whitespace only", `   `, false},
		{"id after nested object", `{"params":{"a":[1]},"id":3}`, false},
		{"comma resets key position", `{"a":1,"id":2}`, false},
		{"colon in string value", `{"a":"x:y","method":"m"}`, true},
		{"brace in string value", `{"a":"}","id":1}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lacksResponseID([]byte(tt.input)); got != tt.want {
				t.Fatalf("lacksResponseID(%s) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
