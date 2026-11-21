# Changelog

All notable changes to this project are documented here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- The body stability check no longer flaps. It took two baseline samples and called the body
  stable if they matched, so a field that moves slowly — a goroutine count, an uptime — slipped
  through and the same cluster reported a `major` body difference on some runs and not others.
  There are now three samples across two time scales, and the candidate probe sits between two
  of them.
- `rewrite-target` also probes the route path itself, which a prefix-stripping rewrite turns
  into the backend root. Probing only below the route landed on a path that does not exist,
  where many backends answer the same 404 whatever the rewrite did.

### Added

- JSON bodies are compared field by field. The fields the baseline changes on its own are left
  out by name and everything else is still compared, so a counter no longer hides a real
  difference elsewhere in the same document. `Observation.bodyIgnoredFields` records what was
  left out. Non-JSON bodies still fall back to being reported as not comparable.

## [0.1.2] - 2026-11-21

Three false positives, all found on the first run against a real cluster rather than against
fixtures. Each one would have made a correct migration look broken.

### Fixed

- An Ingress whose path is a regular expression — `use-regex` with the prefix-strip idiom,
  `/podinfo(/|$)(.*)` — is no longer reported as having no HTTPRoute. Pairing now compares
  coverage rather than strings: a `PathPrefix` pairs with any route it serves, and where
  several cover one route the most specific wins, the way Gateway API itself routes.
  Found running v0.1.1 against a real cluster, where it produced a `critical` finding that
  failed CI for a correct migration.
- A regex Ingress path is no longer probed verbatim by any rule. `/podinfo(/|$)(.*)` is not a
  URL anyone can request, so the probes 404ed on both sides and proved nothing; every rule now
  probes the literal part of the path.
- A body that the baseline does not return twice — a counter, a timestamp, a pod name — is no
  longer reported as a `major` difference blamed on a different backend. When two bodies differ,
  the baseline is probed once more; if it disagrees with itself, the body is reported as `info`
  and not compared. A run where every body matches sends no extra requests at all.
  Found running v0.1.1 against podinfo on a real cluster.

### Added

- A "no HTTPRoute" finding is now confirmed by the probes. When both endpoints answer the route
  identically, something on the Gateway side is serving it and the finding drops to `info`
  saying so; when the candidate answers differently, the finding stays `critical` and is now
  backed by evidence rather than by static pairing alone.
- `Observation.bodyUnstable` in the v1alpha1 schema, set when the baseline answered the same
  request with two different bodies.
- End-to-end fixtures for the prefix-strip idiom and for a route whose body carries a request
  id. Against a real ingress-nginx and a real Envoy Gateway, both are now silent and `info`
  respectively; before this release the first was `critical` and the second `major`.

## [0.1.1] - 2026-11-15

### Added

- macOS binaries, amd64 and arm64. The persona that runs this against their own kubeconfig is
  on a laptop; only Windows was ruled out (PLAN section 9), and macOS went with it by mistake.
  Not signed or notarized yet, so download with `curl` rather than a browser.

### Fixed

- README code blocks no longer carry a `$` prompt, which GitHub's copy button included and
  every shell then rejected.

## [0.1.0] - 2026-11-15

First release. swapsafe compares an ingress-nginx endpoint with a Gateway API endpoint and
reports every difference that matters, attributed to the annotation behind it.

### Security

- Raised the Go floor to 1.26.11 and `golang.org/x/net` to v0.62.0, clearing six advisories
  `govulncheck` reported in the HTTP/2 stack.

### Changed

- `--baseline` and `--candidate` accept a bare host or `host:port`, not only a URL. The scheme
  was already ignored; now it does not have to be invented. README explains why: the scheme of
  each request comes from the rule, so checking `ssl-redirect` sends plain HTTP to an endpoint
  written as `https://`.

- Table cells are truncated at 32 characters rather than 48, and a route with no counterpart
  reports `Ingress <ns>/<name>` against `no HTTPRoute`. With six columns on screen a wide one
  wraps the row, and a wrapped row reads as noise. The full text is in the hint and the json.

### Fixed

- `Connection`, `Keep-Alive` and the rest of the hop-by-hop headers are now dropped before
  comparing. ingress-nginx sends `Connection: keep-alive` where Envoy sends nothing, which made
  every correctly translated route look broken.
- The ingress-nginx rules are registered by `internal/cli` rather than by `main`, so anything
  driving the command tree has them. Without this the end-to-end suite ran with no rules at all.

### Added

- `.goreleaser.yaml` and a tag-triggered release workflow: Linux amd64 and arm64 archives with
  checksums, built reproducibly from the commit timestamp.
- `hack/demo/`: two fake endpoints and a manifest, so swapsafe can be tried without a cluster.
- README demo GIF, recorded from a real run by `hack/demo.tape` (vhs).
- Colour in the table output: severity by level, the absent marker dimmed, `MISSING` in red.
  Only on a terminal, and never when `NO_COLOR` is set.
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
- README: what swapsafe does, quickstart, the safety defaults, and the supported annotations.
- End-to-end suite: `hack/kind-up.sh` stands up kind with ingress-nginx and Envoy Gateway serving
  the same hostnames, `make e2e` runs swapsafe against both and compares to `test/golden/`.
  Four fixture routes, three mistranslated on purpose and one correct.
- `pkg/api/v1alpha1`: the core types — `RouteSpec`, `TestCase`, `Observation`, `Finding`,
  `Severity` — carrying the JSON tags that define `results.json`
  (`swapsafe.dev/v1alpha1`), with round-trip and determinism tests.
- `internal/collector`: read Ingress and HTTPRoute objects from multi-document YAML manifests and
  pair them by namespace, host and path. An Ingress route with no matching HTTPRoute becomes a
  `critical` finding.
- `pkg/rule`: the `Rule` interface, the registry rules register themselves into from `init()`,
  a stable `TestCaseID` hash, and `BaselineRule` — a GET of the route path, the same path with
  the trailing slash flipped, and a sub-path, over HTTP and (for TLS routes) HTTPS.
- `internal/generator`: collects the test cases every applicable rule wants for a route,
  drops duplicate IDs, and reports each `nginx.ingress.kubernetes.io/` annotation no rule
  verifies as an `info` finding rather than passing over it in silence.
- `internal/prober`: sends each test case to both endpoints in parallel, dialling the endpoint
  address while presenting the route's Host header and SNI, so a Gateway can be probed before
  DNS moves. Redirects are not followed, the rate is capped per endpoint, unsafe methods are
  refused without an explicit flag, and response bodies are hashed but not kept.
- `internal/normalize`: drops per-request headers (`Date`, `X-Request-Id`, `Server`, `Via`,
  `X-Envoy-*`, `Traceparent`, `Content-Length`) plus anything in `ignoreHeaders`, masks
  credential headers, resolves `Location` to an absolute canonical URL, and reduces
  `Set-Cookie` to its name and behavioural attributes with the value redacted.
- `internal/differ`: compares two normalized observations and assigns severity — crossing a
  status class is `critical`, a redirect target or a header only one side sends is `major`,
  a differing value inside the same status class is `minor`.
- `internal/attribution`: names the annotation behind each finding and fills in the rule's hint.
- `internal/report`: `table` and `json` output. The table is sorted most-severe-first and names
  the request behind each row; hints are printed once however many findings share them.
- `swapsafe scan`: lists the routes, where each one is meant to land, and which rules would
  probe it. Sends no requests.
- `swapsafe run`: the full pipeline, with `--baseline`, `--candidate`, `--ingress-file`,
  `--httproute-file`, `--output`, `--fail-on`, `--concurrency`, `--rate`, `--timeout`,
  `--insecure-skip-verify`, `--allow-unsafe-methods` and `--include-bodies`.
  Exit code 0 when nothing reaches `--fail-on` (default `critical`), 1 on failure, 2 on findings.
- `rules/ingressnginx`: the ten v0.1 rules — `ssl-redirect`, `force-ssl-redirect`,
  `rewrite-target`, `use-regex`, `app-root`, `permanent-redirect`, `temporal-redirect`,
  `enable-cors` (with the `cors-*` family), `proxy-body-size` and `backend-protocol`.
- `rule.Reporter`: an optional interface for rules whose annotation cannot be verified by
  probing, so they report what they know instead of staying silent.
- `swapsafe rules`: lists every rule and the annotations it verifies.
- `--from-cluster`: read Ingress and HTTPRoute objects from a live cluster, filtered by
  `--namespace`, `--selector` and `--ingress-class`, with `--kubeconfig` and `--context`.
  swapsafe only ever lists; `get` and `list` on Ingress and HTTPRoute is all it needs.
- The generator now sends each distinct request once per route however many rules ask for it,
  and gives the request to the rule that knows why it is asking rather than to the baseline.
