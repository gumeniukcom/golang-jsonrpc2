// Command mcp-stdio is a minimal MCP-shaped JSON-RPC server over stdio,
// used by docs/mcp-lsp.md. It implements just enough of the Model Context
// Protocol wire surface to be inspected with an MCP client: initialize,
// tools/list, and tools/call for one "add" tool.
//
// The MCP peer is whatever process spawned us: everything on stdin is
// untrusted input, stdout carries protocol frames only, and all logging
// goes to stderr (slog's default).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/jsonrpcstdio"
)

type initializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      serverInfo     `json:"serverInfo"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type toolsListResult struct {
	Tools []tool `json:"tools"`
}

type toolsCallParams struct {
	Name      string    `json:"name"`
	Arguments addParams `json:"arguments"`
}

type addParams struct {
	A int `json:"a"`
	B int `json:"b"`
}

type toolsCallResult struct {
	Content []content `json:"content"`
}

type content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func main() {
	rpc := jsonrpc.New()

	must(jsonrpc.RegisterTyped(rpc, "initialize", func(_ context.Context, _ map[string]any) (initializeResult, error) {
		return initializeResult{
			ProtocolVersion: "2025-06-18",
			Capabilities:    map[string]any{"tools": map[string]any{}},
			ServerInfo:      serverInfo{Name: "mcp-stdio-example", Version: "0.1.0"},
		}, nil
	}))

	must(jsonrpc.RegisterTyped(rpc, "tools/list", func(_ context.Context, _ struct{}) (toolsListResult, error) {
		return toolsListResult{Tools: []tool{{
			Name:        "add",
			Description: "Add two integers.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"a": map[string]any{"type": "integer"},
					"b": map[string]any{"type": "integer"},
				},
				"required": []string{"a", "b"},
			},
		}}}, nil
	}))

	must(jsonrpc.RegisterTyped(rpc, "tools/call", func(_ context.Context, p toolsCallParams) (toolsCallResult, error) {
		if p.Name != "add" {
			// MCP convention: an unknown tool name is an invalid-params error
			// (-32602) on tools/call, not method-not-found.
			return toolsCallResult{}, jsonrpc.NewRPCError(jsonrpc.InvalidParamsErrorCode, fmt.Errorf("unknown tool %q", p.Name))
		}
		return toolsCallResult{Content: []content{{
			Type: "text",
			Text: fmt.Sprintf("%d", p.Arguments.A+p.Arguments.B),
		}}}, nil
	}))

	// MCP clients send notifications/initialized after the handshake; a
	// notification needs no result — registering it silences
	// method-not-found noise in the server log.
	must(jsonrpc.RegisterTyped(rpc, "notifications/initialized", func(_ context.Context, _ map[string]any) (struct{}, error) {
		return struct{}{}, nil
	}))

	// stdout is the protocol channel; Serve returns nil when the client
	// closes our stdin — the MCP shutdown signal.
	if err := jsonrpcstdio.Serve(context.Background(), rpc, jsonrpcstdio.FramingNDJSON, os.Stdin, os.Stdout); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func must(err error) {
	if err != nil {
		slog.Error("register", "error", err)
		os.Exit(1)
	}
}
