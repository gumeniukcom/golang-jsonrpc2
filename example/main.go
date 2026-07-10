package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	jrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpchttp"
)

func main() {
	serv := jrpc.New()

	if err := serv.RegisterMethod("sum", sum); err != nil {
		panic(err)
	}

	mux := http.NewServeMux()
	// jsonrpchttp.Handler bounds the body (1 MiB default), answers
	// notifications with 204, and maps transport failures to HTTP codes.
	mux.Handle("/", jsonrpchttp.Handler(serv))

	srv := &http.Server{
		Addr:              ":8088",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
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

func sum(_ context.Context, data json.RawMessage) (json.RawMessage, int, error) {
	inc := &income{}
	if err := json.Unmarshal(data, inc); err != nil {
		return nil, jrpc.InvalidParamsErrorCode, err
	}

	mdata, err := json.Marshal(outcome{Sum: inc.A + inc.B})
	if err != nil {
		return nil, jrpc.InternalErrorCode, err
	}
	return mdata, jrpc.OK, nil
}
