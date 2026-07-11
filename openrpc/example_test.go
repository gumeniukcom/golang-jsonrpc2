package openrpc_test

import (
	"context"
	"encoding/json"
	"fmt"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/openrpc"
)

type addParams struct {
	A int `json:"a"`
	B int `json:"b"`
}

type addResult struct {
	Sum int `json:"sum"`
}

// ExampleRegisterDiscover shows in-band service discovery: methods opt into
// the document with jsonrpc.WithPublic (discovery is default-deny — the
// unannotated internal method never appears), and rpc.discover serves the
// live registry.
func ExampleRegisterDiscover() {
	serv := jsonrpc.New()

	_ = jsonrpc.RegisterTyped(serv, "math.add",
		func(_ context.Context, p addParams) (addResult, error) {
			return addResult{Sum: p.A + p.B}, nil
		},
		jsonrpc.WithSummary("Add two integers"),
		jsonrpc.WithPublishedExtra("stability", "stable"), // published as x-extra
		jsonrpc.WithExtra("auth", "public"),               // private: never published
		jsonrpc.WithPublic(),                              // opt into discovery
	)
	_ = jsonrpc.RegisterTyped(serv, "admin.purge",
		func(_ context.Context, _ struct{}) (string, error) { return "done", nil },
		// no WithPublic: hidden from discovery (but still callable — gate
		// calls with middleware).
	)

	_ = openrpc.RegisterDiscover(serv, openrpc.Info{Title: "Calculator", Version: "1.0.0"})

	resp := serv.HandleRPCJSONRawMessage(context.Background(),
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"rpc.discover"}`))

	var env struct {
		Result struct {
			Info    openrpc.Info `json:"info"`
			Methods []struct {
				Name  string         `json:"name"`
				Extra map[string]any `json:"x-extra"`
			} `json:"methods"`
		} `json:"result"`
	}
	_ = json.Unmarshal(resp, &env)

	fmt.Println("service:", env.Result.Info.Title)
	for _, m := range env.Result.Methods {
		fmt.Println("method:", m.Name, m.Extra)
	}
	// Output:
	// service: Calculator
	// method: math.add map[stability:stable]
	// method: rpc.discover map[]
}

// ExampleDocument shows the out-of-band flow: render the public subset of
// the registry to serve from your own endpoint (pass serv.Methods() verbatim
// instead of the Public filter for a trusted internal audience).
func ExampleDocument() {
	serv := jsonrpc.New()
	_ = jsonrpc.RegisterTyped(serv, "math.add",
		func(_ context.Context, p addParams) (addResult, error) {
			return addResult{Sum: p.A + p.B}, nil
		},
		jsonrpc.WithPublic(),
	)

	doc, err := openrpc.Document(
		openrpc.Info{Title: "Calculator", Version: "1.0.0"},
		openrpc.Public(serv.Methods()),
	)
	if err != nil {
		panic(err)
	}

	var head struct {
		OpenRPC string `json:"openrpc"`
		Info    struct {
			Title string `json:"title"`
		} `json:"info"`
	}
	_ = json.Unmarshal(doc, &head)
	fmt.Println(head.OpenRPC, head.Info.Title)
	// Output:
	// 1.3.2 Calculator
}
