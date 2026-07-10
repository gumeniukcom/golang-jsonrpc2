// Package jsonrpcstdio adapts a jsonrpc.JSONRPC dispatcher to a byte stream —
// canonically a process's stdin/stdout — which is how Language Server
// Protocol (LSP) servers and Model Context Protocol (MCP) servers speak
// JSON-RPC 2.0. The two ecosystems frame the same protocol differently, so
// the framing is an explicit, mandatory choice:
//
//   - FramingContentLength — the LSP base protocol: each message is preceded
//     by an ASCII header block ("Content-Length: N" terminated by an empty
//     line) and carried as exactly N bytes of JSON.
//   - FramingNDJSON — the MCP stdio transport: one JSON message per
//     newline-terminated line.
//
// There is deliberately no default: the framings are mutually unintelligible
// on the wire, and a silently wrong default would break against a
// spec-compliant peer. The zero Framing value is invalid and rejected.
//
// The canonical server is a single blocking call:
//
//	err := jsonrpcstdio.Serve(ctx, rpc, jsonrpcstdio.FramingContentLength, os.Stdin, os.Stdout)
//
// Serve returns nil when the peer closes stdin (the orderly shutdown signal
// for both protocols), so `if err != nil { os.Exit(1) }` yields conventional
// exit codes for free.
//
// # stdout is the protocol channel
//
// Nothing but JSON-RPC frames may be written to the output stream — both
// specs forbid it, and a single stray log line corrupts the session. All
// process logging must go to stderr. This package does no logging of its
// own; the dispatcher logs through jsonrpc.SetLogger, and slog.Default()
// writes to stderr, so the default configuration is already conformant. Do
// not wire a stdout slog handler in a stdio server.
//
// # Ordering and concurrency
//
// Messages are dispatched strictly sequentially and in order by default
// (DefaultMaxConcurrentCalls = 1) — LSP's ordering rules assume it and MCP
// SDKs default to it. The cost is head-of-line blocking: one slow handler
// delays every request behind it (bounded by the dispatcher's per-request
// timeout). WithMaxConcurrentCalls raises the parallelism for servers whose
// methods are order-independent; responses may then interleave in any order,
// which JSON-RPC permits (correlation is by id).
//
// Note the interaction with cancellation-style notifications: under
// sequential dispatch a notification such as LSP's $/cancelRequest is not
// read from the stream until the request it targets has already finished.
// Servers that need in-flight cancellation must opt into
// WithMaxConcurrentCalls(n > 1) and handle ordering themselves.
//
// # Limits
//
// WithMaxMessageSize (8 MiB default) bounds one inbound frame; violating it
// is a fatal stream error, because an over-limit frame cannot be skipped
// safely (under Content-Length framing the stream cannot be resynchronized
// at all, and silently dropping an NDJSON line would swallow a message the
// peer believes was delivered). For a graceful limit, set the dispatcher's
// jsonrpc.SetMaxMessageSize at or below the transport cap: messages between
// the two limits then draw a proper JSON-RPC error response and the stream
// survives. The transport cap is the hard DoS bound; core's cap is the
// polite one — and note core's is disabled (0) by default, so out of the box
// the transport cap is the only bound. Well-framed garbage (invalid JSON,
// invalid JSON-RPC) is never fatal: core answers -32700/-32600 and the
// stream continues.
//
// WithMaxConcurrentCalls bounds dispatch, not background work: with
// jsonrpc.SetEnforcedTimeout(true) a handler that ignores its context keeps
// running after its slot is released, so a hostile peer can accumulate
// goroutines behind the bound. Keep the default inline timeout mode for
// stdio servers exposed to untrusted peers.
//
// # Push
//
// Handlers can send server-initiated notifications (LSP
// textDocument/publishDiagnostics, MCP resource updates): the transport
// injects a jsonrpc.Pusher into every request context, retrieved with
// jsonrpc.PusherFromContext. The pusher stays valid for the whole
// connection — a handler may hand it to a background goroutine for a
// long-lived subscription. Once Serve returns, Notify fails with an error
// instead of touching the closed stream.
//
// # Deliberately not handled
//
// This is a transport, not a protocol SDK. It does not implement LSP's
// initialize/shutdown/exit state machine or $/cancelRequest bookkeeping,
// MCP's initialize/version negotiation or its removal of batching
// (2025-06-18) — batch frames are handled by the dispatcher regardless, and
// rejecting them is application policy. It does not manage subprocesses on
// the client side, and v1 does not support server-initiated *requests*
// (reverse calls with response correlation, e.g. MCP sampling or LSP
// workspace/configuration): every inbound server-side frame is treated as a
// request or notification, and inbound response-shaped frames draw a -32600
// error reply, not a silent drop.
//
// Also note: method names arriving over the stream are attacker-controlled
// raw strings — bound and sanitize them before using them as metric labels
// or in non-escaping log sinks.
package jsonrpcstdio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
)

// Framing selects the wire framing of the stream. The zero value is invalid:
// Serve and NewClient reject it instead of guessing.
type Framing uint8

const (
	// The zero value is deliberately invalid so it can never be mistaken
	// for a default; Serve and NewClient reject it.
	_ Framing = iota

	// FramingContentLength is the LSP base protocol: an ASCII header block
	// ("Content-Length: N", header lines terminated by \r\n or bare \n, block
	// terminated by an empty line) followed by exactly N bytes of UTF-8 JSON.
	FramingContentLength

	// FramingNDJSON is newline-delimited JSON as used by the MCP stdio
	// transport: one JSON-RPC message per \n-terminated line, no embedded
	// newlines.
	FramingNDJSON
)

// String returns the constant name, for error messages and logs.
func (f Framing) String() string {
	switch f {
	case FramingContentLength:
		return "FramingContentLength"
	case FramingNDJSON:
		return "FramingNDJSON"
	default:
		return fmt.Sprintf("Framing(%d)", uint8(f))
	}
}

// valid reports whether f is a known framing.
func (f Framing) valid() bool {
	return f == FramingContentLength || f == FramingNDJSON
}

// errInvalidFraming is shared by Serve and NewClient.
func errInvalidFraming(f Framing) error {
	return fmt.Errorf("jsonrpcstdio: invalid framing %v: must be FramingContentLength or FramingNDJSON", f)
}

// DefaultMaxMessageSize bounds a single inbound frame (a Content-Length body
// or one NDJSON line) unless overridden with WithMaxMessageSize. It is
// larger than the 1 MiB default of jsonrpchttp/jsonrpcws because on stdio a
// limit violation is fatal to the stream (there is no 413 to answer), and
// real LSP payloads (textDocument/didOpen) routinely carry multi-megabyte
// documents.
const DefaultMaxMessageSize = 8 << 20 // 8 MiB

// DefaultMaxConcurrentCalls is the dispatch parallelism unless overridden
// with WithMaxConcurrentCalls. The default of 1 gives strictly ordered,
// sequential handling — what LSP's ordering rules assume and what MCP SDKs
// default to. The consequence is head-of-line blocking: a slow handler
// delays all later requests on the stream (bounded by the dispatcher's
// per-request timeout); raise WithMaxConcurrentCalls for order-independent
// methods, as jsonrpcws does by default with 16.
const DefaultMaxConcurrentCalls = 1

type server struct {
	maxMessageSize int64
	maxCalls       int
}

// Option configures Serve.
type Option func(*server)

// WithMaxMessageSize caps a single inbound frame in bytes; an oversized
// frame is a fatal stream error (Serve returns). Non-positive values keep
// DefaultMaxMessageSize.
//
// For a graceful degradation band, additionally set the dispatcher's
// jsonrpc.SetMaxMessageSize (an int) at or below this cap: messages between
// the two limits then get a JSON-RPC error response instead of killing the
// stream.
func WithMaxMessageSize(n int64) Option {
	return func(s *server) {
		if n > 0 {
			s.maxMessageSize = n
		}
	}
}

// WithMaxConcurrentCalls sets how many inbound messages may be dispatched
// concurrently. 1 (the default) preserves request order end-to-end; higher
// values dispatch jsonrpcws-style — slot-bounded, with responses free to
// reorder (JSON-RPC correlates by id). Note the slot unit is a message, and
// one message may be a batch of up to the dispatcher's SetMaxBatchSize
// requests. Non-positive values keep the default.
func WithMaxConcurrentCalls(n int) Option {
	return func(s *server) {
		if n > 0 {
			s.maxCalls = n
		}
	}
}

// Serve reads framed JSON-RPC messages from r, dispatches them on rpc, and
// writes responses to w until ctx is canceled, r reaches EOF, or the stream
// fails. It blocks; run it as the main body of a stdio server process:
//
//	err := jsonrpcstdio.Serve(ctx, rpc, jsonrpcstdio.FramingContentLength, os.Stdin, os.Stdout)
//
// Serve returns nil on clean EOF (the peer closed our stdin — the orderly
// shutdown signal of both LSP and MCP), ctx.Err() (unwrapped, matching
// errors.Is) when the context ended first, and a wrapped error on framing,
// read, or write failures. In-flight handlers are always drained before it
// returns, and clean EOF observed before cancellation wins over a
// subsequent cancellation.
//
// Cancellation caveat: a read blocked on a quiet stream cannot be
// interrupted portably, so after canceling ctx, Serve may not return until
// the peer next writes or closes the stream. To force a prompt return, close
// the reader (e.g. os.Stdin.Close()) after canceling ctx. The same applies
// to writes: a peer that stops reading our output while keeping the pipe
// open blocks the response write indefinitely (pipes carry no deadlines),
// and Serve cannot return until that write unblocks — in the subprocess
// model the parent owns both ends, so a dead parent closes them and
// resolves both cases.
func Serve(ctx context.Context, rpc *jsonrpc.JSONRPC, framing Framing, r io.Reader, w io.Writer, opts ...Option) (err error) {
	if rpc == nil {
		return errors.New("jsonrpcstdio: rpc must not be nil")
	}
	if !framing.valid() {
		return errInvalidFraming(framing)
	}
	if r == nil || w == nil {
		return errors.New("jsonrpcstdio: reader and writer must not be nil")
	}

	s := &server{
		maxMessageSize: DefaultMaxMessageSize,
		maxCalls:       DefaultMaxConcurrentCalls,
	}
	for _, opt := range opts {
		opt(s)
	}

	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// connErr records the first connection-fatal error raised outside the
	// read path: a failed response/push write, or a transport-layer panic.
	// errOnce also cancels connCtx so the read loop stops at the next frame
	// boundary.
	var (
		errOnce sync.Once
		connErr error
	)
	fail := func(e error) {
		errOnce.Do(func() {
			connErr = e
			cancel()
		})
	}

	fr := newFramer(framing, r, w, s.maxMessageSize, "WithMaxMessageSize")
	cw := &connWriter{fr: fr, fail: fail}
	pusher := &stdioPusher{w: cw}

	var wg sync.WaitGroup
	slots := make(chan struct{}, s.maxCalls)
	var (
		readErr  error
		eofClean bool // peer closed the stream while ctx was still alive
	)

	// Single exit point: every return path funnels through here so the
	// ordering guarantees hold on all of them — recover transport panics,
	// stop the world, drain in-flight handlers, latch the writer (a late
	// background Notify must get an error, not write to a dead stdout and
	// die on SIGPIPE), then pick the result by precedence.
	defer func() {
		if p := recover(); p != nil {
			fail(fmt.Errorf("jsonrpcstdio: internal panic: %v", p))
		}
		// On orderly shutdown (peer closed the stream) in-flight handlers
		// finish and write their responses; on every other path they are
		// canceled first. (The function-level deferred cancel releases the
		// context either way once we return.)
		if !eofClean {
			cancel()
		}
		wg.Wait()
		cw.close()
		switch {
		case connErr != nil:
			err = connErr
		case eofClean:
			// The peer closed the stream before any cancellation: orderly
			// shutdown wins even if ctx was canceled while draining.
			err = nil
		case ctx.Err() != nil:
			err = ctx.Err()
		case readErr != nil:
			err = readErr
		default:
			err = nil
		}
	}()

	// The pusher rides the request context so handlers can send
	// server-initiated notifications over this stream. Both values are
	// per-connection constants, so the context is built once, not per
	// message.
	reqCtx := jsonrpc.ContextWithPusher(connCtx, pusher)

	dispatch := func(frame []byte) {
		resp := rpc.HandleRPCJSONRawMessage(reqCtx, frame)
		if len(resp) == 0 {
			return // notification (or all-notification batch): write nothing
		}
		_ = cw.write(resp) //nolint:errcheck // a write failure latches the writer and fails the connection via fail()
	}

	for {
		if connCtx.Err() != nil {
			// Canceled between frames — by the caller, or by a write failure
			// in a concurrent dispatch. The deferred exit picks the reason.
			return nil
		}
		frame, ferr := fr.ReadFrame()
		if ferr != nil {
			switch {
			case !errors.Is(ferr, io.EOF):
				readErr = ferr
			case ctx.Err() == nil:
				// Peer-initiated shutdown. When ctx was canceled first, the
				// EOF is just the escape hatch (caller closed the reader to
				// unblock us) and cancellation stays the reported reason.
				eofClean = true
			}
			return nil // the deferred exit decides
		}

		if s.maxCalls == 1 {
			// Inline dispatch: zero per-message goroutines and guaranteed
			// ordering. A write failure is observed on the next loop
			// iteration, before the read would block again.
			dispatch(frame)
			continue
		}

		select {
		case slots <- struct{}{}:
		case <-connCtx.Done():
			return nil
		}
		wg.Add(1)
		go func(frame []byte) {
			// One deferred cleanup so the order cannot be broken by a
			// future insertion: recover first (it must observe the panic),
			// then release the slot, then mark the message done.
			defer func() {
				if p := recover(); p != nil {
					fail(fmt.Errorf("jsonrpcstdio: internal panic: %v", p))
				}
				<-slots
				wg.Done()
			}()
			dispatch(frame)
		}(frame)
	}
}
