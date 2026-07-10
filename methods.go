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

// rpcDiscover is the OpenRPC service-discovery method. JSON-RPC 2.0 §4.1
// reserves the "rpc." prefix "for rpc-internal methods and extensions";
// rpc.discover is exactly such a sanctioned extension (the OpenRPC spec defines
// it), so it is the one reserved name a server is allowed to register — e.g. to
// serve the document from the openrpc subpackage.
const rpcDiscover = "rpc.discover"

// registerMethod atomically installs a method and its introspection metadata,
// rejecting a duplicate name. Both maps are kept in lockstep so Methods()
// always mirrors the dispatch registry.
func (j *JSONRPC) registerMethod(name string, method RPCMethod, info MethodInfo) error {
	return j.updateRegistry(func(c *config) error {
		if strings.HasPrefix(name, "rpc.") && name != rpcDiscover {
			return fmt.Errorf("method name %q: the \"rpc.\" prefix is reserved (JSON-RPC 2.0 §4.1); only %q is permitted", name, rpcDiscover)
		}
		if _, ok := c.methods[name]; ok {
			return fmt.Errorf("method %q already exists", name)
		}
		c.methods[name] = method
		c.methodInfo[name] = info
		return nil
	})
}
