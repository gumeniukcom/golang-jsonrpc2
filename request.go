package jsonrpc

import (
	"github.com/google/uuid"

	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

// NewRequest creates a new JSON-RPC request with a random UUID as the ID.
func NewRequest(methodName string, params ParamsDataMarshaler) (*structs.Request, error) {
	paramsBytes, err := params.MarshalJSON()
	if err != nil {
		return nil, err
	}
	requestID := uuid.New().String()

	req := &structs.Request{
		Version: Version,
		Method:  methodName,
		Params:  paramsBytes,
		ID:      requestID,
	}

	err = validateRequest(req)
	if err != nil {
		return nil, err
	}

	return req, nil
}
