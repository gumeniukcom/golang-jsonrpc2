# benchmarks — internal comparison suite

Cross-library benchmarks of `golang-jsonrpc2` against other Go JSON-RPC
implementations. **Internal tooling: not tagged, not supported, may break
or vanish at any time.** All code lives in `_test.go` files and an
`internal/` package, so nothing here is importable; do not depend on this
module.

Published, curated results live in [`../docs/BENCHMARKS.md`](../docs/BENCHMARKS.md)
with hardware/version disclosure. Raw local runs land in `results/`
(gitignored).

## Running

From the repo root:

```bash
make bench-compare   # -count=10, all arenas — takes a few minutes
make bench-save      # same, saved to benchmarks/results/<date>.txt
```

Compare two saved runs with `make benchstat OLD=... NEW=...`.

## Arenas

| Arena | File | Who competes | What it answers |
|---|---|---|---|
| Message-level | `message_test.go` | ours, hand-rolled baseline | dispatcher cost with no transport (others have no such entry point) |
| End-to-end `net.Pipe` | `pipe_test.go` | ours, sourcegraph/jsonrpc2, creachadair/jrpc2, net/rpc¹, baseline | the headline: one full call, client encode → server dispatch → client decode |
| Batch 10/100 | `batch_test.go` | ours, creachadair | server-side batch processing; sourcegraph and net/rpc have no batches (N/A) |
| 10 KiB echo | `payload_test.go` | ours, sourcegraph, creachadair, net/rpc | large-payload throughput (MB/s) |

¹ stdlib `net/rpc` with its jsonrpc codec speaks JSON-RPC **1.0** — it is
the "what does stdlib cost" floor, not a 2.0 peer. The baseline is a
hand-rolled `encoding/json` dispatcher: the "no library at all" floor.

## Fairness rules

Enforced by `internal/arena` and the adapter shape; changes must preserve
them:

1. Handler logic is defined once (`arena.Sum`, `arena.Echo`) — adapters are
   thin wiring only.
2. Identical wire payloads, built by shared arena builders.
3. All logging disabled in every library.
4. Every response is read **and validated** every iteration — dead-code
   elimination cannot fake a fast library, and a broken adapter cannot
   ship a number.
5. `b.ReportAllocs()` everywhere; framings matched as closely as each
   library allows (NDJSON-style line framing for ours, `PlainObjectCodec`
   for sourcegraph, `channel.Line` for creachadair).
6. Competitor versions are pinned in `go.mod`; `replace` points our side at
   the working tree, so the suite always measures the current code.

Excluded libraries and why: ybbus/jsonrpc (client-only — no server to
benchmark), osamingo/jsonrpc (HTTP-handler-only; an HTTP arena measures
net/http, not the library), semrush/zenrpc (dormant), filecoin/go-jsonrpc
(reflection+WebSocket-first API — no comparable stream entry point).

## Result hygiene

- Absolute numbers are only published from a quiet local machine (see the
  disclosure header in `docs/BENCHMARKS.md`) using benchstat medians over
  `-count=10`.
- CI runs this suite monthly (artifact only, never auto-committed): shared
  runners produce noisy absolutes, so only the *ordering* is checked
  there. One red scheduled run → re-run it; two consecutive → re-measure
  locally and update `docs/BENCHMARKS.md` or retract the claim.
- Competitor version bumps (Dependabot, monthly, grouped) are reviewed
  before merge — their code executes on CI runners — and prompt a local
  re-measure.
