package jsonrpcws

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/coder/websocket"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

// DefaultClientMaxMessageSize bounds a single incoming response frame unless
// overridden with WithClientMaxMessageSize.
const DefaultClientMaxMessageSize = 16 << 20 // 16 MiB

// ErrClientClosed is returned by calls made on (or interrupted by) a closed
// client.
var ErrClientClosed = errors.New("jsonrpcws: client closed")

// ErrBatchTooLarge is returned by CallBatch when the batch exceeds the
// client's configured limit (WithClientMaxBatchSize).
var ErrBatchTooLarge = errors.New("jsonrpcws: batch exceeds client max batch size")

// NotificationHandler receives a server-pushed notification: its method and
// raw params (nil when the notification carried none). It runs on the
// client's single read loop, so it must return promptly — offload slow work
// to another goroutine; a blocking handler stalls responses to pending calls.
//
// method and params come from the server and are untrusted: validate them
// before mapping to any privileged local action.
type NotificationHandler func(method string, params json.RawMessage)

// Client is a JSON-RPC 2.0 client over one WebSocket connection. It
// implements jsonrpc.Caller and jsonrpc.BatchCaller: concurrent calls
// multiplex over the connection and responses correlate by id, so
// out-of-order replies are fine. Server-pushed notifications (id-less frames)
// go to the handler set with WithNotificationHandler, or are dropped when
// none is set. Response frames with ids the client never sent are dropped.
// Safe for concurrent use.
//
// A server rejection that carries no id (an id:null top-level error such as
// batch_too_large or request_too_large) cannot be correlated to one call on a
// multiplexed connection, so it fails every in-flight call — an innocent
// concurrent call may also fail. Keep the client's batch/message limits at or
// below the server's to avoid provoking such rejections.
type Client struct {
	conn         *websocket.Conn
	writeMu      sync.Mutex
	onNotify     NotificationHandler
	maxBatchSize int

	mu      sync.Mutex
	pending map[string]chan *structs.Response
	seq     int64

	closeOnce sync.Once
	closed    chan struct{}
}

// DialOption configures DialClient.
type DialOption func(*dialConfig)

type dialConfig struct {
	maxMessageSize int64
	maxBatchSize   int
	header         map[string][]string
	onNotify       NotificationHandler
}

// WithNotificationHandler registers a handler for server-pushed
// notifications. Without it such frames are dropped.
func WithNotificationHandler(h NotificationHandler) DialOption {
	return func(c *dialConfig) { c.onNotify = h }
}

// WithClientMaxBatchSize caps how many entries CallBatch will send; a larger
// batch fails locally with ErrBatchTooLarge instead of reaching the server.
// It defaults to jsonrpc.DefaultMaxBatchSize (the server's default cap), so
// oversized batches are rejected before the server would answer with an
// unaddressable id:null error. Zero disables the client-side check.
func WithClientMaxBatchSize(n int) DialOption {
	return func(c *dialConfig) { c.maxBatchSize = n }
}

// WithClientMaxMessageSize caps a single incoming response frame; larger
// frames close the connection with status 1009. Non-positive values keep
// DefaultClientMaxMessageSize.
func WithClientMaxMessageSize(n int64) DialOption {
	return func(c *dialConfig) {
		if n > 0 {
			c.maxMessageSize = n
		}
	}
}

// WithDialHeader adds HTTP headers to the handshake request (e.g.
// Authorization).
func WithDialHeader(key string, values ...string) DialOption {
	return func(c *dialConfig) {
		c.header[key] = append(c.header[key], values...)
	}
}

// DialClient connects to a JSON-RPC WebSocket endpoint (ws:// or wss://).
// ctx bounds the handshake only; the connection itself lives until Close.
func DialClient(ctx context.Context, url string, opts ...DialOption) (*Client, error) {
	cfg := &dialConfig{
		maxMessageSize: DefaultClientMaxMessageSize,
		maxBatchSize:   jsonrpc.DefaultMaxBatchSize,
		header:         map[string][]string{},
	}
	for _, opt := range opts {
		opt(cfg)
	}

	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: cfg.header})
	if err != nil {
		return nil, fmt.Errorf("jsonrpcws: dial: %w", err)
	}
	conn.SetReadLimit(cfg.maxMessageSize)

	c := &Client{
		conn:         conn,
		onNotify:     cfg.onNotify,
		maxBatchSize: cfg.maxBatchSize,
		pending:      map[string]chan *structs.Response{},
		closed:       make(chan struct{}),
	}
	go c.readLoop()
	return c, nil
}

// Close tears the connection down and fails all pending calls with
// ErrClientClosed.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
		_ = c.conn.Close(websocket.StatusNormalClosure, "")
		c.failPending()
	})
	return nil
}

// Call sends a request and waits for the matching response. A JSON-RPC
// error response is returned as *structs.Error; ctx cancellation abandons
// the wait (a late response frame is dropped by the read loop).
func (c *Client) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	select {
	case <-c.closed:
		return nil, ErrClientClosed
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

	if err := c.send(ctx, method, params, id); err != nil {
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
		return nil, ErrClientClosed
	}
}

func (c *Client) nextID() structs.ID {
	c.mu.Lock()
	c.seq++
	id := structs.ID(strconv.AppendInt(nil, c.seq, 10))
	c.mu.Unlock()
	return id
}

// CallBatch sends specs as one JSON-RPC batch over the connection and waits
// for the responses, which correlate by id (and share the pending map with
// concurrent single Calls). Results align by index with specs; notification
// specs get the zero BatchResult. An empty batch makes no network call.
func (c *Client) CallBatch(ctx context.Context, specs []jsonrpc.Spec) ([]jsonrpc.BatchResult, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	if c.maxBatchSize > 0 && len(specs) > c.maxBatchSize {
		// Reject locally: an oversized batch would draw a top-level id:null
		// error from the server, which cannot be correlated back to this call.
		return nil, ErrBatchTooLarge
	}
	select {
	case <-c.closed:
		return nil, ErrClientClosed
	default:
	}

	frame, ids, err := jsonrpc.MarshalBatch(specs, c.nextID)
	if err != nil {
		return nil, err
	}

	chans := make(map[string]chan *structs.Response, len(ids))
	c.mu.Lock()
	for _, id := range ids {
		if id == nil {
			continue
		}
		ch := make(chan *structs.Response, 1)
		c.pending[string(id)] = ch
		chans[string(id)] = ch
	}
	c.mu.Unlock()

	unregister := func() {
		c.mu.Lock()
		for k := range chans {
			delete(c.pending, k)
		}
		c.mu.Unlock()
	}

	if err := c.sendFrame(ctx, frame); err != nil {
		unregister()
		return nil, err
	}

	results := make([]jsonrpc.BatchResult, len(specs))
	for i, id := range ids {
		if id == nil {
			continue // notification: no response slot
		}
		select {
		case resp := <-chans[string(id)]:
			results[i] = jsonrpc.BatchResultFromResponse(resp)
		case <-ctx.Done():
			unregister()
			return nil, ctx.Err()
		case <-c.closed:
			return nil, ErrClientClosed
		}
	}
	return results, nil
}

// Notify sends a notification: no id, no response, no server-side error
// reporting.
func (c *Client) Notify(ctx context.Context, method string, params any) error {
	select {
	case <-c.closed:
		return ErrClientClosed
	default:
	}
	return c.send(ctx, method, params, nil)
}

func (c *Client) send(ctx context.Context, method string, params any, id structs.ID) error {
	rawParams, err := jsonrpc.MarshalParams(params)
	if err != nil {
		return err
	}
	frame, err := structs.Request{
		Version: jsonrpc.Version,
		Method:  method,
		Params:  rawParams,
		ID:      id,
	}.MarshalJSON()
	if err != nil {
		return fmt.Errorf("jsonrpcws: marshal request: %w", err)
	}
	return c.sendFrame(ctx, frame)
}

// sendFrame writes a pre-marshaled frame under the write mutex.
func (c *Client) sendFrame(ctx context.Context, frame []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.conn.Write(ctx, websocket.MessageText, frame); err != nil {
		return fmt.Errorf("jsonrpcws: write: %w", err)
	}
	return nil
}

// readLoop routes incoming frames to pending calls. It exits when the
// connection dies, failing everything still pending.
func (c *Client) readLoop() {
	defer func() { _ = c.Close() }()
	for {
		// The connection outlives any single call, so reads run under the
		// background context; Close unblocks them by closing the socket.
		_, data, err := c.conn.Read(context.Background())
		if err != nil {
			return
		}
		data = bytes.TrimSpace(data)
		if len(data) == 0 {
			continue
		}
		// A batch response is an array frame: stream it element by element and
		// deliver each by id, reusing the per-id pending map. Streaming (one
		// structs.Response at a time) instead of unmarshaling into a []Response
		// avoids the ~20x memory amplification a hostile array frame of empty
		// objects would otherwise cause; the loop also stops once it has
		// delivered as many entries as there are outstanding calls, since a
		// well-behaved server never returns more responses than we sent.
		if data[0] == '[' {
			c.deliverBatch(data)
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
		if !c.deliver(&resp) && resp.Error != nil && isNullID(resp.ID) {
			// A top-level error with id:null rejects a whole sent frame
			// (batch_too_large, request_too_large, parse error, invalid
			// request) and cannot be correlated to a specific call, since it
			// carries no usable id. Rather than let the waiting call hang
			// forever, fail every pending call on the connection. This blast
			// radius is unavoidable over a multiplexed connection — the
			// rejection could belong to any in-flight frame — so an innocent
			// concurrent call may also fail. WithClientMaxBatchSize prevents
			// the common batch trigger before it reaches the server.
			c.failPendingWith(resp.Error)
		}
	}
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

// deliverBatch streams a batch-response array frame, delivering each entry by
// id. It processes at most as many entries as there are outstanding pending
// calls, so a hostile array frame cannot make the read loop spin over
// millions of bogus entries.
func (c *Client) deliverBatch(data []byte) {
	c.mu.Lock()
	limit := len(c.pending)
	c.mu.Unlock()

	// json/v2 migration point: streaming array decode. jsontext.Decoder
	// replaces this when json/v2 lands (see MIGRATION.md); kept on
	// encoding/json until then.
	dec := json.NewDecoder(bytes.NewReader(data))
	if _, err := dec.Token(); err != nil { // consume '['
		return
	}
	for n := 0; dec.More() && n < limit; n++ {
		var resp structs.Response
		if err := dec.Decode(&resp); err != nil {
			return // malformed entry — stop
		}
		c.deliver(&resp)
	}
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

// failPendingWith delivers err to every pending call, so they return an error
// instead of hanging. Used for a top-level frame rejection that cannot be
// correlated to one call.
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

// interface guards
var (
	_ jsonrpc.Caller      = (*Client)(nil)
	_ jsonrpc.BatchCaller = (*Client)(nil)
)
