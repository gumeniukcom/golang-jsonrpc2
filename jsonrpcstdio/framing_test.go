package jsonrpcstdio

import (
	"bytes"
	"errors"
	"io"
	"math"
	"strings"
	"testing"
)

func newCLFramer(input string, maxSize int64) (*contentLengthFramer, *bytes.Buffer) {
	out := &bytes.Buffer{}
	return newFramer(FramingContentLength, strings.NewReader(input), out, maxSize, "WithMaxMessageSize").(*contentLengthFramer), out
}

func newNDFramer(input string, maxSize int64) (*ndjsonFramer, *bytes.Buffer) {
	out := &bytes.Buffer{}
	return newFramer(FramingNDJSON, strings.NewReader(input), out, maxSize, "WithMaxMessageSize").(*ndjsonFramer), out
}

// Content-Length frames that must parse, per the LSP base protocol.
func TestContentLengthReadFrame(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"canonical", "Content-Length: 2\r\n\r\n{}", []string{"{}"}},
		{"with content-type header", "Content-Length: 2\r\nContent-Type: application/vscode-jsonrpc; charset=utf-8\r\n\r\n{}", []string{"{}"}},
		{"case-insensitive header name", "content-length: 2\r\n\r\n{}", []string{"{}"}},
		{"bare LF terminators", "Content-Length: 2\n\n{}", []string{"{}"}},
		{"whitespace-padded value", "Content-Length:   2  \r\n\r\n{}", []string{"{}"}},
		{"two frames back to back", "Content-Length: 2\r\n\r\n{}Content-Length: 4\r\n\r\ntrue", []string{"{}", "true"}},
		{"zero length body is a valid empty frame", "Content-Length: 0\r\n\r\nContent-Length: 2\r\n\r\n{}", []string{"", "{}"}},
		{"leading UTF-8 BOM stripped", "\xEF\xBB\xBFContent-Length: 2\r\n\r\n{}", []string{"{}"}},
		{"unknown header ignored", "X-Whatever: yes\r\nContent-Length: 2\r\n\r\n{}", []string{"{}"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fr, _ := newCLFramer(tt.input, 1<<20)
			for i, want := range tt.want {
				got, err := fr.ReadFrame()
				if err != nil {
					t.Fatalf("frame %d: unexpected error: %v", i, err)
				}
				if string(got) != want {
					t.Fatalf("frame %d: got %q, want %q", i, got, want)
				}
			}
			if _, err := fr.ReadFrame(); !errors.Is(err, io.EOF) {
				t.Fatalf("after last frame want clean io.EOF, got %v", err)
			}
		})
	}
}

// Malformed Content-Length streams: each is fatal, and the error text must
// point the user in the right direction.
func TestContentLengthReadFrameFatal(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantMsg string // substring the error must contain
	}{
		{"missing content-length", "Content-Type: application/json\r\n\r\n{}", "missing Content-Length"},
		{"negative length", "Content-Length: -5\r\n\r\n{}", "invalid Content-Length"},
		{"non-numeric length", "Content-Length: two\r\n\r\n{}", "invalid Content-Length"},
		{"duplicate content-length", "Content-Length: 2\r\nContent-Length: 3\r\n\r\n{}", "duplicate Content-Length"},
		{"header line without colon", "Content-Length 2\r\n\r\n{}", "invalid header line"},
		{"json where headers belong hints ndjson", `{"jsonrpc":"2.0","method":"x","id":1}` + "\n", "did you mean FramingNDJSON?"},
		{"eof mid-header", "Content-Len", "mid-header"},
		{"eof mid-body", "Content-Length: 10\r\n\r\n{}", "mid-body"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fr, _ := newCLFramer(tt.input, 1<<20)
			_, err := fr.ReadFrame()
			if err == nil {
				t.Fatal("want fatal error, got nil")
			}
			if errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("fatal case must not be clean EOF: %v", err)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("error %q must contain %q", err, tt.wantMsg)
			}
		})
	}
}

// An over-limit Content-Length must fail before the body allocation, and
// the error must name the limit and the option that raises it.
func TestContentLengthOversizeFatalBeforeAlloc(t *testing.T) {
	// Claim a body of 1 TiB: if the framer allocated first, this test would
	// blow up; failing fast proves the check precedes the allocation.
	fr, _ := newCLFramer("Content-Length: 1099511627776\r\n\r\n", 64)
	_, err := fr.ReadFrame()
	if err == nil {
		t.Fatal("want fatal error, got nil")
	}
	for _, want := range []string{"64-byte limit", "WithMaxMessageSize"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q must contain %q", err, want)
		}
	}
}

// A single endless header line must die at the header cap with bounded
// memory, not accumulate forever.
func TestContentLengthHeaderCap(t *testing.T) {
	long := strings.Repeat("a", 40<<10) // 40 KiB, colon-free, no newline
	fr, _ := newCLFramer(long, 1<<20)
	_, err := fr.ReadFrame()
	if err == nil {
		t.Fatal("want fatal error, got nil")
	}
	if !strings.Contains(err.Error(), "header line exceeds") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Error messages embedding peer bytes must escape control characters and
// truncate, so raw attacker bytes cannot reach a log.
func TestContentLengthErrorsQuotePeerBytes(t *testing.T) {
	evil := "bad\x1b[31mheader\nrest" // ANSI escape + newline
	fr, _ := newCLFramer(evil+"\r\n\r\n", 1<<20)
	_, err := fr.ReadFrame()
	if err == nil {
		t.Fatal("want fatal error, got nil")
	}
	msg := err.Error()
	if strings.ContainsRune(msg, '\x1b') || strings.Contains(msg, "\nrest") {
		t.Fatalf("error must not contain raw control bytes: %q", msg)
	}
	if !strings.Contains(msg, `\x1b`) {
		t.Fatalf("error must contain the escaped form: %q", msg)
	}
}

// Content-Length write output is byte-exact and the body is not copied into
// the header buffer.
func TestContentLengthWriteFrame(t *testing.T) {
	fr, out := newCLFramer("", 1<<20)
	if err := fr.WriteFrame([]byte("{}")); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "Content-Length: 2\r\n\r\n{}"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	out.Reset()
	if err := fr.WriteFrame([]byte(`{"jsonrpc":"2.0","result":null,"id":1}`)); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "Content-Length: 38\r\n\r\n"+`{"jsonrpc":"2.0","result":null,"id":1}`; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// NDJSON frames that must parse, per the MCP stdio transport.
func TestNDJSONReadFrame(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"unix line endings", "{}\ntrue\n", []string{"{}", "true"}},
		{"windows line endings", "{}\r\ntrue\r\n", []string{"{}", "true"}},
		{"blank lines skipped", "\n\n{}\n\n\ntrue\n\n", []string{"{}", "true"}},
		{"final unterminated line accepted", "{}\ntrue", []string{"{}", "true"}},
		{"leading UTF-8 BOM stripped", "\xEF\xBB\xBF{}\n", []string{"{}"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fr, _ := newNDFramer(tt.input, 1<<20)
			for i, want := range tt.want {
				got, err := fr.ReadFrame()
				if err != nil {
					t.Fatalf("frame %d: unexpected error: %v", i, err)
				}
				if string(got) != want {
					t.Fatalf("frame %d: got %q, want %q", i, got, want)
				}
			}
			if _, err := fr.ReadFrame(); !errors.Is(err, io.EOF) {
				t.Fatalf("after last frame want clean io.EOF, got %v", err)
			}
		})
	}
}

// A line longer than the bufio buffer still parses (accumulation path) and
// one longer than the limit dies with bounded memory.
func TestNDJSONLongLines(t *testing.T) {
	big := `{"x":"` + strings.Repeat("a", 60<<10) + `"}` // > readerBufferSize
	fr, _ := newNDFramer(big+"\n", 1<<20)
	got, err := fr.ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != big {
		t.Fatalf("long line roundtrip mismatch: got %d bytes, want %d", len(got), len(big))
	}

	fr, _ = newNDFramer(strings.Repeat("a", 4096)+"\n", 64)
	_, err = fr.ReadFrame()
	if err == nil {
		t.Fatal("want fatal oversize error, got nil")
	}
	for _, want := range []string{"64-byte limit", "WithMaxMessageSize"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q must contain %q", err, want)
		}
	}
}

// An LSP header inside an NDJSON stream is a misconfiguration, not a
// message: fatal, with a hint at the right framing.
func TestNDJSONContentLengthHint(t *testing.T) {
	fr, _ := newNDFramer("Content-Length: 2\r\n\r\n{}", 1<<20)
	_, err := fr.ReadFrame()
	if err == nil {
		t.Fatal("want fatal error, got nil")
	}
	if !strings.Contains(err.Error(), "did you mean FramingContentLength?") {
		t.Fatalf("error must hint at Content-Length framing: %v", err)
	}
}

// NDJSON write appends exactly one newline; multiline JSON is compacted
// rather than rejected (see TestNDJSONWriteFrameCompactsMultilineJSON).
func TestNDJSONWriteFrame(t *testing.T) {
	fr, out := newNDFramer("", 1<<20)
	if err := fr.WriteFrame([]byte("{}")); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "{}\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// Framing implements fmt.Stringer for diagnostics.
func TestFramingString(t *testing.T) {
	tests := []struct {
		f    Framing
		want string
	}{
		{FramingContentLength, "FramingContentLength"},
		{FramingNDJSON, "FramingNDJSON"},
		{Framing(0), "Framing(0)"},
		{Framing(9), "Framing(9)"},
	}
	for _, tt := range tests {
		if got := tt.f.String(); got != tt.want {
			t.Errorf("Framing(%d).String() = %q, want %q", uint8(tt.f), got, tt.want)
		}
	}
}

// Regression: WithMaxMessageSize(math.MaxInt64) means "practically
// unlimited" — the NDJSON size math must not overflow and reject everything.
func TestNDJSONMaxInt64LimitNoOverflow(t *testing.T) {
	fr, _ := newNDFramer("{}\n", math.MaxInt64)
	got, err := fr.ReadFrame()
	if err != nil {
		t.Fatalf("MaxInt64 limit must not reject frames: %v", err)
	}
	if string(got) != "{}" {
		t.Fatalf("got %q", got)
	}
}

// Regression: the NDJSON-side framing hint must be as case-insensitive as
// the Content-Length parser it points at.
func TestNDJSONContentLengthHintCaseInsensitive(t *testing.T) {
	fr, _ := newNDFramer("content-length: 2\r\n\r\n{}", 1<<20)
	_, err := fr.ReadFrame()
	if err == nil || !strings.Contains(err.Error(), "FramingContentLength") {
		t.Fatalf("lowercase header must still trigger the hint, got %v", err)
	}
}

// Regression: the CL-side NDJSON hint is a one-byte peek, so it fires even
// for JSON lines far larger than the header cap (previously masked by a
// header-size error).
func TestContentLengthHintFiresForHugeJSONLine(t *testing.T) {
	line := `{"jsonrpc":"2.0","method":"x","params":"` + strings.Repeat("a", 40<<10) + `"}`
	fr, _ := newCLFramer(line+"\n", 1<<20)
	_, err := fr.ReadFrame()
	if err == nil || !strings.Contains(err.Error(), "did you mean FramingNDJSON?") {
		t.Fatalf("hint must fire regardless of line length, got %v", err)
	}
}

// Regression: a handler smuggling pretty-printed JSON into a RawMessage
// result must not kill the NDJSON stream — the frame is compacted instead.
func TestNDJSONWriteFrameCompactsMultilineJSON(t *testing.T) {
	fr, out := newNDFramer("", 1<<20)
	pretty := "{\n  \"a\": [1,\n 2]\n}"
	if err := fr.WriteFrame([]byte(pretty)); err != nil {
		t.Fatalf("compactable JSON must be framed, got %v", err)
	}
	if got, want := out.String(), `{"a":[1,2]}`+"\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	// Non-JSON with a newline is genuinely unframeable: error, nothing
	// written, and the error is marked errUnframeable so the writer does
	// not latch the connection.
	out.Reset()
	err := fr.WriteFrame([]byte("not\njson"))
	if err == nil || !errors.Is(err, errUnframeable) {
		t.Fatalf("want errUnframeable, got %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("nothing may reach the wire on an unframeable message, got %q", out.String())
	}
}

// Fuzz invariants for the Content-Length reader: no panic, frames respect
// the limit, and the reader either errs or makes progress (no livelock).
func FuzzContentLengthReadFrame(f *testing.F) {
	f.Add([]byte("Content-Length: 2\r\n\r\n{}"))
	f.Add([]byte("Content-Length: 0\r\n\r\n"))
	f.Add([]byte("content-length:2\nx:y\n\n{}"))
	f.Add([]byte("Content-Length: -1\r\n\r\n"))
	f.Add([]byte("Content-Length: 2\r\nContent-Length: 2\r\n\r\n{}"))
	f.Add([]byte("\xEF\xBB\xBFContent-Length: 1\n\nz"))
	f.Add([]byte("{\"jsonrpc\":\"2.0\"}\n"))
	f.Add([]byte(""))
	f.Fuzz(func(t *testing.T, data []byte) {
		const maxSize = 1 << 10
		fr := newFramer(FramingContentLength, bytes.NewReader(data), io.Discard, maxSize, "opt")
		for i := 0; i < 100; i++ { // input is finite; 100 frames is plenty
			frame, err := fr.ReadFrame()
			if err != nil {
				return
			}
			if int64(len(frame)) > maxSize {
				t.Fatalf("frame of %d bytes exceeds the %d limit", len(frame), maxSize)
			}
		}
	})
}

// Fuzz invariants for the NDJSON reader: same contract.
func FuzzNDJSONReadFrame(f *testing.F) {
	f.Add([]byte("{}\n"))
	f.Add([]byte("\n\n\r\n{}"))
	f.Add([]byte("Content-Length: 2\r\n"))
	f.Add([]byte("\xEF\xBB\xBF{}\n"))
	f.Add([]byte(""))
	f.Fuzz(func(t *testing.T, data []byte) {
		const maxSize = 1 << 10
		fr := newFramer(FramingNDJSON, bytes.NewReader(data), io.Discard, maxSize, "opt")
		for i := 0; i < 100; i++ {
			frame, err := fr.ReadFrame()
			if err != nil {
				return
			}
			if int64(len(frame)) > maxSize {
				t.Fatalf("frame of %d bytes exceeds the %d limit", len(frame), maxSize)
			}
			if len(frame) == 0 {
				continue // only Content-Length framing can carry an empty frame
			}
		}
	})
}

// WriteFrame→ReadFrame is the identity for newline-free messages, under
// both framings.
func FuzzRoundTrip(f *testing.F) {
	f.Add([]byte("{}"))
	f.Add([]byte(`{"jsonrpc":"2.0","method":"x","params":[1,2],"id":1}`))
	f.Add([]byte("null"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if bytes.IndexByte(data, '\n') >= 0 || len(data) == 0 || data[len(data)-1] == '\r' {
			return // NDJSON forbids embedded newlines; empty is a blank line,
			// and a trailing \r is indistinguishable from a CRLF ending
		}
		for _, framing := range []Framing{FramingContentLength, FramingNDJSON} {
			var buf bytes.Buffer
			wfr := newFramer(framing, strings.NewReader(""), &buf, 1<<20, "opt")
			if err := wfr.WriteFrame(data); err != nil {
				t.Fatalf("%v: write: %v", framing, err)
			}
			rfr := newFramer(framing, bytes.NewReader(buf.Bytes()), io.Discard, 1<<20, "opt")
			got, err := rfr.ReadFrame()
			if err != nil {
				t.Fatalf("%v: read back: %v", framing, err)
			}
			if !bytes.Equal(got, data) {
				t.Fatalf("%v: roundtrip mismatch: got %q, want %q", framing, got, data)
			}
		}
	})
}
