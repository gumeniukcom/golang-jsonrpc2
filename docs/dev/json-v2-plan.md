> Internal engineering plan for developing this library itself — not user
> documentation. For migrating *your* code, see [../migrating.md](../migrating.md).

# encoding/json/v2 migration plan

**Status (2026-08-29): deliberately not started. Re-evaluate around Go 1.29.**

Go 1.27 (released 2026-08-19) reimplemented `encoding/json` on top of
`encoding/json/v2`, and made `encoding/json/v2` and `encoding/json/jsontext`
importable with no `GOEXPERIMENT`. The v1 API keeps v1 semantics; only the
implementation underneath it changed. Go documents marshal as "broadly at
parity" and unmarshal as "significantly faster".

## Why the migration is not urgent

**The new implementation is selected by a build tag, not by the `go`
directive in go.mod and not by a GODEBUG:**

```
$GOROOT/src/encoding/json/encode.go:5     //go:build !goexperiment.jsonv2
$GOROOT/src/encoding/json/v2_encode.go:5  //go:build goexperiment.jsonv2
```

So this module, at its declared floor of `go 1.26.0`, **already gets the whole
improvement whenever it is built by a Go 1.27 toolchain**. Measured on this
repo's own `bench_test.go` (Apple M3, `-count=10`, benchstat,
`GOEXPERIMENT=nojsonv2` → default, both on go1.27.0):

| | B/op | allocs/op | sec/op |
|---|---:|---:|---:|
| SingleRequest | −21.1% | −15.0% | −8.6% |
| Notification | −29.6% | −18.8% | ~ |
| Batch10 | −18.3% | −15.4% | −31.3% |
| Batch100 | −21.3% | −17.1% | −20.3% |
| LargeParams10KB | ~ | ~ | −39.9% (+66% B/s) |
| **geomean** | **−17.1%** | **−12.3%** | −13.2% |

Consumers get this by upgrading their own toolchain, with no change in this
library and no floor bump. This matters for how the floor is chosen: the floor
tracks Go's **supported set** (as of v2.8.0, `go 1.26.0` — the oldest release
Go still supports), it is *not* a lever for performance. Raising it to 1.27
early would buy **zero** performance; it would only permit writing the new
import paths, at the cost of every consumer on 1.26. Move it to 1.27 when 1.27
becomes the oldest supported release, not before.

Note also *where* the gain lands. The request/response envelope is decoded by
easyjson on raw bytes, not by reflection, so the improvement applies to the
**user's** param and result structs. It scales with payload size and is close
to noise on small calls.

## What is already prepared

- **All generic, reflection-based JSON goes through one seam:**
  `internal/codec` (`Marshal` / `Unmarshal` / `Valid`). Method params/results
  (`Typed`), client payloads (`CallResult`, `MarshalParams`, `BatchResultAs`),
  `toID` fallbacks, structural validation (`errorForMalformed`, id checks),
  arbitrary `Error.Data` values (`structs/error_codec.go`), and OpenRPC
  document generation all call `codec.*` instead of `json.*`.
- **CI runs the whole suite under `GOEXPERIMENT=nojsonv2`** (the `nojsonv2`
  job), covering the opt-out consumers reach for when they hit trouble. The
  default matrix entries cover the v2-backed implementation. Delete that job
  once Go removes the flag.

## The trap: v2 defaults are not v1 defaults

A naive flip of `internal/codec` to `encoding/json/v2` is a silent breaking
change. Measured on Go 1.27:

| input | v1 (today) | bare `json/v2` |
|---|---|---|
| `{"a":1}` into field `A` | `A=1` | **`A=0`, no error** |
| `{"A":1,"A":2}` | `A=2` | error |
| marshal nil slice / nil map | `null` | `[]` / `{}` |

The first row is silent data loss for every caller that sends a differently
cased member; the third changes the wire format of any result containing a nil
slice or map. **Any flip must pass `json.DefaultOptionsV1()`** to stay
bug-for-bug compatible. Adopting the stricter defaults is a breaking change
that belongs in a major version, or behind an opt-in server option defaulting
to off — never a silent default change in a v2 minor.

## Remaining steps, and their gating

1. **Flip `internal/codec`** (`internal/codec/codec.go`) to
   `encoding/json/v2`, passing `json.DefaultOptionsV1()`. Requires a go.mod
   floor of 1.27 (unconditional import), so it is gated on the floor bump —
   *not* on json/v2 being available. Blast radius: none if
   `DefaultOptionsV1()` is used; see the table above for what happens if it
   is not.

2. **Replace the two streaming batch-array decoders** — marked with
   `// json/v2 migration point` in `jsonrpchttp/client.go` and
   `jsonrpcws/client.go` — with `jsontext.Decoder` (`ReadToken` / `ReadValue`).
   Both are localized (~10 lines each) and currently correct (array root
   validated, iteration bounded by the pending-id count, body size-capped
   before decoding). They work today because `json.Decoder` dispatches to
   `structs.Response`'s `json.Unmarshaler`; `jsontext.Decoder` has no such
   notion, so the replacement must read each element as a raw value and hand
   it to whatever codec `structs.Response` exposes. **Sequence this with
   step 3** — if step 3 changes that method first, both call sites break at
   once. Also gated on the floor bump.

3. **Retire easyjson.** The `structs` package (Request, Response, Error, ID)
   uses easyjson-generated and hand-written codecs (`structs/*_easyjson.go`,
   `structs/*_codec.go`). This is a **supply-chain** decision with a measured
   performance cost, not a performance win — do not present it as one.
   Measured envelope decode of `structs.Request` on Go 1.27:

   | path | sec/op | B/op |
   |---|---:|---:|
   | easyjson (today) | **281 ns** | **48 B** |
   | `encoding/json/v2` direct | 377 ns | 112 B |
   | stdlib v1 API on 1.27 | 449 ns | 112 B |

   Purpose-built codecs beat reflection on small fixed-shape structs; v2's
   "significantly faster" is relative to v1 reflection, not to generated code.
   If it is done anyway, preserve the two safety properties the hand-written
   codecs exist for: a present `"id":null` must stay distinguishable from an
   absent id, and `Error.Data` must be captured as raw bytes rather than
   decoded into an `any` through a depth-unbounded recursive lexer.

4. **Bump the `go` directive** in go.mod (and the adapter modules) from 1.26
   to 1.27, and drop the `nojsonv2` CI job once the flag is gone. Gate the
   bump on Go 1.27 becoming the *oldest supported* release — i.e. when Go 1.29
   ships — so it costs consumers nothing beyond the support policy they are
   already on. That is the release in which steps 1 and 2 become available.

## Strictness is blocked on step 3, not available in step 1

An earlier revision of this plan suggested opting into strict decoding in
step 1 "to close the last parser-differential gap". That is wrong on two
counts.

First, the symbol it named, `jsonv2.RejectDuplicateNames`, does not exist —
the shipped API is `jsontext.AllowDuplicateNames(false)`. The plan was written
against a pre-release API.

Second, and more important: the differential lives in the **envelope**, which
`internal/codec` does not parse. easyjson takes last-value-wins on a
duplicated member, so `{"jsonrpc":"2.0","method":"m","method":"nope","id":1}`
dispatches `nope` while a first-wins security proxy in front of it sees `m`.
Turning on strictness in `codec` would make *params* strict while the envelope
stayed permissive — a worse asymmetry, with the method-name smuggling vector
still open. The gap only closes when the envelope parser changes, i.e. at
step 3.

## Compatibility promise

The library guarantees **semantic** equivalence of its JSON output across
toolchains and `GOEXPERIMENT` settings, not byte-for-byte equality. Go 1.27
already changed one observable encoding: invalid UTF-8 marshals as raw U+FFFD
bytes where the old implementation emitted the `�` escape. Do not add
byte-exact golden assertions over `codec.Marshal` output.
