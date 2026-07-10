// Package jsonrpchttp adapts a jsonrpc.JSONRPC dispatcher to net/http.
//
// Transport-level failures map to HTTP status codes (405, 415, 413, 400);
// everything that parses as a JSON-RPC message — including malformed JSON,
// which yields a -32700 response object — is answered with HTTP 200 and a
// JSON-RPC body, and notifications yield 204 No Content, per common JSON-RPC
// over HTTP conventions.
//
// The handler bounds the request body (WithMaxBodySize, 1 MiB by default),
// but run it behind an http.Server with ReadHeaderTimeout/ReadTimeout set —
// slow-client protection belongs to the server, not the handler.
//
// The handler does not implement authentication, CORS, or CSRF protection —
// those are application policy; wrap it in your own middleware. Note that a
// missing Content-Type header is tolerated, so with cookie-based auth a
// cross-site request can reach the handler without a CORS preflight: either
// use token-based auth or add CSRF protection in front.
package jsonrpchttp

import (
	"errors"
	"io"
	"mime"
	"net/http"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
)

// DefaultMaxBodySize bounds the request body a Handler reads into memory
// unless overridden with WithMaxBodySize.
const DefaultMaxBodySize = 1 << 20 // 1 MiB

type handler struct {
	rpc         *jsonrpc.JSONRPC
	maxBodySize int64
}

// Option configures the Handler.
type Option func(*handler)

// WithMaxBodySize caps how many request-body bytes are read into memory;
// larger bodies are rejected with 413 before dispatch. Zero or negative
// disables the cap (bound it elsewhere — an unbounded body is a DoS vector).
func WithMaxBodySize(n int64) Option {
	return func(h *handler) { h.maxBodySize = n }
}

// Handler wraps the dispatcher into an http.Handler serving JSON-RPC 2.0
// over POST.
func Handler(rpc *jsonrpc.JSONRPC, opts ...Option) http.Handler {
	h := &handler{rpc: rpc, maxBodySize: DefaultMaxBodySize}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "JSON-RPC requests must be POSTed", http.StatusMethodNotAllowed)
		return
	}

	// An absent Content-Type is tolerated; a present one must be JSON.
	if ct := r.Header.Get("Content-Type"); ct != "" {
		mediaType, _, err := mime.ParseMediaType(ct)
		if err != nil || mediaType != "application/json" {
			http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
			return
		}
	}

	body := r.Body
	if h.maxBodySize > 0 {
		body = http.MaxBytesReader(w, r.Body, h.maxBodySize)
	}
	defer func() { _ = body.Close() }()

	data, err := io.ReadAll(body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "cannot read request body", http.StatusBadRequest)
		}
		return
	}

	resp := h.rpc.HandleRPCJSONRawMessage(r.Context(), data)
	if len(resp) == 0 {
		// Notification (or all-notification batch): the server MUST NOT
		// reply with a JSON-RPC message.
		w.WriteHeader(http.StatusNoContent)
		return
	}

	hdr := w.Header()
	hdr.Set("Content-Type", "application/json")
	// RPC responses are per-request: keep misbehaving intermediaries from
	// caching them, and disable content sniffing like http.Error does.
	hdr.Set("Cache-Control", "no-store")
	hdr.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(resp)
}
