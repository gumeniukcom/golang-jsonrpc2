package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// RPCMethod defines the function signature for JSON-RPC methods.
type RPCMethod func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error)

// RPCMethods is a registry of named RPC methods.
type RPCMethods map[string]RPCMethod

// RegisterMethod registers a new method in the RPC registry. The method is
// recorded with name-only introspection metadata (no params/result types);
// use RegisterTyped to capture types and documentation.
func (j *JSONRPC) RegisterMethod(name string, method RPCMethod) error {
	return j.registerMethod(name, method, MethodInfo{Name: name})
}

// registerMethod atomically installs a method and its introspection metadata,
// rejecting a duplicate name. Both maps are kept in lockstep so Methods()
// always mirrors the dispatch registry.
func (j *JSONRPC) registerMethod(name string, method RPCMethod, info MethodInfo) error {
	return j.updateConfig(func(c *config) error {
		if strings.HasPrefix(name, "rpc.") {
			return fmt.Errorf("method name %q: the \"rpc.\" prefix is reserved by the JSON-RPC 2.0 spec", name)
		}
		if _, ok := c.methods[name]; ok {
			return fmt.Errorf("method %q already exists", name)
		}
		c.methods[name] = method
		c.methodInfo[name] = info
		return nil
	})
}
