package jsonrpc

const (
	// Version defines the JSON-RPC protocol version.
	// See https://www.jsonrpc.org/specification#request_object
	Version = "2.0"
)

// Error codes per JSON-RPC 2.0 specification.
// See http://xmlrpc-epi.sourceforge.net/specs/rfc.fault_codes.php
const (
	// ParseErrorCode indicates an invalid JSON was received.
	ParseErrorCode = -32700

	// InvalidRequestErrorCode indicates the request is not a valid JSON-RPC object.
	InvalidRequestErrorCode = -32600

	// MethodNotFoundErrorCode indicates the requested method does not exist.
	MethodNotFoundErrorCode = -32601

	// InvalidParamsErrorCode indicates invalid method parameters.
	InvalidParamsErrorCode = -32602

	// InternalErrorCode indicates an internal JSON-RPC error.
	InternalErrorCode = -32603

	// OK indicates the operation completed successfully.
	OK = 0

	// MethodNotImplemented indicates the method is not yet implemented.
	MethodNotImplemented = -32604

	// RequestTimeLimit indicates the request exceeded the time limit.
	RequestTimeLimit = -32605
)
