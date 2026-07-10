package openrpc_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/openrpc"
)

type createUserParams struct {
	Name    string            `json:"name"`
	Age     int               `json:"age,omitempty"`
	Email   *string           `json:"email,omitempty"`
	Tags    []string          `json:"tags,omitempty"`
	Meta    map[string]string `json:"meta,omitempty"`
	Born    time.Time         `json:"born"`
	Blob    []byte            `json:"blob,omitempty"`
	private string            //nolint:unused // must be skipped by the generator
	Skipped string            `json:"-"`
}

type user struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// node is recursive: schema generation must terminate via $ref.
type node struct {
	Value    int     `json:"value"`
	Children []*node `json:"children,omitempty"`
}

func buildServer(t *testing.T) *jsonrpc.JSONRPC {
	t.Helper()
	j := jsonrpc.New()

	err := jsonrpc.RegisterTyped(j, "user.create",
		func(_ context.Context, _ createUserParams) (user, error) { return user{}, nil },
		jsonrpc.WithSummary("Create a user"),
		jsonrpc.WithDescription("Creates a user record."),
		jsonrpc.WithTags("users", "write"),
		jsonrpc.WithErrors(jsonrpc.ErrorInfo{Code: 1001, Message: "user_exists", Description: "duplicate"}),
		jsonrpc.WithExample("basic", createUserParams{Name: "bob", Age: 42}, user{ID: 1, Name: "bob"}),
		jsonrpc.WithExtra("auth", "required"), // private: must never reach the document
		jsonrpc.WithPublishedExtra("stability", "beta"),
		jsonrpc.WithPublic(),
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := jsonrpc.RegisterTyped(j, "tree.walk",
		func(_ context.Context, n node) (node, error) { return n, nil },
		jsonrpc.WithDeprecated(),
		jsonrpc.WithPublic(),
	); err != nil {
		t.Fatal(err)
	}

	// Untyped method: no type info, params/result must degrade to "any".
	if err := j.RegisterMethod("legacy", func(_ context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		return data, jsonrpc.OK, nil
	}); err != nil {
		t.Fatal(err)
	}

	return j
}

func TestRegisterDiscover(t *testing.T) {
	j := buildServer(t)

	if err := openrpc.RegisterDiscover(j, openrpc.Info{Title: "Test API", Version: "1.2.3"},
		jsonrpc.WithTags("meta")); err != nil {
		t.Fatalf("register: %v", err)
	}

	// rpc.discover is now in the registry (permitted despite the reserved prefix).
	var found bool
	for _, m := range j.Methods() {
		if m.Name == "rpc.discover" {
			found = true
		}
	}
	if !found {
		t.Fatal("rpc.discover not registered")
	}

	// Dispatching it returns the OpenRPC document, listing every method including
	// rpc.discover itself.
	req := []byte(`{"jsonrpc":"2.0","id":1,"method":"rpc.discover","params":{}}`)
	resp := j.HandleRPCJSONRawMessage(context.Background(), req)
	var env struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resp, &env); err != nil {
		t.Fatalf("bad response: %v (%s)", err, resp)
	}
	if env.Error != nil {
		t.Fatalf("rpc.discover errored: code %d", env.Error.Code)
	}
	var doc struct {
		OpenRPC string `json:"openrpc"`
		Info    struct {
			Title string `json:"title"`
		} `json:"info"`
		Methods []struct {
			Name string `json:"name"`
		} `json:"methods"`
	}
	if err := json.Unmarshal(env.Result, &doc); err != nil {
		t.Fatalf("result is not an OpenRPC document: %v", err)
	}
	if doc.OpenRPC != "1.3.2" || doc.Info.Title != "Test API" {
		t.Errorf("unexpected doc header: %+v", doc)
	}
	names := map[string]bool{}
	for _, m := range doc.Methods {
		names[m.Name] = true
	}
	for _, want := range []string{"user.create", "tree.walk", "rpc.discover"} {
		if !names[want] {
			t.Errorf("document is missing public method %q", want)
		}
	}
	// Discovery is default-deny: the unannotated method must NOT appear.
	if names["legacy"] {
		t.Error("unannotated method \"legacy\" must be hidden from discovery")
	}

	// Registering it twice is a duplicate error.
	if err := openrpc.RegisterDiscover(j, openrpc.Info{Title: "x", Version: "0"}); err == nil {
		t.Error("second RegisterDiscover must fail (duplicate)")
	}
}

// Every common client spelling of "no params" must work: absent, null, {},
// and the positional [] (the OpenRPC spec's own discover definition uses []).
func TestRegisterDiscoverParamsVariants(t *testing.T) {
	j := buildServer(t)
	if err := openrpc.RegisterDiscover(j, openrpc.Info{Title: "t", Version: "1"}); err != nil {
		t.Fatal(err)
	}

	for _, params := range []string{"", `,"params":null`, `,"params":{}`, `,"params":[]`} {
		req := []byte(`{"jsonrpc":"2.0","id":1,"method":"rpc.discover"` + params + `}`)
		resp := j.HandleRPCJSONRawMessage(context.Background(), req)
		var env struct {
			Error *struct {
				Code int `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(resp, &env); err != nil {
			t.Fatalf("params %q: bad response %s", params, resp)
		}
		if env.Error != nil {
			t.Errorf("params %q: rpc.discover errored with code %d", params, env.Error.Code)
		}
	}
}

// The document is built per call, so a method registered AFTER discovery was
// first called still appears — registration order does not matter.
func TestRegisterDiscoverReflectsLateRegistrations(t *testing.T) {
	j := buildServer(t)
	if err := openrpc.RegisterDiscover(j, openrpc.Info{Title: "t", Version: "1"}); err != nil {
		t.Fatal(err)
	}

	discover := func() map[string]bool {
		resp := j.HandleRPCJSONRawMessage(context.Background(),
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"rpc.discover"}`))
		var env struct {
			Result struct {
				Methods []struct {
					Name    string `json:"name"`
					Summary string `json:"summary"`
				} `json:"methods"`
			} `json:"result"`
		}
		if err := json.Unmarshal(resp, &env); err != nil {
			t.Fatalf("bad response: %v (%s)", err, resp)
		}
		names := map[string]bool{}
		for _, m := range env.Result.Methods {
			names[m.Name] = true
		}
		return names
	}

	if discover()["late.method"] {
		t.Fatal("late.method must not exist yet")
	}
	if err := jsonrpc.RegisterTyped(j, "late.method", func(context.Context, struct{}) (string, error) {
		return "", nil
	}, jsonrpc.WithPublic()); err != nil {
		t.Fatal(err)
	}
	if !discover()["late.method"] {
		t.Error("a method registered after the first discover call must appear in the next document")
	}
}

// User options override the built-in summary (options apply last-write-wins).
func TestRegisterDiscoverSummaryOverride(t *testing.T) {
	j := buildServer(t)
	if err := openrpc.RegisterDiscover(j, openrpc.Info{Title: "t", Version: "1"},
		jsonrpc.WithSummary("custom summary")); err != nil {
		t.Fatal(err)
	}
	for _, m := range j.Methods() {
		if m.Name == "rpc.discover" {
			if m.Summary != "custom summary" {
				t.Errorf("summary = %q, want the override", m.Summary)
			}
			return
		}
	}
	t.Fatal("rpc.discover not found")
}

func TestDocumentGeneratesValidOpenRPC(t *testing.T) {
	j := buildServer(t)

	raw, err := openrpc.Document(openrpc.Info{Title: "Test API", Version: "1.2.3"}, j.Methods())
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		t.Fatalf("document must be valid JSON: %s", raw)
	}

	var doc struct {
		OpenRPC string `json:"openrpc"`
		Info    struct {
			Title   string `json:"title"`
			Version string `json:"version"`
		} `json:"info"`
		Methods []struct {
			Name           string `json:"name"`
			Summary        string `json:"summary"`
			Deprecated     bool   `json:"deprecated"`
			ParamStructure string `json:"paramStructure"`
			Params         []struct {
				Name     string          `json:"name"`
				Required bool            `json:"required"`
				Schema   json.RawMessage `json:"schema"`
			} `json:"params"`
			Result *struct {
				Schema json.RawMessage `json:"schema"`
			} `json:"result"`
			Errors []struct {
				Code int `json:"code"`
			} `json:"errors"`
			Examples []struct {
				Name   string `json:"name"`
				Params []struct {
					Name string `json:"name"`
				} `json:"params"`
			} `json:"examples"`
			Extra map[string]any `json:"x-extra"`
		} `json:"methods"`
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}

	if doc.OpenRPC != "1.3.2" || doc.Info.Title != "Test API" {
		t.Fatalf("bad envelope: %s", raw)
	}
	if len(doc.Methods) != 3 {
		t.Fatalf("expected 3 methods, got %d", len(doc.Methods))
	}

	byName := map[string]int{}
	for i, m := range doc.Methods {
		byName[m.Name] = i
	}

	cu := doc.Methods[byName["user.create"]]
	if cu.Summary != "Create a user" || cu.ParamStructure != "by-name" {
		t.Fatalf("user.create metadata wrong: %+v", cu)
	}
	paramNames := map[string]bool{}
	for _, p := range cu.Params {
		paramNames[p.Name] = p.Required
	}
	if !paramNames["name"] || paramNames["age"] || paramNames["skipped"] || paramNames["private"] {
		t.Fatalf("params must respect json tags and omitempty: %v", paramNames)
	}
	if _, ok := paramNames["Skipped"]; ok {
		t.Fatalf("json:\"-\" field must be skipped: %v", paramNames)
	}
	if len(cu.Errors) != 1 || cu.Errors[0].Code != 1001 {
		t.Fatalf("errors missing: %+v", cu.Errors)
	}
	if len(cu.Examples) != 1 || len(cu.Examples[0].Params) == 0 {
		t.Fatalf("examples missing: %+v", cu.Examples)
	}
	// Private metadata (WithExtra) must never be published; only
	// WithPublishedExtra values appear as x-extra.
	if _, leaked := cu.Extra["auth"]; leaked {
		t.Fatalf("private Extra leaked into the document: %+v", cu.Extra)
	}
	if cu.Extra["stability"] != "beta" {
		t.Fatalf("published extra missing: %+v", cu.Extra)
	}

	tw := doc.Methods[byName["tree.walk"]]
	if !tw.Deprecated {
		t.Fatal("tree.walk must be deprecated")
	}

	// Recursive type must be present in components and referenced.
	if _, ok := doc.Components.Schemas["node"]; !ok {
		t.Fatalf("recursive node schema must land in components: %s", raw)
	}
	if !strings.Contains(string(raw), `"$ref":"#/components/schemas/node"`) {
		t.Fatalf("recursion must resolve via $ref: %s", raw)
	}

	// Untyped method degrades to empty params and any-result.
	lg := doc.Methods[byName["legacy"]]
	if len(lg.Params) != 0 || lg.Result == nil {
		t.Fatalf("legacy method must have no params and an any-result, got: %+v", lg)
	}
}

type embedded struct {
	X int `json:"x"`
}

type withNilEmbedded struct {
	*embedded
	Y int `json:"y"`
}

// An example value with a nil embedded pointer must not panic the generator.
func TestExampleWithNilEmbeddedPointerDoesNotPanic(t *testing.T) {
	j := jsonrpc.New()
	err := jsonrpc.RegisterTyped(j, "m",
		func(_ context.Context, _ withNilEmbedded) (int, error) { return 0, nil },
		jsonrpc.WithExample("bad", withNilEmbedded{Y: 5}, 1)) // embedded is nil
	if err != nil {
		t.Fatal(err)
	}

	raw, err := openrpc.Document(openrpc.Info{Title: "t", Version: "0"}, j.Methods())
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		t.Fatalf("document must stay valid JSON: %s", raw)
	}
	if !strings.Contains(string(raw), `"name":"y"`) {
		t.Fatalf("reachable example fields must survive: %s", raw)
	}
}

type box[T any] struct {
	Value T       `json:"value"`
	Next  *box[T] `json:"next,omitempty"`
}

// Generic type names contain brackets — component keys must be sanitized to
// the ^[a-zA-Z0-9._-]+$ charset OpenRPC/OpenAPI validators require.
func TestGenericComponentNamesSanitized(t *testing.T) {
	j := jsonrpc.New()
	if err := jsonrpc.RegisterTyped(j, "g",
		func(_ context.Context, b box[string]) (box[string], error) { return b, nil }); err != nil {
		t.Fatal(err)
	}

	raw, err := openrpc.Document(openrpc.Info{Title: "t", Version: "0"}, j.Methods())
	if err != nil {
		t.Fatal(err)
	}

	var doc struct {
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Components.Schemas) == 0 {
		t.Fatalf("generic struct must land in components: %s", raw)
	}
	for name := range doc.Components.Schemas {
		for _, r := range name {
			ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
				r == '.' || r == '_' || r == '-'
			if !ok {
				t.Fatalf("component key %q contains invalid rune %q", name, r)
			}
		}
	}
}

func TestSchemaKinds(t *testing.T) {
	j := jsonrpc.New()
	if err := jsonrpc.RegisterTyped(j, "kinds",
		func(_ context.Context, _ createUserParams) (map[string][]int, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}

	raw, err := openrpc.Document(openrpc.Info{Title: "k", Version: "0"}, j.Methods())
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{
		`"type":"string"`,                        // Name
		`"type":"integer"`,                       // Age
		`"format":"date-time"`,                   // Born
		`"contentEncoding":"base64"`,             // Blob
		`"additionalProperties":{"type":"array"`, // map[string][]int result
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("schema output must contain %q: %s", want, s)
		}
	}
}

// Discovery visibility is default-deny end to end: a server that registers
// discovery without annotating anything publishes only rpc.discover itself.
func TestDiscoverDefaultDeny(t *testing.T) {
	j := jsonrpc.New()
	if err := jsonrpc.RegisterTyped(j, "secret.op", func(context.Context, struct{}) (string, error) {
		return "", nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := openrpc.RegisterDiscover(j, openrpc.Info{Title: "t", Version: "1"}); err != nil {
		t.Fatal(err)
	}

	resp := j.HandleRPCJSONRawMessage(context.Background(),
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"rpc.discover"}`))
	var env struct {
		Result struct {
			Methods []struct {
				Name string `json:"name"`
			} `json:"methods"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resp, &env); err != nil {
		t.Fatalf("bad response: %v (%s)", err, resp)
	}
	if len(env.Result.Methods) != 1 || env.Result.Methods[0].Name != "rpc.discover" {
		t.Fatalf("default-deny document must contain only rpc.discover, got %+v", env.Result.Methods)
	}
	// Hidden is not blocked: the unlisted method is still callable.
	resp = j.HandleRPCJSONRawMessage(context.Background(),
		[]byte(`{"jsonrpc":"2.0","id":2,"method":"secret.op"}`))
	var call struct {
		Error *json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(resp, &call); err != nil {
		t.Fatalf("bad response: %v (%s)", err, resp)
	}
	if call.Error != nil {
		t.Fatalf("hidden method must remain callable, got error %s", *call.Error)
	}
}

// openrpc.Public filters to the WithPublic subset without mutating input.
func TestPublicFilter(t *testing.T) {
	j := buildServer(t) // user.create + tree.walk public, legacy not
	all := j.Methods()
	pub := openrpc.Public(all)

	names := map[string]bool{}
	for _, m := range pub {
		names[m.Name] = true
	}
	if !names["user.create"] || !names["tree.walk"] || names["legacy"] {
		t.Fatalf("Public() filtered wrong set: %v", names)
	}
	if len(all) != 3 {
		t.Fatalf("input slice must be untouched, got %d entries", len(all))
	}
}
