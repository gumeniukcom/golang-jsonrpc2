package jsonrpc

const (
	// Version const for define version of protocol
	// @see https://www.jsonrpc.org/specification#request_object
	Version = "2.0"
)

// @see http://xmlrpc-epi.sourceforge.net/specs/rfc.fault_codes.php
const (
	// ParseErrorCode : parse error. not well formed
	ParseErrorCode = -32700

	// InvalidRequestErrorCode : Invalid Request
	InvalidRequestErrorCode = -32600

	// MethodNotFoundErrorCode : requested method not found
	MethodNotFoundErrorCode = -32601

	// InvalidParamsErrorCode : invalid method parameters
	InvalidParamsErrorCode = -32602

	// InternalErrorCode : Internal error
	InternalErrorCode = -32603

	// OK : everything is ok
	OK = 0

	//MethodNotImplemented : use for develop method
	MethodNotImplemented = -32604

	//RequestTimeLimit : tooooo long
	RequestTimeLimit = -32605
)
