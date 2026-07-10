package jsonrpc

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

func TestBatchResultAs(t *testing.T) {
	r, err := BatchResultAs[int](BatchResult{Result: json.RawMessage(`7`)})
	if err != nil || r != 7 {
		t.Fatalf("typed decode: got %d, %v", r, err)
	}

	_, err = BatchResultAs[int](BatchResult{Error: &structs.Error{Code: -32601, Message: "nope"}})
	var rpcErr *structs.Error
	if err == nil {
		t.Fatal("error result must surface an error")
	}
	if e, ok := err.(*structs.Error); !ok || e.Code != -32601 {
		t.Fatalf("expected *structs.Error(-32601), got: %v", err)
	}
	_ = rpcErr

	if v, err := BatchResultAs[string](BatchResult{}); err != nil || v != "" {
		t.Fatalf("empty result must be zero value, got %q, %v", v, err)
	}
}

func TestMarshalBatchAssignsIDsAndSkipsNotifications(t *testing.T) {
	var n int64
	next := func() structs.ID {
		n++
		return structs.ID(strconv.AppendInt(nil, n, 10))
	}
	specs := []Spec{
		{Method: "a", Params: map[string]int{"x": 1}},
		{Method: "b", Notify: true},
		{Method: "c", Params: 2},
	}
	frame, ids, err := MarshalBatch(specs, next)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 || string(ids[0]) != "1" || ids[1] != nil || string(ids[2]) != "2" {
		t.Fatalf("ids wrong: %v", ids)
	}
	if !json.Valid(frame) || frame[0] != '[' {
		t.Fatalf("frame must be a JSON array: %s", frame)
	}
	var arr []structs.Request
	if err := json.Unmarshal(frame, &arr); err != nil || len(arr) != 3 {
		t.Fatalf("frame must decode to 3 requests: %v (%s)", err, frame)
	}
	if arr[1].Method != "b" || len(arr[1].ID) != 0 {
		t.Fatalf("notification entry must carry no id: %+v", arr[1])
	}
}

func TestBatchResultFromResponse(t *testing.T) {
	res := json.RawMessage(`{"ok":true}`)
	if br := BatchResultFromResponse(&structs.Response{Result: &res}); string(br.Result) != `{"ok":true}` || br.Error != nil {
		t.Fatalf("result response: %+v", br)
	}
	e := &structs.Error{Code: -32000, Message: "x"}
	if br := BatchResultFromResponse(&structs.Response{Error: e}); br.Error != e || br.Result != nil {
		t.Fatalf("error response: %+v", br)
	}
}
