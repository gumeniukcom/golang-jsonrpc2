package jsonrpcstdio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

// DefaultClientMaxMessageSize bounds a single inbound frame on the client
// unless overridden with WithClientMaxMessageSize.
const DefaultClientMaxMessageSize = 16 << 20 // 16 MiB

// ErrClientClosed is returned by calls made on (or interrupted by) a closed
// client.
var ErrClientClosed = errors.New("jsonrpcstdio: client closed")

// NotificationHandler receives a server-pushed notification: its method and
// raw params (nil when the notification carried none). It runs on the
// client's single read loop, so it must return promptly — offload slow work
// to another goroutine; a blocking handler stalls responses to pending
// calls.
//
// method and params come from the peer and are untrusted: validate them
// before mapping to any privileged local action.
type NotificationHandler func(method string, params json.RawMessage)

// Client is a JSON-RPC 2.0 client over one established byte stream —
// canonically the stdin/stdout pipes of a child server process. It
// implements jsonrpc.Caller: concurrent calls multiplex over the stream and
// responses correlate by id, so out-of-order replies are fine. Server-pushed
// notifications (id-less frames) go to the handler set with
// WithNotificationHandler, or are dropped when none is set. Response frames
// with ids the client never sent are dropped. Safe for concurrent use.
//
// The client does not spawn or manage the peer process; see NewClient.
//
// A peer rejection that carries no id (an id:null top-level error such as a
// parse error) cannot be correlated to one call on a multiplexed stream, so
// it fails every in-flight call — an innocent concurrent call may also
// fail. Keep the client's message limit at or above the peer's responses to
// avoid provoking such rejections.
type Client struct {
	fr       framer
	w        *connWriter // serializes writes and latches on failure/Close
	onNotify NotificationHandler

	mu      sync.Mutex
	pending map[string]chan *structs.Response
	seq     int64
	cause   error // first fatal stream error, wrapped into ErrClientClosed

	closeOnce sync.Once
	closed    chan struct{}
}

type clientConfig struct {
	maxMessageSize int64
	onNotify       NotificationHandler
}

// ClientOption configures NewClient.
type ClientOption func(*clientConfig)

// WithNotificationHandler registers a handler for server-pushed
// notifications. Without it such frames are dropped.
func WithNotificationHandler(h NotificationHandler) ClientOption {
	return func(c *clientConfig) { c.onNotify = h }
}

// WithClientMaxMessageSize caps a single inbound frame; a larger frame is a
// fatal stream error that closes the client. Non-positive values keep
// DefaultClientMaxMessageSize.
func WithClientMaxMessageSize(n int64) ClientOption {
	return func(c *clientConfig) {
		if n > 0 {
			c.maxMessageSize = n
		}
	}
}

// NewClient starts a JSON-RPC client over an established byte stream: r is
// the peer's output and w is the peer's input. For a child server process:
//
//	cmd := exec.Command("path/to/server")
//	stdin, _ := cmd.StdinPipe()   // → w
//	stdout, _ := cmd.StdoutPipe() // → r
//	// wire cmd.Stderr somewhere visible; start the process, then:
//	c, err := jsonrpcstdio.NewClient(jsonrpcstdio.FramingNDJSON, stdout, stdin)
//
// There is no ctx parameter because there is no handshake to bound: the
// streams already exist. NewClient starts the read loop and returns.
//
// The client does not spawn or manage the peer process — spawning, stderr
// plumbing, and the shutdown ladder (close stdin, wait, then SIGTERM/SIGKILL
// per the MCP spec) belong to the caller.
func NewClient(framing Framing, r io.Reader, w io.Writer, opts ...ClientOption) (*Client, error) {
	if !framing.valid() {
		return nil, errInvalidFraming(framing)
	}
	if r == nil || w == nil {
		return nil, errors.New("jsonrpcstdio: reader and writer must not be nil")
	}
	cfg := &clientConfig{maxMessageSize: DefaultClientMaxMessageSize}
	for _, opt := range opts {
		opt(cfg)
	}

	c := &Client{
		fr:       newFramer(framing, r, w, cfg.maxMessageSize, "WithClientMaxMessageSize"),
		onNotify: cfg.onNotify,
		pending:  map[string]chan *structs.Response{},
		closed:   make(chan struct{}),
	}
	// Writes share the server's latch-on-failure writer: the framers emit a
	// frame in more than one Write call, so a mid-frame failure permanently
	// desynchronizes the outbound stream — the connection must fail fast,
	// not keep appending frames after an orphaned header.
	c.w = &connWriter{fr: c.fr, fail: func(err error) { c.shutdown(err) }}
	go c.readLoop()
	return c, nil
}

// Close marks the client closed, latches the writer (no frame can be sent
// after Close returns), and fails all pending calls with ErrClientClosed.
// It does NOT close r or w, and it cannot interrupt a read already blocked
// on a quiet stream: the read loop goroutine stops handling frames
// immediately but exits only when its current read returns. To release it
// promptly, close the underlying stream (or terminate the peer process)
// after Close — for a child process, close its stdin and wait; its exit
// EOFs the stdout pipe and ends the loop.
func (c *Client) Close() error {
	c.w.close()
	c.shutdown(nil)
	return nil
}

// shutdown closes the client once, recording the first fatal cause (nil for
// a caller-initiated Close). It never touches the writer mutex, so it is
// safe to call from connWriter.fail while that mutex is held.
func (c *Client) shutdown(cause error) {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.cause = cause
		c.mu.Unlock()
		close(c.closed)
		c.failPending()
	})
}

// closedErr returns ErrClientClosed, enriched with the fatal stream error
// that caused the close (an oversized frame, a mid-frame write failure) so
// the reason is observable instead of every path collapsing into a generic
// "client closed".
func (c *Client) closedErr() error {
	c.mu.Lock()
	cause := c.cause
	c.mu.Unlock()
	if cause != nil {
		return fmt.Errorf("%w: %w", ErrClientClosed, cause)
	}
	return ErrClientClosed
}

// Call sends a request and waits for the matching response. A JSON-RPC
// error response is returned as *structs.Error (match with errors.As).
//
// ctx bounds only the wait for the response: a pipe write cannot carry a
// deadline, and cancellation abandons the wait (a late response frame is
// dropped by the read loop).
func (c *Client) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	select {
	case <-c.closed:
		return nil, c.closedErr()
	default:
	}

	id := c.nextID()
	ch := make(chan *structs.Response, 1)
	c.mu.Lock()
	c.pending[string(id)] = ch
	c.mu.Unlock()

	unregister := func() {
		c.mu.Lock()
		delete(c.pending, string(id))
		c.mu.Unlock()
	}

	if err := c.send(method, params, id); err != nil {
		unregister()
		return nil, err
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		if resp.Result == nil {
			return nil, nil
		}
		return *resp.Result, nil
	case <-ctx.Done():
		unregister()
		return nil, ctx.Err()
	case <-c.closed:
		return nil, c.closedErr()
	}
}

// Notify sends a notification: no id, no response, no server-side error
// reporting. ctx is accepted for jsonrpc.Caller symmetry; a pipe write
// cannot carry a deadline.
func (c *Client) Notify(_ context.Context, method string, params any) error {
	select {
	case <-c.closed:
		return c.closedErr()
	default:
	}
	return c.send(method, params, nil)
}

func (c *Client) nextID() structs.ID {
	c.mu.Lock()
	c.seq++
	id := structs.ID(strconv.AppendInt(nil, c.seq, 10))
	c.mu.Unlock()
	return id
}

func (c *Client) send(method string, params any, id structs.ID) error {
	rawParams, err := jsonrpc.MarshalParams(params)
	if err != nil {
		return fmt.Errorf("jsonrpcstdio: marshal params: %w", err)
	}
	frame, err := structs.Request{
		Version: jsonrpc.Version,
		Method:  method,
		Params:  rawParams,
		ID:      id,
	}.MarshalJSON()
	if err != nil {
		return fmt.Errorf("jsonrpcstdio: marshal request: %w", err)
	}
	if err := c.w.write(frame); err != nil {
		if errors.Is(err, errWriterClosed) {
			return c.closedErr()
		}
		return err
	}
	return nil
}

// readLoop routes incoming frames to pending calls. It exits when the
// stream dies (EOF, read error, or a fatal framing error such as an
// oversized frame), failing everything still pending with the cause, or
// stops handling frames once the client is closed.
func (c *Client) readLoop() {
	for {
		data, err := c.fr.ReadFrame()
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = nil // peer went away cleanly: plain ErrClientClosed
			}
			c.w.close()
			c.shutdown(err)
			return
		}
		select {
		case <-c.closed:
			// Closed while we were reading: stop delivering — a closed
			// client must not keep invoking the notification handler.
			return
		default:
		}
		data = bytes.TrimSpace(data)
		if len(data) == 0 {
			continue
		}
		// An array frame would be a batch response, but this client never
		// sends batches, so its entries cannot correlate to anything — drop.
		if data[0] == '[' {
			continue
		}
		var resp structs.Response
		if err := resp.UnmarshalJSON(data); err != nil {
			continue // not a frame we understand — drop
		}
		// Responses to our calls always carry the id we sent (even a
		// "result":null reply). A frame without an id is a server-pushed
		// notification. Keying off id — not off result/error being nil —
		// avoids misrouting a legitimate null result into the push path.
		if len(resp.ID) == 0 && resp.Error == nil {
			c.handlePush(data)
			continue
		}
		if resp.Error == nil && resp.Result == nil && len(resp.ID) != 0 && c.isRequestFrame(data) {
			// A server-initiated REQUEST (id + method) decodes as a
			// response with neither result nor error; without this probe an
			// id collision with one of our in-flight calls would deliver a
			// fabricated empty success to it. Reverse calls are not
			// supported in v1 — drop the frame. Probing costs a decode only
			// on this rare shape: real responses carry result or error
			// ("result":null is probed too, then delivered normally).
			continue
		}
		if !c.deliver(&resp) && resp.Error != nil && isNullID(resp.ID) {
			// A top-level error with id:null rejects a whole sent frame
			// (parse error, request too large) and cannot be correlated to a
			// specific call. Rather than let the waiting call hang forever,
			// fail every pending call on the stream.
			c.failPendingWith(resp.Error)
		}
	}
}

// isRequestFrame reports whether the frame carries a method member, i.e. is
// a request/notification rather than a response.
func (c *Client) isRequestFrame(data []byte) bool {
	var probe structs.Request
	return probe.UnmarshalJSON(data) == nil && probe.Method != ""
}

func isNullID(id structs.ID) bool {
	return len(id) == 0 || string(id) == "null"
}

// handlePush routes a server-initiated notification to the registered
// handler. The frame is re-decoded as a request to recover method/params;
// this runs only for id-less/result-less frames, not the common reply path.
func (c *Client) handlePush(data []byte) {
	if c.onNotify == nil {
		return // no handler: drop
	}
	var req structs.Request
	if err := req.UnmarshalJSON(data); err != nil || req.Method == "" {
		return // not a notification we understand — drop
	}
	c.onNotify(req.Method, req.Params)
}

// deliver routes resp to the waiting call by id and reports whether a waiter
// was found.
func (c *Client) deliver(resp *structs.Response) bool {
	if len(resp.ID) == 0 {
		return false // response without an id: uncorrelatable
	}
	c.mu.Lock()
	ch, ok := c.pending[string(resp.ID)]
	if ok {
		delete(c.pending, string(resp.ID))
	}
	c.mu.Unlock()
	if ok {
		ch <- resp // buffered: never blocks
	}
	return ok
}

// failPendingWith delivers err to every pending call, so they return an
// error instead of hanging. Used for a top-level frame rejection that cannot
// be correlated to one call.
func (c *Client) failPendingWith(err *structs.Error) {
	c.mu.Lock()
	pend := c.pending
	c.pending = map[string]chan *structs.Response{}
	c.mu.Unlock()
	resp := &structs.Response{Error: err}
	for _, ch := range pend {
		ch <- resp // each channel is cap 1 with one waiter: never blocks
	}
}

func (c *Client) failPending() {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Waiters learn about the close via c.closed; dropping the channels here
	// just makes the map collectable.
	c.pending = map[string]chan *structs.Response{}
}

// interface guard
var _ jsonrpc.Caller = (*Client)(nil)
