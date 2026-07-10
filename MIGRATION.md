# encoding/json/v2 migration plan

The standard library's `encoding/json/v2` (with `encoding/json/jsontext`) is
available in Go 1.25/1.26 **only** under `GOEXPERIMENT=jsonv2`, and is targeted
to become the default, importable-without-a-flag package in a later Go release
(≈ Go 1.27). Importing it unconditionally today would break every normal
`go build` on Go 1.25/1.26 — so the library stays on `encoding/json` (v1) by
default and remains usable on Go 1.25+ with no build flags.

This document records the migration so the switch, when json/v2 is stable, is
a small, well-scoped change rather than a rewrite.

## What is already prepared

- **All generic, reflection-based JSON goes through one seam:**
  `internal/codec` (`Marshal` / `Unmarshal` / `Valid`). Method params/results
  (`Typed`), client payloads (`CallResult`, `MarshalParams`, `BatchResultAs`),
  `toID` fallbacks, structural validation (`errorForMalformed`, id checks),
  and OpenRPC document generation all call `codec.*` instead of `json.*`.
- **CI already runs the whole suite under `GOEXPERIMENT=jsonv2`** (the
  `jsonv2` job), so forward compatibility is continuously verified.

## Remaining steps when json/v2 lands as a stable package

1. **Flip `internal/codec`** (`internal/codec/codec.go`) from `encoding/json`
   to `encoding/json/v2`. Optionally opt into stricter decoding
   (`jsonv2.RejectDuplicateNames`) to close the last parser-differential gap.
   This is a one-file change; every call site already routes through it.

2. **Replace the two streaming batch-array decoders** — marked with
   `// json/v2 migration point` in `jsonrpchttp/client.go` and
   `jsonrpcws/client.go` — with `jsontext.Decoder` (`ReadToken` / `ReadValue`).
   Both are localized (~10 lines each).

3. **Retire easyjson.** The `structs` package (Request, Response, Error, ID)
   uses easyjson-generated and hand-written codecs (`structs/*_easyjson.go`,
   `structs/*_codec.go`). Regenerate them as json/v2 codecs or replace with
   json/v2 reflection, then drop the `github.com/mailru/easyjson` dependency
   and the `make easy` target. This removes the last third-party serialization
   dependency (a long-standing supply-chain concern) and is the largest step.

4. **Bump `go` directive** in `go.mod` (and the adapter modules) to the first
   release where json/v2 is default, and drop the `GOEXPERIMENT=jsonv2` CI job
   (it becomes redundant).

Until all steps land, the default build behavior is unchanged and Go 1.25+
apps are unaffected.
