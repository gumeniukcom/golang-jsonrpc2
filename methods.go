package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
)

// RPCMethod defines the function signature for JSON-RPC methods.
type RPCMethod func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error)

// RPCMethods is a registry of named RPC methods.
type RPCMethods map[string]RPCMethod

// RegisterMethod registers a new method in the RPC registry.
func (j *JSONRPC) RegisterMethod(name string, method RPCMethod) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, ok := j.methods[name]; ok {
		return fmt.Errorf("method %q already exists", name)
	}
	j.methods[name] = method
	return nil
}
