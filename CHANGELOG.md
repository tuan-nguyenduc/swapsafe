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
