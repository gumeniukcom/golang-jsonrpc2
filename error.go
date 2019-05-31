package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gumeniukcom/golang-jsonrpc2/structs"
)

// ErrorMessages map of errors
type ErrorMessages map[int]string

//RegisterError register new error
// @see http://xmlrpc-epi.sourceforge.net/specs/rfc.fault_codes.php
func (j *JSONRPC) RegisterError(code int, msg string) error {
	if _, ok := j.errors[code]; ok {
		return fmt.Errorf("error with code %d exist", code)
	}
	if code >= -32768 && code <= -32000 {
		return fmt.Errorf("error with code %d : range -32768..-32000 reserved", code)
	}
	j.errors[code] = msg
	return nil
}

// NewError is method for create response with error code
func (j *JSONRPC) NewError(
	ctx context.Context,
	err error,
	errorCode int,
	id interface{},
) *structs.Response {
	errMsg, ok := j.errors[errorCode]
	if err == nil {
		err = fmt.Errorf("")
	}
	if !ok {
		return newError(j.errors[InternalErrorCode], InternalErrorCode, err.Error(), id)
	}
	return newError(errMsg, errorCode, err.Error(), id)
}

func newError(errMsg string, errorCode int, info string, id interface{}) *structs.Response {
	return newResponse(id, nil, &structs.Error{
		Code:    errorCode,
		Message: errMsg,
		Data:    info,
	})
}

func errorInternal() json.RawMessage {
	return []byte("{\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32603,\"message\":\"internal_error\"}, \"id\":1}")
}

func errorInvalidRequest() json.RawMessage {
	return []byte(
		"{\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32600,\"message\":\"invalid_request_not_conforming_to_spec\"}, \"id\":1}")
}
