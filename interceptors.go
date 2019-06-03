package jsonrpc

import (
	"context"
	"encoding/json"
)

//InterceptorCallMethod interface for interceptor method
type InterceptorCallMethod func(ctx context.Context,
	methodName string,
	data json.RawMessage,
	id interface{}) (context.Context, int, error)

//InterceptorCallMethods registry for global interceptors
type InterceptorCallMethods []InterceptorCallMethod

//RegisterGlobalInterceptorCall register global interceptors
func (j *JSONRPC) RegisterGlobalInterceptorCall(method InterceptorCallMethod) {
	j.globalInterceptors = append(j.globalInterceptors, method)
}

func (j *JSONRPC) callGlobalInterceptors(ctx context.Context,
	methodName string,
	data json.RawMessage,
	id interface{}) (context.Context, int, error) {
	for _, interceptor := range j.globalInterceptors {
		ctx, code, err := interceptor(ctx, methodName, data, id)
		if err != nil {
			return ctx, code, err
		}
	}

	return ctx, OK, nil
}
