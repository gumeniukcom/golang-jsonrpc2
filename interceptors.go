package jsonrpc

import (
	"context"
	"encoding/json"
)

// InterceptorCallMethod is the function signature for interceptors that run
// before each method call. Interceptors can modify the context, return an
// error code to abort, or return nil to continue the chain.
type InterceptorCallMethod func(ctx context.Context,
	methodName string,
	data json.RawMessage,
	id any) (context.Context, int, error)

// InterceptorCallMethods is a slice of global interceptors.
type InterceptorCallMethods []InterceptorCallMethod

// RegisterGlobalInterceptorCall appends an interceptor to the global chain.
func (j *JSONRPC) RegisterGlobalInterceptorCall(method InterceptorCallMethod) {
	_ = j.updateConfig(func(c *config) error {
		c.globalInterceptors = append(c.globalInterceptors, method)
		return nil
	})
}

// callGlobalInterceptors runs the chain from an immutable config snapshot, so
// no locking or defensive copy is needed.
func callGlobalInterceptors(ctx context.Context,
	cfg *config,
	methodName string,
	data json.RawMessage,
	id any) (context.Context, int, error) {
	var code int
	var err error
	for _, interceptor := range cfg.globalInterceptors {
		ctx, code, err = interceptor(ctx, methodName, data, id)
		if err != nil {
			return ctx, code, err
		}
	}

	return ctx, OK, nil
}
