# Changelog

All notable changes to this project are documented here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Repository skeleton: Go module, directory layout, Apache-2.0 license, README, `CLAUDE.md`.
- `Makefile` with `build`, `test`, `lint`, `fmt`, `e2e`, `golden`, `snapshot` and a self-documenting `help`.
- `.golangci.yml` enforcing the linter set and the package dependency rules from PLAN section 4.
- CI workflow (`ci.yml`): lint, `test -race` and build on Linux and macOS, plus `govulncheck`
  and a `zizmor` workflow audit. All actions pinned to full commit SHAs; `permissions: contents: read`.
- Community files: `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md` (Contributor Covenant 2.1),
  `SECURITY.md`, `GOVERNANCE.md`, `MAINTAINERS.md`, issue forms, PR template, `CODEOWNERS`.
- cobra command skeleton with the `version` command (version, commit, build date via `-ldflags`)
  and global `--log-level` / `--log-format` flags backed by `log/slog`.
- Dependabot for `gomod` and `github-actions` (PLAN section 6.3).
- `internal/model`: the core types — `RouteSpec`, `TestCase`, `Observation`, `Finding` — plus
  `Severity` with `--fail-on` ordering.
- `pkg/api/v1alpha1`: the public JSON schema (`swapsafe.dev/v1alpha1`) for `results.json`,
  with conversions from the internal model and round-trip tests.
- `internal/collector`: read Ingress and HTTPRoute objects from multi-document YAML manifests and
  pair them by namespace, host and path. An Ingress route with no matching HTTPRoute becomes a
  `critical` finding.
- `pkg/rule`: the `Rule` interface, the registry rules register themselves into from `init()`,
  a stable `TestCaseID` hash, and `BaselineRule` — a GET of the route path, the same path with
  the trailing slash flipped, and a sub-path, over HTTP and (for TLS routes) HTTPS.
