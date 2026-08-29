# Benchmarks

> Measured **2026-08-29** on Apple M3 (macOS, mains power), **Go 1.27.0**.
> Competitor versions: `sourcegraph/jsonrpc2 v0.2.1`,
> `creachadair/jrpc2 v1.3.5` (pinned in `benchmarks/go.mod`). Numbers are
> **benchstat medians over `-count=15`** runs of the in-repo suite —
> reproduce with `make bench-compare` (or `make bench-save`). These are
> laptop numbers: the **relative ordering is the claim, not the absolute
> microseconds.**
>
> The previous snapshot was taken on Go 1.25.12. This one crosses **two** Go
> releases — 1.26 (Green Tea GC on by default) and 1.27 (`encoding/json`
> reimplemented on `encoding/json/v2`, size-specialized allocation). Absolute
> numbers improved substantially for *every* library, and some orderings
> moved; see "What changed since Go 1.25" below. No library code changed.

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
| baseline (hand-rolled) | 3.03 µs | 13 | 0.43 |
| **this library** | **6.72 µs** | **31** | **0.96** |
| net/rpc (JSON-RPC 1.0) | 7.02 µs | 23 | 1.00 |
| creachadair/jrpc2 | 11.30 µs | 91 | 1.61 |
| sourcegraph/jsonrpc2 | 14.31 µs | 108 | 2.04 |

Concurrent callers on one connection (`RunParallel`; sourcegraph runs its
`AsyncHandler`, its idiomatic concurrent path):

| Library | time/op | allocs/op |
|---|---:|---:|
| **this library** | **6.01 µs** | **31** |
| net/rpc (JSON-RPC 1.0) | 6.72 µs | 23 |
| sourcegraph/jsonrpc2 | 10.87 µs | 109 |
| creachadair/jrpc2 | 11.02 µs | 91 |

## Message-level dispatch (no transport)

Raw request bytes in, raw response bytes out — only our dispatcher and the
baseline have this entry point:

| | time/op | allocs/op |
|---|---:|---:|
| **this library** (`HandleRPCJSONRawMessage`) | **1.04 µs** | 17 |
| hand-rolled `encoding/json` dispatcher | 1.02 µs | 9 |

## Batch (server-side, one raw frame)

sourcegraph/jsonrpc2 and net/rpc have no batch support (that is a feature
gap, not a benchmark omission):

| | 10 calls | 100 calls |
|---|---:|---:|
| **this library** | **30.7 µs / 184 allocs** | **222.1 µs / 1 569 allocs** |
| creachadair/jrpc2 | 48.8 µs / 376 allocs | 336.5 µs / 3 566 allocs |

## 10 KiB echo (large payloads)

| Library | throughput (MiB/s) | allocs/op |
|---|---:|---:|
| net/rpc (JSON-RPC 1.0) | 291 | 26 |
| **this library** | **232** | 52 |
| creachadair/jrpc2 | 190 | 101 |
| sourcegraph/jsonrpc2 | 103 | 115 |

## Reading the numbers honestly

- Against the two widely-used JSON-RPC **2.0** libraries, an end-to-end call
  here is **1.7–2.1× faster with ~3× fewer allocations**, batches are ~1.5×
  faster with ~2.3× fewer allocations, and large-payload throughput is
  1.2–2.2× higher.
- **Stdlib `net/rpc` is no longer ahead on small end-to-end calls.** On Go
  1.25 it was ~23% faster; it is now roughly a tie sequentially (6.72 µs vs
  7.02 µs, and our figure carries the wider spread of the two), and we are
  ~11% ahead with concurrent callers. Read this as *parity reached*, not as a
  win — `net/rpc` still allocates less per call (23 vs 31) and speaks the
  simpler JSON-RPC 1.0 wire format with no batches, spec notifications,
  limits, middleware, or context plumbing.
- **`net/rpc` is now clearly ahead on large payloads** (291 vs 232 MiB/s, and
  half the allocations). On Go 1.25 the two were level. Large-payload echo is
  the one arena where the stdlib floor beats us.
- At the message level this library is now **at parity with** the hand-rolled
  baseline (1.04 µs vs 1.02 µs, inside the noise) rather than below it. The
  baseline gained more from the Go 1.27 stdlib than we did — its allocations
  nearly halved (17 → 9) — because it decodes with reflection, which is
  exactly what Go 1.27 sped up, while our envelope path already avoided it.

## What changed since Go 1.25

Same code, same competitor versions, two Go releases later. Recompiling alone
moved this library by −16.9% time / −23.4% bytes / −16.2% allocations on the
pipe arena and −21.1% bytes on message dispatch. Consumers get that by
upgrading their own toolchain; the module's floor is `go 1.26.0` and no
source change was needed (see [dev/json-v2-plan.md](dev/json-v2-plan.md) for
why). The gains are not evenly distributed: reflection-heavy code — the
baseline, `net/rpc`, and the competitors' decode paths — benefited most,
which is why several gaps narrowed and two orderings flipped.

## Caveats — what these numbers do NOT show

No network, no TLS, one machine, one method shape (`sum`), JSON-RPC 1.0
for `net/rpc`. Transport-level arenas (HTTP servers, WebSocket) are
deliberately excluded — they mostly measure `net/http`, not the RPC
library. Your handlers will dominate real-world latency long before the
dispatcher does.

## Refresh policy

Re-measured and updated on: competitor version bumps (Dependabot,
monthly, grouped), our minor/major releases, **major Go releases** (the
stdlib moves under everyone, as 1.26/1.27 showed), and at latest every six
months — the date stamp above makes staleness self-evident. A monthly CI
workflow re-runs the suite on shared runners as an ordering tripwire only
(absolutes from shared runners are not published).
