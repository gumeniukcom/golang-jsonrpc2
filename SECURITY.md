# Security Policy

## Supported versions

Only the latest release of the `v2` module line receives security fixes.

| Version | Supported |
|---|---|
| latest `v2.x` | yes |
| older `v2.x` | no — upgrade |
| `v1` (unversioned module path) | no |

## Reporting a vulnerability

Please report vulnerabilities **privately** via
[GitHub Security Advisories](https://github.com/gumeniukcom/golang-jsonrpc2/security/advisories/new)
("Report a vulnerability"). Do not open a public issue for security problems.

You can expect an acknowledgement within **7 days** and a fix or a public
advisory within **90 days** of the report. Credit is given in the advisory
unless you prefer otherwise.

## Scope notes

- The core module and the transport subpackages (`jsonrpchttp`,
  `jsonrpcws`, `jsonrpcstdio`) and nested adapter modules (`jsonrpcfiber`,
  `jsonrpcfiberv3`) are in scope.
- The `benchmarks/` module is internal dev tooling (never tagged, not
  consumable) — dependency reports against it are still welcome but are
  handled as routine maintenance, not advisories.
- The security posture users should rely on (limits, defaults, trust
  models per transport) is documented in
  [docs/production.md](docs/production.md).
