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
	j.mu.Lock()
	defer j.mu.Unlock()
	j.globalInterceptors = append(j.globalInterceptors, method)
}

func (j *JSONRPC) callGlobalInterceptors(ctx context.Context,
	methodName string,
	data json.RawMessage,
	id any) (context.Context, int, error) {
	j.mu.RLock()
	interceptors := make(InterceptorCallMethods, len(j.globalInterceptors))
	copy(interceptors, j.globalInterceptors)
	j.mu.RUnlock()

	var code int
	var err error
	for _, interceptor := range interceptors {
		ctx, code, err = interceptor(ctx, methodName, data, id)
		if err != nil {
			return ctx, code, err
		}
	}

	return ctx, OK, nil
}
