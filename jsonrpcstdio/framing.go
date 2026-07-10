package jsonrpcstdio

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
)

// maxHeaderBytes caps one Content-Length header block. Real blocks are under
// a hundred bytes; the cap only exists to bound a hostile peer.
const maxHeaderBytes = 8 << 10

// readerBufferSize is the shared bufio.Reader buffer. Header lines and
// typical NDJSON lines fit in one fill; larger frames are accumulated.
const readerBufferSize = 32 << 10

// errUnframeable marks a WriteFrame rejection that happened BEFORE any byte
// reached the stream: the message cannot be framed, but the stream itself is
// still perfectly synchronized. Writers (connWriter) use it to fail only the
// offending message instead of latching the whole connection.
var errUnframeable = errors.New("jsonrpcstdio: message cannot be framed")

// framer reads and writes one wire framing. Implementations are safe for
// exactly one concurrent reader (ReadFrame owns the bufio.Reader) plus
// mutex-serialized writers (WriteFrame state is disjoint from read state,
// but WriteFrame itself is not safe for concurrent use — callers hold a
// write mutex, see connWriter).
type framer interface {
	// ReadFrame returns the next message body. The returned slice is freshly
	// allocated and owned by the caller: core hands it to handlers, which may
	// retain params past the call (easyjson aliases the input buffer), so the
	// read side must never pool or reuse these buffers.
	// It returns io.EOF only on clean EOF at a frame boundary; EOF mid-frame
	// is a wrapped io.ErrUnexpectedEOF.
	ReadFrame() ([]byte, error)
	// WriteFrame writes one message. Not safe for concurrent use. An error
	// wrapping errUnframeable means nothing was written and the stream is
	// still usable; any other error may have left a partial frame behind.
	WriteFrame(data []byte) error
}

// framerBase carries the state both framings share. sizeOpt names the
// user-facing option that raises maxSize, so limit errors point at the
// right knob.
type framerBase struct {
	r       *bufio.Reader
	w       io.Writer
	maxSize int64
	sizeOpt string
	started bool
}

// begin runs once before the first read: it discards a single UTF-8 BOM at
// the very start of the stream. Windows tooling occasionally emits one;
// under Content-Length framing it would corrupt the first header name,
// under NDJSON the first message. It must stay lazy (not in newFramer):
// an eager Peek there would block NewClient until the peer writes.
func (b *framerBase) begin() {
	if b.started {
		return
	}
	b.started = true
	p, err := b.r.Peek(3)
	if err == nil && p[0] == 0xEF && p[1] == 0xBB && p[2] == 0xBF {
		_, _ = b.r.Discard(3)
	}
}

// limitErr is the shared oversize error; both framings and both checks must
// emit the same text so tests and operators see one message.
func (b *framerBase) limitErr() error {
	return fmt.Errorf("jsonrpcstdio: frame exceeds the %d-byte limit (raise with %s)", b.maxSize, b.sizeOpt)
}

// writeAll writes parts sequentially with one shared error wrap. Each part
// stays its own Write call — see the callers for why the body is never
// copied into a compose buffer.
func writeAll(w io.Writer, parts ...[]byte) error {
	for _, p := range parts {
		if _, err := w.Write(p); err != nil {
			return fmt.Errorf("jsonrpcstdio: write: %w", err)
		}
	}
	return nil
}

// newFramer builds the framer for f.
func newFramer(f Framing, r io.Reader, w io.Writer, maxSize int64, sizeOpt string) framer {
	base := framerBase{
		r:       bufio.NewReaderSize(r, readerBufferSize),
		w:       w,
		maxSize: maxSize,
		sizeOpt: sizeOpt,
	}
	switch f {
	case FramingContentLength:
		return &contentLengthFramer{framerBase: base}
	case FramingNDJSON:
		return &ndjsonFramer{framerBase: base}
	default:
		panic("jsonrpcstdio: invalid framing") // unreachable: Serve/NewClient validate first
	}
}

// quoteTrunc renders peer-derived bytes safely for error messages: quoted
// (control characters escaped, so raw peer bytes cannot inject into logs)
// and truncated.
func quoteTrunc(b []byte) string {
	const max = 64
	if len(b) > max {
		return strconv.Quote(string(b[:max])) + "...(truncated)"
	}
	return strconv.Quote(string(b))
}

// contentLengthFramer implements the LSP base protocol: an ASCII header
// block ("Content-Length: N", lines terminated by \r\n or bare \n, block
// terminated by an empty line) followed by exactly N bytes of JSON.
type contentLengthFramer struct {
	framerBase
	hdr []byte // reusable write buffer for the ~30-byte header only
}

// clHeaderName is compared case-insensitively without allocating strings.
var clHeaderName = []byte("Content-Length")

// readHeaderLine reads one raw header line including its terminator.
// atBlockStart reports whether no byte of the current block was consumed
// yet, which is the only position where EOF is clean.
func (f *contentLengthFramer) readHeaderLine(atBlockStart bool) ([]byte, error) {
	line, err := f.r.ReadSlice('\n')
	switch {
	case err == nil:
		return line, nil
	case errors.Is(err, bufio.ErrBufferFull):
		// ReadSlice gives up at the reader buffer, which is deliberately
		// larger than the header cap — report the cap that governs.
		return nil, fmt.Errorf("jsonrpcstdio: header line exceeds the %d-byte header limit", maxHeaderBytes)
	case errors.Is(err, io.EOF):
		if len(line) == 0 && atBlockStart {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("jsonrpcstdio: stream ended mid-header: %w", io.ErrUnexpectedEOF)
	default:
		return nil, fmt.Errorf("jsonrpcstdio: read: %w", err)
	}
}

func (f *contentLengthFramer) ReadFrame() ([]byte, error) {
	f.begin()

	// Wrong-framing sniff, done by peeking so it works regardless of how
	// long the JSON line is (a header-cap error would otherwise mask the
	// hint for large messages): a header block can only start with a header
	// name, never with a JSON document.
	switch p, err := f.r.Peek(1); {
	case err == nil && (p[0] == '{' || p[0] == '['):
		return nil, errors.New(
			"jsonrpcstdio: expected a Content-Length header but the stream starts with JSON; peer appears to use NDJSON framing (did you mean FramingNDJSON?)")
	case errors.Is(err, io.EOF):
		return nil, io.EOF
	case err != nil:
		return nil, fmt.Errorf("jsonrpcstdio: read: %w", err)
	}

	var (
		contentLen  int64 = -1
		headerBytes int
		first       = true
	)
	for {
		line, err := f.readHeaderLine(first)
		if err != nil {
			return nil, err
		}
		first = false
		headerBytes += len(line)
		if headerBytes > maxHeaderBytes {
			return nil, fmt.Errorf("jsonrpcstdio: header block exceeds %d bytes", maxHeaderBytes)
		}

		trimmed := trimEOL(line)
		if len(trimmed) == 0 { // blank line: end of header block
			if contentLen < 0 {
				return nil, errors.New("jsonrpcstdio: missing Content-Length header")
			}
			break
		}

		idx := bytes.IndexByte(trimmed, ':')
		if idx < 0 {
			return nil, fmt.Errorf("jsonrpcstdio: invalid header line %s", quoteTrunc(trimmed))
		}
		name := bytes.TrimSpace(trimmed[:idx])
		if !bytes.EqualFold(name, clHeaderName) {
			continue // unknown headers (Content-Type, ...) are ignored, unparsed
		}
		if contentLen >= 0 {
			// Two lengths in one block is the classic desync primitive:
			// refusing is the only safe answer.
			return nil, errors.New("jsonrpcstdio: duplicate Content-Length header")
		}
		value := bytes.TrimSpace(trimmed[idx+1:])
		n, err := strconv.ParseInt(string(value), 10, 64)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("jsonrpcstdio: invalid Content-Length %s", quoteTrunc(value))
		}
		if n > f.maxSize {
			// Checked before allocating: a hostile length must not translate
			// into a huge allocation.
			return nil, f.limitErr()
		}
		contentLen = n
	}

	// A zero-length body is perfectly framed: consume nothing and hand the
	// empty message to core, which answers -32700 and the stream continues.
	buf := make([]byte, contentLen)
	if _, err := io.ReadFull(f.r, buf); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("jsonrpcstdio: stream ended mid-body: %w", io.ErrUnexpectedEOF)
		}
		return nil, fmt.Errorf("jsonrpcstdio: read: %w", err)
	}
	return buf, nil
}

func (f *contentLengthFramer) WriteFrame(data []byte) error {
	// Two writes instead of composing header+body into one buffer: this
	// avoids copying a potentially multi-MiB body per message just to
	// prepend a ~30-byte header.
	f.hdr = f.hdr[:0]
	f.hdr = append(f.hdr, "Content-Length: "...)
	f.hdr = strconv.AppendInt(f.hdr, int64(len(data)), 10)
	f.hdr = append(f.hdr, "\r\n\r\n"...)
	return writeAll(f.w, f.hdr, data)
}

// ndjsonFramer implements newline-delimited JSON as used by the MCP stdio
// transport: one JSON-RPC message per \n-terminated line.
type ndjsonFramer struct {
	framerBase
}

// contentLengthPrefix inside an NDJSON stream is never valid JSON whatever
// its casing, so matching it carries zero false-positive risk.
var contentLengthPrefix = []byte("Content-Length:")

func (f *ndjsonFramer) ReadFrame() ([]byte, error) {
	f.begin()

	for {
		var acc []byte
		for {
			chunk, err := f.r.ReadSlice('\n')
			// The -2 (not maxSize+2) keeps the comparison overflow-safe for
			// WithMaxMessageSize(math.MaxInt64); the slack allows a \r\n
			// terminator on a maxSize-byte line while still bounding memory
			// for a hostile endless line.
			if int64(len(acc)+len(chunk))-2 > f.maxSize {
				return nil, f.limitErr()
			}
			acc = append(acc, chunk...) // always copies: the caller owns the result
			if err == nil {
				break
			}
			if errors.Is(err, bufio.ErrBufferFull) {
				continue // line longer than the reader buffer: keep accumulating
			}
			if errors.Is(err, io.EOF) {
				if len(acc) == 0 {
					return nil, io.EOF
				}
				break // final unterminated line: accept it as a frame
			}
			return nil, fmt.Errorf("jsonrpcstdio: read: %w", err)
		}

		line := trimEOL(acc)
		if len(line) == 0 {
			continue // blank lines tolerated (keepalives, trailing newline)
		}
		// Reachable despite the loop bound: a bare-\n or unterminated final
		// line can come out of trimEOL 1-2 bytes over maxSize — this check
		// enforces the exact documented limit.
		if int64(len(line)) > f.maxSize {
			return nil, f.limitErr()
		}
		if len(line) >= len(contentLengthPrefix) && bytes.EqualFold(line[:len(contentLengthPrefix)], contentLengthPrefix) {
			// Wrong-framing sniff: an LSP header (any casing — the real CL
			// parser is case-insensitive, so the hint must be too) where a
			// JSON line belongs.
			return nil, fmt.Errorf(
				"jsonrpcstdio: read a Content-Length header inside NDJSON framing (%s); peer appears to use Content-Length framing (did you mean FramingContentLength?)",
				quoteTrunc(line))
		}
		return line, nil
	}
}

func (f *ndjsonFramer) WriteFrame(data []byte) error {
	// Core and easyjson emit compact JSON, so data is normally newline-free.
	// A handler CAN smuggle newlines in via a json.RawMessage result (e.g.
	// json.MarshalIndent output embedded verbatim by core) — compacting
	// recovers those messages instead of dropping them: a raw newline inside
	// a JSON string is invalid JSON, so any compactable message is
	// newline-free after json.Compact.
	if bytes.IndexByte(data, '\n') >= 0 {
		var buf bytes.Buffer
		if err := json.Compact(&buf, data); err != nil || bytes.IndexByte(buf.Bytes(), '\n') >= 0 {
			// Nothing was written: the stream is still synchronized, only
			// this message is lost. errUnframeable tells the writer not to
			// latch the connection.
			return fmt.Errorf("%w: message contains a raw newline and is not valid JSON", errUnframeable)
		}
		data = buf.Bytes()
	}
	return writeAll(f.w, data, newline)
}

var newline = []byte{'\n'}

// trimEOL strips one trailing \n and an optional preceding \r.
func trimEOL(b []byte) []byte {
	if n := len(b); n > 0 && b[n-1] == '\n' {
		b = b[:n-1]
	}
	if n := len(b); n > 0 && b[n-1] == '\r' {
		b = b[:n-1]
	}
	return b
}
