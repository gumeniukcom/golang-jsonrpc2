package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	jrpc "github.com/gumeniukcom/golang-jsonrpc2"
)

func main() {

	serv := jrpc.New()

	if err := serv.RegisterMethod("sum", sum); err != nil {
		panic(err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}
		defer r.Body.Close()

		w.Header().Set("Content-Type", "applicaition/json")
		w.WriteHeader(http.StatusOK)
		if _, err = w.Write(serv.HandleRPCJsonRawMessage(ctx, body)); err != nil {
			panic(err)
		}
	})

	if err := http.ListenAndServe(":8088", nil); err != nil {
		panic(err)
	}
}

type income struct {
	A int `json:"a"`
	B int `json:"b"`
}
type outcome struct {
	Sum int `json:"sum"`
}

func sum(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
	if data == nil {
		return nil, jrpc.InvalidRequestErrorCode, fmt.Errorf("empty request")
	}
	inc := &income{}
	err := json.Unmarshal(data, inc)
	if err != nil {
		return nil, jrpc.InvalidRequestErrorCode, err
	}

	C := outcome{
		Sum: inc.A + inc.B,
	}

	mdata, err := json.Marshal(C)
	if err != nil {
		return nil, jrpc.InternalErrorCode, err
	}
	return mdata, jrpc.OK, nil
}
