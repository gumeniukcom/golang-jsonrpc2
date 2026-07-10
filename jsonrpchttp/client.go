package jsonrpchttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync/atomic"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
	"github.com/gumeniukcom/golang-jsonrpc2/v2/structs"
)

// DefaultMaxResponseSize bounds how many response-body bytes the Client
// reads into memory unless overridden with WithMaxResponseSize.
const DefaultMaxResponseSize = 16 << 20 // 16 MiB

// ErrBatchTooLarge is returned by CallBatch when the batch exceeds the
// client's configured limit (WithMaxBatchSize).
var ErrBatchTooLarge = errors.New("jsonrpchttp: batch exceeds client max batch size")

// Client issues JSON-RPC 2.0 calls over HTTP POST. It implements
// jsonrpc.Caller; ids are a per-client integer sequence. Safe for
// concurrent use.
type Client struct {
	url             string
	hc              *http.Client
	maxResponseSize int64
	maxBatchSize    int
	seq             atomic.Int64
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithMaxBatchSize caps how many entries CallBatch will send; a larger batch
// fails locally with ErrBatchTooLarge. Defaults to jsonrpc.DefaultMaxBatchSize
// (the server's default cap). Zero disables the client-side check.
func WithMaxBatchSize(n int) ClientOption {
	return func(c *Client) { c.maxBatchSize = n }
}

// WithHTTPClient substitutes the http.Client used for requests (custom
// transport, timeouts, proxies, instrumentation).
func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *Client) {
		if hc != nil {
			c.hc = hc
		}
	}
}

// WithMaxResponseSize caps how many response-body bytes are read into
// memory. Non-positive values keep the default.
func WithMaxResponseSize(n int64) ClientOption {
	return func(c *Client) {
		if n > 0 {
			c.maxResponseSize = n
		}
	}
}

// NewClient creates a Client for the given endpoint URL.
func NewClient(url string, opts ...ClientOption) *Client {
	c := &Client{
		url:             url,
		hc:              http.DefaultClient,
		maxResponseSize: DefaultMaxResponseSize,
		maxBatchSize:    jsonrpc.DefaultMaxBatchSize,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Call sends a request and returns the raw result. A JSON-RPC error
// response is returned as *structs.Error; transport-level failures
// (non-2xx, unreadable body) are ordinary errors.
func (c *Client) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.nextID()
	body, err := c.post(ctx, method, params, id)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("jsonrpchttp: empty response to a call (id %s)", id)
	}

	var resp structs.Response
	if err := resp.UnmarshalJSON(body); err != nil {
		return nil, fmt.Errorf("jsonrpchttp: decode response: %w", err)
	}
	if resp.Error != nil {
		return nil, resp.Error
	}
	if len(resp.ID) != 0 && string(resp.ID) != string(id) {
		return nil, fmt.Errorf("jsonrpchttp: response id %s does not match request id %s", resp.ID, id)
	}
	if resp.Result == nil {
		return nil, nil
	}
	return *resp.Result, nil
}

// Notify sends a notification: no id, and no error reporting from the
// server — any 2xx status is success.
func (c *Client) Notify(ctx context.Context, method string, params any) error {
	_, err := c.post(ctx, method, params, nil)
	return err
}

func (c *Client) nextID() structs.ID {
	return structs.ID(strconv.AppendInt(nil, c.seq.Add(1), 10))
}

// CallBatch sends specs as one JSON-RPC batch (a single POST) and returns
// results aligned by index with specs; notification specs get the zero
// BatchResult. An empty batch makes no request.
func (c *Client) CallBatch(ctx context.Context, specs []jsonrpc.Spec) ([]jsonrpc.BatchResult, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	if c.maxBatchSize > 0 && len(specs) > c.maxBatchSize {
		return nil, ErrBatchTooLarge
	}
	frame, ids, err := jsonrpc.MarshalBatch(specs, c.nextID)
	if err != nil {
		return nil, err
	}

	body, err := c.postRaw(ctx, frame)
	if err != nil {
		return nil, err
	}

	results := make([]jsonrpc.BatchResult, len(specs))
	// expected maps each awaited id to its spec index; only responses whose id
	// we actually sent are kept, so a hostile server that returns millions of
	// bogus entries cannot inflate memory beyond len(specs).
	expected := make(map[string]int, len(specs))
	for i, id := range ids {
		if id != nil {
			expected[string(id)] = i
		}
	}
	if len(expected) == 0 {
		return results, nil // all notifications: server sends no response
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("jsonrpchttp: empty response to a batch with calls")
	}

	// Stream the array (one structs.Response at a time) rather than
	// unmarshaling the whole []Response; stop once every awaited id is seen
	// or once we have scanned as many entries as we sent — a well-behaved
	// server returns exactly one response per call, so a longer array is a
	// hostile server padding the frame, not worth scanning.
	// json/v2 migration point: streaming array decode. jsontext.Decoder
	// replaces this when json/v2 lands (see docs/dev/json-v2-plan.md); kept on
	// encoding/json until then.
	dec := json.NewDecoder(bytes.NewReader(body))
	tok, err := dec.Token() // expect '['
	if err != nil {
		return nil, fmt.Errorf("jsonrpchttp: decode batch response: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		// A batch of calls must draw an array; a top-level object here is a
		// server-side whole-frame rejection (e.g. batch_too_large).
		return nil, fmt.Errorf("jsonrpchttp: batch response is not an array (server rejected the batch?)")
	}
	for scanned := 0; dec.More() && len(expected) > 0 && scanned < len(ids); scanned++ {
		var resp structs.Response
		if err := dec.Decode(&resp); err != nil {
			return nil, fmt.Errorf("jsonrpchttp: decode batch response: %w", err)
		}
		if i, ok := expected[string(resp.ID)]; ok {
			results[i] = jsonrpc.BatchResultFromResponse(&resp)
			delete(expected, string(resp.ID))
		}
	}
	for id, i := range expected {
		results[i] = jsonrpc.BatchResult{Error: &structs.Error{
			Code: jsonrpc.InternalErrorCode, Message: "no response for batch id " + id}}
	}
	return results, nil
}

func (c *Client) post(ctx context.Context, method string, params any, id structs.ID) ([]byte, error) {
	rawParams, err := jsonrpc.MarshalParams(params)
	if err != nil {
		return nil, err
	}
	reqBody, err := structs.Request{
		Version: jsonrpc.Version,
		Method:  method,
		Params:  rawParams,
		ID:      id,
	}.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("jsonrpchttp: marshal request: %w", err)
	}
	return c.postRaw(ctx, reqBody)
}

func (c *Client) postRaw(ctx context.Context, reqBody []byte) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("jsonrpchttp: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.hc.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("jsonrpchttp: do request: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(httpResp.Body, c.maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("jsonrpchttp: read response: %w", err)
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode > 299 {
		return nil, fmt.Errorf("jsonrpchttp: unexpected HTTP status %d", httpResp.StatusCode)
	}
	return body, nil
}

// interface guards
var (
	_ jsonrpc.Caller      = (*Client)(nil)
	_ jsonrpc.BatchCaller = (*Client)(nil)
)
