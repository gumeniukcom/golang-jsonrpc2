# OpenSSF Best Practices badge — questionnaire draft (passing level)

> Internal engineering doc. The project is registered as
> **<https://www.bestpractices.dev/en/projects/13570>**. Answers are for
> the **passing** level as of 2026-07-11.
>
> **Fastest path**: open each prefill URL below (they use the badge app's
> URL-prefill mechanism), review the filled fields, press **Submit** —
> one section per URL. The tables further down hold the same content for
> manual editing.

## Prefill URLs (open → review → Submit)

- **basics**:
  <https://www.bestpractices.dev/en/projects/13570/passing/edit?contribution_status=Met&contribution_justification=https%3A%2F%2Fgithub.com%2Fgumeniukcom%2Fgolang-jsonrpc2%2Fblob%2Fmaster%2FCONTRIBUTING.md+describes+the+process%3A+GitHub+issues+for+reports%2C+pull+requests+against+master+for+code.&contribution_requirements_status=Met&contribution_requirements_justification=CONTRIBUTING.md+%27Requirements+for+changes%27%3A+tests+required+for+functionality+changes%2C+full+check+suite+%28make+all%29+must+pass%2C+gofmt%2Fgolangci-lint+style%2C+Conventional+Commits.+https%3A%2F%2Fgithub.com%2Fgumeniukcom%2Fgolang-jsonrpc2%2Fblob%2Fmaster%2FCONTRIBUTING.md&documentation_interface_status=Met&documentation_interface_justification=Full+API+reference+generated+from+doc+comments%2C+with+runnable+examples%2C+on+https%3A%2F%2Fpkg.go.dev%2Fgithub.com%2Fgumeniukcom%2Fgolang-jsonrpc2%2Fv2&english_status=Met&english_justification=All+documentation%2C+code+comments%2C+and+issue+discussion+are+in+English.&maintained_status=Met&maintained_justification=Actively+maintained%3A+30%2B+commits+and+three+releases+in+the+last+90+days.+https%3A%2F%2Fgithub.com%2Fgumeniukcom%2Fgolang-jsonrpc2%2Freleases>
- **changecontrol**:
  <https://www.bestpractices.dev/en/projects/13570/passing/edit?repo_interim_status=Met&repo_interim_justification=Development+happens+on+master+with+interim+commits+between+releases%3B+the+full+history+is+public.&version_unique_status=Met&version_unique_justification=Every+release+has+a+unique+SemVer+git+tag+%28v2.x.y%29%2C+plus+nested-module+tags+for+adapters.&version_semver_status=Met&version_semver_justification=Semantic+Versioning+for+the+%2Fv2+Go+module%3B+documented+in+the+README+%27Versioning%27+section.&version_tags_status=Met&version_tags_justification=Releases+are+git+tags%3A+https%3A%2F%2Fgithub.com%2Fgumeniukcom%2Fgolang-jsonrpc2%2Ftags&release_notes_vulns_status=N%2FA&release_notes_vulns_justification=No+publicly+known+vulnerabilities+have+existed+in+this+project%27s+code%3B+when+one+is+fixed%2C+its+advisory+ID+will+be+listed+in+CHANGELOG.md+and+the+GitHub+release+notes.>
- **reporting**:
  <https://www.bestpractices.dev/en/projects/13570/passing/edit?report_tracker_status=Met&report_tracker_justification=GitHub+issue+tracker%3A+https%3A%2F%2Fgithub.com%2Fgumeniukcom%2Fgolang-jsonrpc2%2Fissues&report_responses_status=Met&report_responses_justification=The+maintainer+responds+to+bug+reports%3B+the+tracker+history+shows+responses+to+all+reports+in+the+period.&enhancement_responses_status=Met&enhancement_responses_justification=Enhancement+requests+are+acknowledged+in+the+issue+tracker.&report_archive_status=Met&report_archive_justification=Issues+and+pull+requests+are+permanently+archived+and+searchable%3A+https%3A%2F%2Fgithub.com%2Fgumeniukcom%2Fgolang-jsonrpc2%2Fissues&vulnerability_report_process_status=Met&vulnerability_report_process_justification=Published+in+SECURITY.md%3A+https%3A%2F%2Fgithub.com%2Fgumeniukcom%2Fgolang-jsonrpc2%2Fblob%2Fmaster%2FSECURITY.md&vulnerability_report_private_status=Met&vulnerability_report_private_justification=GitHub+private+vulnerability+reporting+is+enabled%3B+SECURITY.md+links+the+private+advisory+form%3A+https%3A%2F%2Fgithub.com%2Fgumeniukcom%2Fgolang-jsonrpc2%2Fsecurity%2Fadvisories&vulnerability_report_response_status=Met&vulnerability_report_response_justification=SECURITY.md+commits+to+acknowledgement+within+7+days+%28criterion+allows+14%29.+No+vulnerability+reports+were+received+in+the+last+6+months.>
- **quality**:
  <https://www.bestpractices.dev/en/projects/13570/passing/edit?build_floss_tools_status=Met&build_floss_tools_justification=The+entire+toolchain+is+FLOSS%3A+Go%2C+make%2C+golangci-lint%2C+easyjson.&test_status=Met&test_justification=Comprehensive+go+test+suite+including+a+JSON-RPC+2.0+spec-conformance+file+%28spec_test.go%29%2C+fuzz+targets%2C+and+race-enabled+runs%3B+invocation+documented+in+CONTRIBUTING.md.+https%3A%2F%2Fgithub.com%2Fgumeniukcom%2Fgolang-jsonrpc2%2Fblob%2Fmaster%2FCONTRIBUTING.md&test_invocation_status=Met&test_invocation_justification=Standard+for+the+language%3A+go+test+.%2F...+%28also+make+test+%2F+make+testrace%29.&test_most_status=Met&test_most_justification=Core+dispatcher+coverage+above+90+percent+is+printed+in+CI%3B+transports+have+end-to-end+suites+over+real+pipes%2Fsockets.&test_continuous_integration_status=Met&test_continuous_integration_justification=GitHub+Actions+on+every+push+and+PR%3A+test+matrix+on+two+Go+versions%2C+-race%2C+lint%2C+govulncheck%2C+CodeQL%2C+nested+adapter+modules.+https%3A%2F%2Fgithub.com%2Fgumeniukcom%2Fgolang-jsonrpc2%2Factions&test_policy_status=Met&test_policy_justification=CONTRIBUTING.md+%27Requirements+for+changes%27+%231%3A+new+functionality+must+add+tests%3B+bug+fixes+must+include+a+regression+test.&tests_are_added_status=Met&tests_are_added_justification=Recent+major+changes+demonstrate+the+policy+-+e.g.+the+v2.6.0+discovery-visibility+change+shipped+with+default-deny%2C+filter%2C+and+deep-copy+tests%3B+the+v2.4.0+stdio+transport+shipped+with+framing+golden+tests+and+fuzz+targets.&tests_documented_added_status=Met&tests_documented_added_justification=Documented+in+CONTRIBUTING.md%3A+https%3A%2F%2Fgithub.com%2Fgumeniukcom%2Fgolang-jsonrpc2%2Fblob%2Fmaster%2FCONTRIBUTING.md&warnings_status=Met&warnings_justification=golangci-lint+%28config+in-repo%29+and+go+vet+run+in+CI%3B+gofmt%2Fgoimports+formatting+is+enforced.&warnings_fixed_status=Met&warnings_fixed_justification=CI+fails+on+any+lint+warning%3B+the+current+state+is+0+issues.&warnings_strict_status=Met&warnings_strict_justification=Lint+failures+are+blocking+in+CI%3B+the+linter+config+enables+additional+analyzers+beyond+defaults.>
- **security**:
  <https://www.bestpractices.dev/en/projects/13570/passing/edit?know_secure_design_status=Met&know_secure_design_justification=The+primary+developer+applies+secure-design+principles+%28least+privilege%2C+fail-closed+defaults%2C+distrust+of+input%29+-+demonstrated+by+the+documented+hardening+guide+https%3A%2F%2Fgithub.com%2Fgumeniukcom%2Fgolang-jsonrpc2%2Fblob%2Fmaster%2Fdocs%2Fproduction.md+and+the+default-deny+service-discovery+design.&know_common_errors_status=Met&know_common_errors_justification=Common+error+classes+for+this+kind+of+software+%28DoS+via+unbounded+inputs%2C+injection+via+untrusted+method+names%2C+information+leaks+in+error+text%29+and+their+mitigations+are+documented%3A+https%3A%2F%2Fgithub.com%2Fgumeniukcom%2Fgolang-jsonrpc2%2Fblob%2Fmaster%2Fdocs%2Fproduction.md&crypto_published_status=N%2FA&crypto_published_justification=The+project+implements+no+cryptography+of+its+own%3B+TLS%2C+when+used%2C+comes+from+the+Go+standard+library+configured+by+the+embedding+application.&crypto_call_status=N%2FA&crypto_call_justification=The+project+implements+no+cryptography+of+its+own%3B+TLS%2C+when+used%2C+comes+from+the+Go+standard+library+configured+by+the+embedding+application.&crypto_floss_status=N%2FA&crypto_floss_justification=The+project+implements+no+cryptography+of+its+own%3B+TLS%2C+when+used%2C+comes+from+the+Go+standard+library+configured+by+the+embedding+application.&crypto_keylength_status=N%2FA&crypto_keylength_justification=The+project+implements+no+cryptography+of+its+own%3B+TLS%2C+when+used%2C+comes+from+the+Go+standard+library+configured+by+the+embedding+application.&crypto_working_status=N%2FA&crypto_working_justification=The+project+implements+no+cryptography+of+its+own%3B+TLS%2C+when+used%2C+comes+from+the+Go+standard+library+configured+by+the+embedding+application.&crypto_weaknesses_status=N%2FA&crypto_weaknesses_justification=The+project+implements+no+cryptography+of+its+own%3B+TLS%2C+when+used%2C+comes+from+the+Go+standard+library+configured+by+the+embedding+application.&crypto_pfs_status=N%2FA&crypto_pfs_justification=The+project+implements+no+cryptography+of+its+own%3B+TLS%2C+when+used%2C+comes+from+the+Go+standard+library+configured+by+the+embedding+application.&crypto_password_storage_status=N%2FA&crypto_password_storage_justification=The+project+stores+no+passwords.&crypto_random_status=N%2FA&crypto_random_justification=No+cryptographic+keys+or+nonces+are+generated+by+the+project.&delivery_unsigned_status=Met&delivery_unsigned_justification=Distribution+is+via+Go+modules%3A+module+contents+are+verified+against+the+Go+checksum+database+%28sum.golang.org%29+over+HTTPS%3B+no+hashes+are+retrieved+over+http.&vulnerabilities_fixed_60_days_status=Met&vulnerabilities_fixed_60_days_justification=No+unpatched+publicly+known+vulnerabilities+in+the+project%27s+code%3B+dependency+advisories+are+monitored+by+weekly+govulncheck+CI+runs+and+Dependabot+across+all+modules.&vulnerabilities_critical_fixed_status=Met&vulnerabilities_critical_fixed_justification=None+have+been+reported%3B+SECURITY.md+commits+to+rapid+handling.&no_leaked_credentials_status=Met&no_leaked_credentials_justification=No+credentials+in+the+repository%3B+CI+uses+the+ephemeral+least-privilege+GITHUB_TOKEN+only.>
- **analysis**:
  <https://www.bestpractices.dev/en/projects/13570/passing/edit?static_analysis_status=Met&static_analysis_justification=CodeQL+runs+on+every+push%2FPR+and+weekly%2C+alongside+golangci-lint+and+go+vet%3A+https%3A%2F%2Fgithub.com%2Fgumeniukcom%2Fgolang-jsonrpc2%2Fblob%2Fmaster%2F.github%2Fworkflows%2Fcodeql.yml&static_analysis_common_vulnerabilities_status=Met&static_analysis_common_vulnerabilities_justification=CodeQL%27s+Go+security+queries+plus+govulncheck+%28known-vulnerability+reachability+analysis%29.&static_analysis_fixed_status=Met&static_analysis_fixed_justification=Lint+findings+block+CI%3B+code-scanning+alerts+are+triaged+promptly%3B+none+outstanding.&static_analysis_often_status=Met&static_analysis_often_justification=On+every+commit+and+pull+request.&dynamic_analysis_status=Met&dynamic_analysis_justification=Native+Go+fuzzing+-+three+fuzz+targets+for+the+wire-framing+parsers%2C+exercised+in+CI+via+their+seed+corpora+-+plus+the+race+detector+across+the+full+suite.&dynamic_analysis_unsafe_status=N%2FA&dynamic_analysis_unsafe_justification=The+project+is+written+entirely+in+Go%2C+a+memory-safe+language.&dynamic_analysis_enable_assertions_status=Met&dynamic_analysis_enable_assertions_justification=The+fuzz+targets+and+tests+assert+invariants+during+dynamic+analysis+%28size+limits+respected%2C+no+panics%2C+write%2Fread+round-trip+identity%29%3B+Go+has+no+separate+assertion+mode+to+enable.&dynamic_analysis_fixed_status=Met&dynamic_analysis_fixed_justification=Race+or+fuzz+failures+block+CI%3B+none+outstanding.>

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

- Badge markdown for the README (add once the percentage is respectable):
  `[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/13570/badge)](https://www.bestpractices.dev/projects/13570)`
- Scorecard's CII-Best-Practices check scores 2/10 for "in progress",
  5/10 for "passing"; the entry form saves drafts, so submitting partially
  and finishing later is fine.
- Keep this file in sync when processes change (it is referenced from the
  release checklist habit: review at minor releases alongside COMPARISON).
