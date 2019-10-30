package jsonrpc

import (
	"context"

	"github.com/gumeniukcom/golang-jsonrpc2/structs"

	"github.com/satori/go.uuid"
)

//Request return request instance
func Request(ctx context.Context, methodName string, params ParamsDataMarshaller) (*structs.Request, error) {
	paramsBytes, err := params.MarshalJSON()
	if err != nil {
		return nil, err
	}
	requestID := uuid.NewV4().String()

	req := &structs.Request{
		Version: Version,
		Method:  methodName,
		Params:  paramsBytes,
		ID:      requestID,
	}

	err = validateRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	return req, nil
}
