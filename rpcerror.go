package jsonrpc

import "fmt"

// RPCError is an application-level error that method handlers can return
// (directly or wrapped via fmt.Errorf with %w) to control the JSON-RPC error
// response.
//
// When a returned error chain contains an RPCError, its Code is authoritative:
// it overrides the int code returned through the RPCMethod signature. Data,
// when set, is serialized into the response's error.data field and therefore
// is SENT TO THE CLIENT — put only client-safe detail in it (validation info,
// limits), never internal error text. The wrapped Err stays server-side: it is
// logged but never serialized into the response.
//
// If Code is not registered via RegisterError, the response degrades to
// internal_error WITHOUT Data, and the unregistered code is logged.
type RPCError struct {
	Code int
	Data any
	Err  error
}

// NewRPCError creates an RPCError with the given registered error code,
// wrapping err for server-side logging.
func NewRPCError(code int, err error) *RPCError {
	return &RPCError{Code: code, Err: err}
}

// WithData returns a copy of e with client-visible data attached. It does not
// mutate the receiver, so package-level sentinel errors can be shared safely
// across concurrent requests:
//
//	var ErrCropLimit = jsonrpc.NewRPCError(4001, nil)
//	// per request:
//	return ErrCropLimit.WithData(map[string]any{"limit": 5})
//
// The data value must be JSON-marshalable; avoid typed-nil pointers (they
// serialize as "data":null instead of omitting the field).
func (e *RPCError) WithData(data any) *RPCError {
	c := *e
	c.Data = data
	return &c
}

// Error implements the error interface. It is nil-receiver-safe: a typed-nil
// *RPCError (the classic `var e *RPCError; return e` mistake) formats instead
// of panicking in logging or fmt paths.
func (e *RPCError) Error() string {
	if e == nil {
		return "jsonrpc error <nil>"
	}
	if e.Err != nil {
		return fmt.Sprintf("jsonrpc error %d: %s", e.Code, e.Err.Error())
	}
	return fmt.Sprintf("jsonrpc error %d", e.Code)
}

// Unwrap returns the wrapped error for errors.Is/As chains.
func (e *RPCError) Unwrap() error {
	return e.Err
}
