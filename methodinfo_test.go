package jsonrpc

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

type miParams struct {
	A int `json:"a"`
}

type miResult struct {
	C int `json:"c"`
}

func miHandler(ctx context.Context, p miParams) (miResult, error) {
	return miResult{C: p.A}, nil
}

func TestMethods_CapturesTypesAndMetadata(t *testing.T) {
	j := New()
	j.SetLogger(nil)

	err := RegisterTyped(j, "thing.do", miHandler,
		WithSummary("do a thing"),
		WithDescription("does the thing in detail"),
		WithTags("thing", "public"),
		WithTags("extra"),
		WithDeprecated(),
		WithErrors(
			ErrorInfo{Code: 2002, Message: "validation_failed", Description: "bad input"},
			ErrorInfo{Code: 2001, Message: "not_found", Description: "no such thing"},
		),
		WithExample("basic", miParams{A: 7}, miResult{C: 7}),
		WithExtra("auth", "bearer"),
	)
	if err != nil {
		t.Fatal(err)
	}

	methods := j.Methods()
	if len(methods) != 1 {
		t.Fatalf("expected 1 method, got %d", len(methods))
	}
	m := methods[0]

	if m.Name != "thing.do" {
		t.Errorf("name = %q, want thing.do", m.Name)
	}
	if m.Params != reflect.TypeOf(miParams{}) {
		t.Errorf("params type = %v, want miParams", m.Params)
	}
	if m.Result != reflect.TypeOf(miResult{}) {
		t.Errorf("result type = %v, want miResult", m.Result)
	}
	if m.Summary != "do a thing" {
		t.Errorf("summary = %q", m.Summary)
	}
	if m.Description != "does the thing in detail" {
		t.Errorf("description = %q", m.Description)
	}
	if !m.Deprecated {
		t.Error("expected deprecated")
	}
	if got := m.Tags; len(got) != 3 || got[0] != "thing" || got[1] != "public" || got[2] != "extra" {
		t.Errorf("tags = %v, want [thing public extra]", got)
	}
	if len(m.Errors) != 2 || m.Errors[0].Code != 2002 || m.Errors[1].Code != 2001 {
		t.Errorf("errors = %+v", m.Errors)
	}
	if len(m.Examples) != 1 || m.Examples[0].Name != "basic" {
		t.Errorf("examples = %+v", m.Examples)
	}
	if p, ok := m.Examples[0].Params.(miParams); !ok || p.A != 7 {
		t.Errorf("example params = %+v", m.Examples[0].Params)
	}
	if m.Extra["auth"] != "bearer" {
		t.Errorf("extra = %+v", m.Extra)
	}
}

func TestMethods_UntypedIsNameOnly(t *testing.T) {
	j := New()

	raw := func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error) {
		return nil, OK, nil
	}
	if err := j.RegisterMethod("raw.method", raw); err != nil {
		t.Fatal(err)
	}

	methods := j.Methods()
	if len(methods) != 1 {
		t.Fatalf("expected 1 method, got %d", len(methods))
	}
	m := methods[0]
	if m.Name != "raw.method" {
		t.Errorf("name = %q", m.Name)
	}
	if m.Params != nil || m.Result != nil {
		t.Errorf("untyped method should have nil param/result types, got params=%v result=%v", m.Params, m.Result)
	}
}

func TestMethods_EmptyStructParamsStaysNonNil(t *testing.T) {
	j := New()

	err := RegisterTyped(j, "ping", func(ctx context.Context, _ struct{}) (miResult, error) {
		return miResult{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	m := j.Methods()[0]
	if m.Params == nil {
		t.Fatal("struct{} params must be recorded as a non-nil zero-field type, not nil")
	}
	if m.Params.Kind() != reflect.Struct || m.Params.NumField() != 0 {
		t.Errorf("expected zero-field struct, got %v with %d fields", m.Params, m.Params.NumField())
	}
}

func TestMethods_SortedByName(t *testing.T) {
	j := New()

	h := func(ctx context.Context, _ miParams) (miResult, error) { return miResult{}, nil }
	for _, name := range []string{"zebra", "alpha", "mike"} {
		if err := RegisterTyped(j, name, h); err != nil {
			t.Fatal(err)
		}
	}

	methods := j.Methods()
	got := []string{methods[0].Name, methods[1].Name, methods[2].Name}
	want := []string{"alpha", "mike", "zebra"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestMethods_ReturnsClonesNotAliases(t *testing.T) {
	j := New()

	err := RegisterTyped(j, "thing.do", miHandler,
		WithTags("original"),
		WithErrors(ErrorInfo{Code: 1, Message: "one"}),
		WithExtra("k", "v"),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Mutate the first snapshot's slices/map.
	first := j.Methods()[0]
	first.Tags[0] = "TAMPERED"
	first.Errors[0].Message = "TAMPERED"
	first.Extra["k"] = "TAMPERED"

	// A fresh snapshot must be unaffected.
	second := j.Methods()[0]
	if second.Tags[0] != "original" {
		t.Errorf("tags leaked mutation: %v", second.Tags)
	}
	if second.Errors[0].Message != "one" {
		t.Errorf("errors leaked mutation: %v", second.Errors)
	}
	if second.Extra["k"] != "v" {
		t.Errorf("extra leaked mutation: %v", second.Extra)
	}
}

func TestMethods_Empty(t *testing.T) {
	j := New()
	if got := j.Methods(); len(got) != 0 {
		t.Errorf("expected no methods, got %d", len(got))
	}
}

func TestRegisterTyped_NoOptsStillRecords(t *testing.T) {
	j := New()

	if err := RegisterTyped(j, "bare", miHandler); err != nil {
		t.Fatal(err)
	}
	m := j.Methods()[0]
	if m.Name != "bare" || m.Params == nil || m.Result == nil {
		t.Errorf("bare registration should still record name+types, got %+v", m)
	}
	if m.Summary != "" || len(m.Tags) != 0 || len(m.Errors) != 0 {
		t.Errorf("bare registration should have no metadata, got %+v", m)
	}
}
