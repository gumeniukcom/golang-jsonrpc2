# OpenSSF Best Practices badge — questionnaire draft (passing level)

> Internal engineering doc. Register the project at
> <https://www.bestpractices.dev/en/projects/new> (sign in with GitHub,
> pick the repo — most Basics auto-fill), then paste the justifications
> below. Answers are for the **passing** level as of 2026-07-11.
> Repo URL used in justifications:
> `https://github.com/gumeniukcom/golang-jsonrpc2`.

Legend: **Met** / **N/A** with the justification text to paste.

## Basics

| Criterion | Answer | Justification to paste |
|---|---|---|
| description_good | Met | The README states what the software does (spec-conformant JSON-RPC 2.0 dispatcher for Go with five transports) and who would benefit. https://github.com/gumeniukcom/golang-jsonrpc2#readme |
| interact | Met | GitHub issues and pull requests are open to all. https://github.com/gumeniukcom/golang-jsonrpc2/issues |
| contribution | Met | CONTRIBUTING.md describes the process (issues, PR flow, requirements). https://github.com/gumeniukcom/golang-jsonrpc2/blob/master/CONTRIBUTING.md |
| contribution_requirements | Met | CONTRIBUTING.md "Requirements for changes": tests required, `make all` green, gofmt/golangci-lint style, Conventional Commits. |
| floss_license | Met | MIT. |
| floss_license_osi | Met | MIT is OSI-approved. |
| license_location | Met | LICENSE file in the repository root. https://github.com/gumeniukcom/golang-jsonrpc2/blob/master/LICENSE |
| documentation_basics | Met | README quick start plus task-oriented guides in docs/ (transports, clients, hardening, observability, etc.). https://github.com/gumeniukcom/golang-jsonrpc2/tree/master/docs |
| documentation_interface | Met | Full API reference generated from doc comments on pkg.go.dev, including runnable examples. https://pkg.go.dev/github.com/gumeniukcom/golang-jsonrpc2/v2 |
| sites_https | Met | GitHub and pkg.go.dev are HTTPS-only. |
| discussion | Met | GitHub issues/PRs support discussion with URLs; searchable and archived. |
| english | Met | All documentation and code comments are in English. |
| maintained | Met | Actively maintained: multiple releases and 30+ commits in the last 90 days. https://github.com/gumeniukcom/golang-jsonrpc2/releases |

## Change Control

| Criterion | Answer | Justification to paste |
|---|---|---|
| repo_public | Met | Public GitHub repository. |
| repo_track | Met | git history tracks who/what/when for every change. |
| repo_interim | Met | Interim commits between releases are on master. |
| repo_distributed | Met | git. |
| version_unique | Met | Every release has a unique SemVer tag (v2.x.y). |
| version_semver | Met | SemVer for the /v2 module; documented in README "Versioning". |
| version_tags | Met | Releases are git tags (v2.6.0, ...) plus nested-module tags. https://github.com/gumeniukcom/golang-jsonrpc2/tags |
| release_notes | Met | CHANGELOG.md (Keep a Changelog format) and generated GitHub release notes for every release. https://github.com/gumeniukcom/golang-jsonrpc2/blob/master/CHANGELOG.md |
| release_notes_vulns | N/A | No publicly known vulnerabilities have been fixed in this project's code yet; when one is, its CVE/advisory will be listed in CHANGELOG and the GitHub advisory. |

## Reporting

| Criterion | Answer | Justification to paste |
|---|---|---|
| report_process | Met | Bugs via GitHub issues (CONTRIBUTING.md); vulnerabilities via the private process in SECURITY.md. |
| report_tracker | Met | GitHub issue tracker. |
| report_responses | Met | The maintainer responds to issues; the project is small enough that all reports get a response. |
| enhancement_responses | Met | Enhancement requests are acknowledged in the issue tracker. |
| report_archive | Met | Issues and PRs are permanently archived on GitHub. |
| vulnerability_report_process | Met | SECURITY.md: private reporting via GitHub Security Advisories. https://github.com/gumeniukcom/golang-jsonrpc2/blob/master/SECURITY.md |
| vulnerability_report_private | Met | GitHub "Private vulnerability reporting" is enabled on the repository. |
| vulnerability_report_response | Met | SECURITY.md commits to acknowledgement within 7 days (criterion requires ≤14). |

## Quality

| Criterion | Answer | Justification to paste |
|---|---|---|
| build | Met | Standard Go toolchain: `go build ./...`; Makefile targets for the full pipeline. |
| build_common_tools | Met | go, make — ubiquitous tools. |
| build_floss_tools | Met | The entire toolchain (Go, golangci-lint, easyjson) is FLOSS. |
| test | Met | Comprehensive `go test` suite incl. a JSON-RPC 2.0 spec-conformance file (spec_test.go), fuzz targets, and race-enabled runs. |
| test_invocation | Met | `make test` / `go test ./...`; documented in CONTRIBUTING.md. |
| test_most | Met | Core dispatcher coverage is tracked in CI (>90%); transports have end-to-end suites. |
| test_continuous_integration | Met | GitHub Actions on every push/PR: test matrix (two Go versions), -race, lint, govulncheck, CodeQL, nested modules. https://github.com/gumeniukcom/golang-jsonrpc2/actions |
| test_policy | Met | CONTRIBUTING.md "Requirements for changes" #1: new functionality must add tests; bug fixes must add a regression test. |
| tests_are_added | Met | Recent history demonstrates it — e.g. the v2.6.0 visibility change shipped with default-deny, filter, and deep-copy tests. |
| tests_documented_added | Met | Stated in CONTRIBUTING.md. |
| warnings | Met | golangci-lint (config in repo) + go vet in CI; gofmt/goimports enforced. |
| warnings_fixed | Met | CI is red on any lint warning; current state is 0 issues. |
| warnings_strict | Met | Lint failures block CI. |

## Security

| Criterion | Answer | Justification to paste |
|---|---|---|
| know_secure_design | Met | The maintainer applies secure-design principles; see docs/production.md (least privilege, fail-closed defaults, input distrust) and the default-deny discovery design in v2.6.0. |
| know_common_errors | Met | Common error classes (injection via untrusted method names, DoS via unbounded inputs, information leaks in error text) are documented and mitigated: docs/production.md, docs/observability.md. |
| no_leaked_credentials | Met | No credentials in the repository; CI uses the ephemeral GITHUB_TOKEN with least-privilege permissions. |
| delivery_mitm | Met | Distribution is via Go modules: HTTPS + the Go checksum database (sum.golang.org) cryptographically verifies module contents. |
| delivery_unsigned | Met | Go module checksums (go.sum + sumdb) protect against tampering; hashes are not delivered over an unprotected channel. |
| vulnerabilities_fixed_60_days | Met | No known unfixed vulnerabilities of medium+ severity in the project's code; dependency advisories are handled via weekly govulncheck CI and Dependabot (all current). |
| vulnerabilities_critical_fixed | Met | None outstanding. |
| crypto_published | N/A | The project implements no cryptography; TLS, when used, comes from the platform (Go stdlib) configured by the embedding application. |
| crypto_call | N/A | Same — no crypto functionality of its own. |
| crypto_floss | N/A | Same. |
| crypto_keylength | N/A | Same. |
| crypto_working | N/A | Same. |
| crypto_weaknesses | N/A | Same. |
| crypto_pfs | N/A | Same. |
| crypto_password_storage | N/A | The project stores no passwords. |
| crypto_random | N/A | No cryptographic randomness needed; request ids use google/uuid where applicable, not for security. |

## Analysis

| Criterion | Answer | Justification to paste |
|---|---|---|
| static_analysis | Met | CodeQL (weekly + every push/PR), golangci-lint, go vet — all in CI. https://github.com/gumeniukcom/golang-jsonrpc2/blob/master/.github/workflows/codeql.yml |
| static_analysis_common_vulnerabilities | Met | CodeQL's Go security queries and govulncheck (known-vulnerability reachability analysis, weekly + per-PR). |
| static_analysis_fixed | Met | Findings are fixed promptly; CI blocks on lint, and code-scanning alerts are triaged. |
| static_analysis_often | Met | Every push and PR. |
| dynamic_analysis | Met | Native Go fuzzing (three fuzz targets for the wire-framing parsers, run in CI via their seed corpora) and the race detector on the full suite. |
| dynamic_analysis_unsafe | Met | `go test -race ./...` runs in CI on every push (memory-safety analysis appropriate to Go). |
| dynamic_analysis_fixed | Met | No outstanding dynamic-analysis findings; race/fuzz failures block CI. |
| dynamic_analysis_enable_assertions | N/A | Go has no assertion mechanism to enable; test invariants serve this role. |

## After submitting

- The badge markdown for the README (once at least "in progress"):
  `[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/<ID>/badge)](https://www.bestpractices.dev/projects/<ID>)`
  — replace `<ID>` with the assigned project id, and add it to the README
  badge row.
- Scorecard's CII-Best-Practices check scores 2/10 for "in progress",
  5/10 for "passing"; the entry form saves drafts, so submitting partially
  and finishing later is fine.
- Keep this file in sync when processes change (it is referenced from the
  release checklist habit: review at minor releases alongside COMPARISON).
