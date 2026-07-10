# Benchmarks

> Measured **2026-07-10** on Apple M3 (macOS, mains power), Go 1.25.12.
> Competitor versions: `sourcegraph/jsonrpc2 v0.2.1`,
> `creachadair/jrpc2 v1.3.5` (pinned in `benchmarks/go.mod`). Numbers are
> **benchstat medians over `-count=10`** runs of the in-repo suite —
> reproduce with `make bench-compare` (or `make bench-save`). These are
> laptop numbers: the **relative ordering is the claim, not the absolute
> microseconds.**

The suite lives in [`benchmarks/`](../benchmarks/README.md) with its
fairness rules (shared handler logic and payloads, all logging off, every
response validated, `ReportAllocs` everywhere). `net/rpc` uses its stdlib
jsonrpc codec — **JSON-RPC 1.0**, included as the stdlib floor, not as a
2.0 peer; *baseline* is a hand-rolled `encoding/json` dispatcher — the
"no library at all" floor.

## End-to-end call over net.Pipe (headline)

One complete `sum` call: client encodes and writes, server reads,
dispatches, replies, client reads and decodes.

| Library | time/op | allocs/op | relative time (net/rpc = 1.0) |
|---|---:|---:|---:|
| baseline (hand-rolled) | 4.17 µs | 27 | 0.77 |
| net/rpc (JSON-RPC 1.0) | 5.45 µs | 27 | 1.00 |
| **this library** | **7.04 µs** | **37** | **1.29** |
| creachadair/jrpc2 | 14.27 µs | 128 | 2.62 |
| sourcegraph/jsonrpc2 | 15.08 µs | 125 | 2.77 |

Concurrent callers on one connection (`RunParallel`; sourcegraph runs its
`AsyncHandler`, its idiomatic concurrent path):

| Library | time/op | allocs/op |
|---|---:|---:|
| net/rpc (JSON-RPC 1.0) | 4.94 µs | 27 |
| **this library** | **6.90 µs** | **37** |
| creachadair/jrpc2 | 8.93 µs | 128 |
| sourcegraph/jsonrpc2 | 9.58 µs | 126 |

## Message-level dispatch (no transport)

Raw request bytes in, raw response bytes out — only our dispatcher and the
baseline have this entry point:

| | time/op | allocs/op |
|---|---:|---:|
| **this library** (`HandleRPCJSONRawMessage`) | **1.13 µs** | 20 |
| hand-rolled `encoding/json` dispatcher | 1.26 µs | 17 |

## Batch (server-side, one raw frame)

sourcegraph/jsonrpc2 and net/rpc have no batch support (that is a feature
gap, not a benchmark omission):

| | 10 calls | 100 calls |
|---|---:|---:|
| **this library** | **47.7 µs / 219 allocs** | **212.7 µs / 1 874 allocs** |
| creachadair/jrpc2 | 58.6 µs / 577 allocs | 429.4 µs / 5 467 allocs |

## 10 KiB echo (large payloads)

| Library | throughput (MiB/s) | allocs/op |
|---|---:|---:|
| net/rpc (JSON-RPC 1.0) | 112 | 30 |
| **this library** | **111** | 58 |
| creachadair/jrpc2 | 62 | 137 |
| sourcegraph/jsonrpc2 | 30 | 133 |

## Reading the numbers honestly

- Against the two widely-used JSON-RPC **2.0** libraries, an end-to-end
  call here is **~2× faster with ~3.4× fewer allocations**, batches are
  ~2× faster at 100 entries, and large-payload throughput is 1.8–3.7×
  higher.
- Stdlib `net/rpc` remains ~23% faster on small end-to-end calls — it
  speaks the simpler JSON-RPC 1.0 wire format, has no batches,
  notifications-per-spec, limits, middleware, or context plumbing. We
  match it on large-payload throughput.
- The hand-rolled baseline shows what a zero-feature dispatcher costs;
  at the message level this library is *at or below* that floor (easyjson
  decode beats `encoding/json`), the gap on the pipe arena is the price
  of the full client (id correlation, multiplexing) and transport
  framing.

## Caveats — what these numbers do NOT show

No network, no TLS, one machine, one method shape (`sum`), JSON-RPC 1.0
for `net/rpc`. Transport-level arenas (HTTP servers, WebSocket) are
deliberately excluded — they mostly measure `net/http`, not the RPC
library. Your handlers will dominate real-world latency long before the
dispatcher does.

## Refresh policy

Re-measured and updated on: competitor version bumps (Dependabot,
monthly, grouped), our minor/major releases, and at latest every six
months — the date stamp above makes staleness self-evident. A monthly CI
workflow re-runs the suite on shared runners as an ordering tripwire only
(absolutes from shared runners are not published).
