package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

// ErrorMessages maps error codes to human-readable messages.
type ErrorMessages map[int]string

// RegisterError registers a custom error code and message.
// Codes in the range -32768..-32000 are reserved by the JSON-RPC spec.
// See http://xmlrpc-epi.sourceforge.net/specs/rfc.fault_codes.php
func (j *JSONRPC) RegisterError(code int, msg string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, ok := j.errors[code]; ok {
		return fmt.Errorf("error with code %d already exists", code)
	}
	if code >= -32768 && code <= -32000 {
		return fmt.Errorf("error with code %d: range -32768..-32000 reserved", code)
	}
	j.errors[code] = msg
	return nil
}

// Error builds a JSON-RPC error response for the given error code and request ID.
func (j *JSONRPC) Error(
	ctx context.Context,
	err error,
	errorCode int,
	id any,
) *structs.Response {
	j.mu.RLock()
	errMsg, ok := j.errors[errorCode]
	internalMsg := j.errors[InternalErrorCode]
	j.mu.RUnlock()

	if err == nil {
		err = fmt.Errorf("")
	}
	if !ok {
		return newError(internalMsg, InternalErrorCode, err.Error(), id)
	}
	return newError(errMsg, errorCode, err.Error(), id)
}

func newError(errMsg string, errorCode int, info string, id any) *structs.Response {
	return NewResponse(id, nil, &structs.Error{
		Code:    errorCode,
		Message: errMsg,
		Data:    info,
	})
}

func errorInternal() json.RawMessage {
	return []byte(`{"jsonrpc":"2.0","error":{"code":-32603,"message":"internal_error"},"id":null}`)
}

func errorInvalidRequest() json.RawMessage {
	return []byte(
		`{"jsonrpc":"2.0","error":{"code":-32600,"message":"invalid_request_not_conforming_to_spec"},"id":null}`)
}
