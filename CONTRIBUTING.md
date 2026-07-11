# Contributing

Thanks for considering a contribution. This page is short on purpose — the
process is ordinary GitHub flow.

## How to contribute

- **Bugs and feature requests**: open a
  [GitHub issue](https://github.com/gumeniukcom/golang-jsonrpc2/issues).
  For anything security-sensitive, use the private channel described in
  [SECURITY.md](SECURITY.md) instead.
- **Code**: fork, branch, open a pull request against `master`. Small,
  focused PRs review fastest. For substantial changes, open an issue first
  so the design can be discussed before you invest time.
- **Docs**: corrections to [docs/](docs/README.md), including the
  [comparison](docs/COMPARISON.md) and [benchmark](docs/BENCHMARKS.md)
  pages, are very welcome — those pages explicitly invite them.

## Requirements for changes

1. **Tests are required for functionality changes.** New functionality must
   come with tests covering it; bug fixes must include a regression test
   that fails before the fix. Documentation-only changes are exempt.
2. **The full check suite must pass**: `make all` runs formatting, lint,
   and tests; CI additionally runs the race detector, both supported Go
   versions, `govulncheck`, CodeQL, and the nested adapter modules.
3. **Style** is enforced by `gofmt` and `golangci-lint` (config in
   `.golangci.yml`) — no manual style rules beyond that. Commit messages
   follow [Conventional Commits](https://www.conventionalcommits.org)
   (`feat:`, `fix:`, `docs:`, `chore:`, ...).
4. **Spec conformance matters.** Behavior changes to the dispatcher must
   cite the [JSON-RPC 2.0 spec](https://www.jsonrpc.org/specification)
   section they implement; `spec_test.go` is the conformance suite.
5. **No new dependencies in the core module** without prior discussion —
   heavyweight integrations live in nested modules (see `jsonrpcfiber/`).

## Development quickstart

```bash
make test      # go test ./...
make testrace  # go test -race ./...
make lint      # golangci-lint
make bench     # root package benchmarks
```

The `benchmarks/` directory is an internal cross-library comparison suite
with its own rules — see [benchmarks/README.md](benchmarks/README.md)
before touching it.

## Conduct

Be kind and constructive. Reports of unacceptable behavior go to the
maintainer at <i@gumeniuk.com>.
