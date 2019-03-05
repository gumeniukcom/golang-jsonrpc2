package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
)

//RPCMethod define function interface
type RPCMethod func(ctx context.Context, data json.RawMessage) (json.RawMessage, int, error)

//RPCMethods container for methods
type RPCMethods map[string]RPCMethod

//RegisterMethod new method
func (j *JSONRPC) RegisterMethod(name string, method RPCMethod) error {
	if _, ok := j.methods[name]; ok {
		return fmt.Errorf("error with method \"%s\": it exist", name)
	}
	j.methods[name] = method
	return nil
}
