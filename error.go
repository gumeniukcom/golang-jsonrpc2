package jsonrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

// ErrorMessages maps error codes to human-readable messages.
type ErrorMessages map[int]string

// RegisterError registers a custom error code and message.
// Codes in the range -32768..-32000 are reserved by the JSON-RPC spec.
// See http://xmlrpc-epi.sourceforge.net/specs/rfc.fault_codes.php
func (j *JSONRPC) RegisterError(code int, msg string) error {
	return j.updateConfig(func(c *config) error {
		if _, ok := c.errors[code]; ok {
			return fmt.Errorf("error with code %d already exists", code)
		}
		if code >= -32768 && code <= -32000 {
			return fmt.Errorf("error with code %d: range -32768..-32000 reserved", code)
		}
		c.errors[code] = msg
		return nil
	})
}

// Error builds a JSON-RPC error response for the given error code and request ID.
//
// The error text is never serialized into the response: internal detail
// (driver errors, wrapped messages, panic values) stays server-side and is
// written to the configured logger instead. The only client-visible detail is
// the Data field of a *RPCError in the error chain, whose Code is
// authoritative and overrides errorCode. Unregistered codes degrade to
// internal_error without data; the original code is logged.
func (j *JSONRPC) Error(
	ctx context.Context,
	err error,
	errorCode int,
	id any,
) *structs.Response {
	return j.errorResponse(ctx, j.cfg.Load(), err, errorCode, id)
}

// errorResponse is the hot-path variant of Error operating on an already
// loaded config snapshot.
func (j *JSONRPC) errorResponse(
	ctx context.Context,
	cfg *config,
	err error,
	errorCode int,
	id any,
) *structs.Response {
	var data any
	var rpcErr *RPCError
	if errors.As(err, &rpcErr) && rpcErr != nil {
		errorCode = rpcErr.Code
		data = rpcErr.Data
	}

	msg, ok := cfg.errors[errorCode]
	internalMsg := cfg.errors[InternalErrorCode]
	logger := cfg.logger

	code := errorCode
	if !ok {
		if logger != nil {
			logger.WarnContext(ctx, "jsonrpc: unregistered error code",
				slog.Int("code", errorCode))
		}
		code = InternalErrorCode
		msg = internalMsg
		data = nil
	}

	if err != nil && logger != nil {
		logger.Log(ctx, levelForCode(code), "jsonrpc: error response",
			slog.Int("code", code),
			slog.Any("error", err),
		)
	}

	return newError(msg, code, data, id)
}

// levelForCode picks the server-side log level for an error response:
// genuine server faults log at Error, timeouts at Warn, and client-caused or
// expected application errors at Debug so a flood of bad requests cannot spam
// the log at Error level.
func levelForCode(code int) slog.Level {
	switch code {
	case InternalErrorCode:
		return slog.LevelError
	case RequestTimeLimit:
		return slog.LevelWarn
	default:
		return slog.LevelDebug
	}
}

func newError(errMsg string, errorCode int, data any, id any) *structs.Response {
	return NewResponse(id, nil, &structs.Error{
		Code:    errorCode,
		Message: errMsg,
		Data:    data,
	})
}

// Pre-serialized error responses for reject paths that must stay cheap.
// The returned slices are shared package state: callers (and transports)
// must treat them as read-only.
var (
	respInternal        = json.RawMessage(`{"jsonrpc":"2.0","error":{"code":-32603,"message":"internal_error"},"id":null}`)
	respInvalidRequest  = json.RawMessage(`{"jsonrpc":"2.0","error":{"code":-32600,"message":"invalid_request_not_conforming_to_spec"},"id":null}`)
	respBatchTooLarge   = json.RawMessage(`{"jsonrpc":"2.0","error":{"code":-32600,"message":"batch_too_large"},"id":null}`)
	respRequestTooLarge = json.RawMessage(`{"jsonrpc":"2.0","error":{"code":-32600,"message":"request_too_large"},"id":null}`)
	respParseError      = json.RawMessage(`{"jsonrpc":"2.0","error":{"code":-32700,"message":"parse_error_not_well_formed"},"id":null}`)
)

func errorInternal() json.RawMessage { return respInternal }

func errorInvalidRequest() json.RawMessage { return respInvalidRequest }

func errorBatchTooLarge() json.RawMessage { return respBatchTooLarge }

func errorRequestTooLarge() json.RawMessage { return respRequestTooLarge }

func errorParse() json.RawMessage { return respParseError }

// errorForMalformed classifies undecodable input per the JSON-RPC 2.0 spec:
// syntactically broken JSON is a ParseError (-32700), while valid JSON that
// is not a request object is an InvalidRequest (-32600). json.Valid runs
// only on this error path.
func errorForMalformed(data []byte) json.RawMessage {
	if json.Valid(data) {
		return errorInvalidRequest()
	}
	return errorParse()
}
