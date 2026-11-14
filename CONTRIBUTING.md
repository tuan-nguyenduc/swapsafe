# Contributing to swapsafe

Thanks for helping out. swapsafe is early — the fastest way to contribute is to pick up a
`good first issue`, most of which are "add a rule for annotation X".

## Before you start

Open an issue first for anything beyond a small fix. swapsafe has a deliberately narrow scope —
it verifies behaviour, it does not translate configuration, change traffic, or load test — and
the maintainers will tell you quickly whether an idea fits.

## Development environment

You need Go (the version in `go.mod`), `make`, and — for the end-to-end suite — Docker and `kind`.

```console
$ make build       # binary into bin/
$ make test        # unit tests with -race
$ make lint        # golangci-lint
$ make help        # everything else
```

## Writing a rule

Each ingress-nginx annotation is one `Rule` in `rules/ingressnginx/`: one `.go` file and one
`testdata/` directory. A rule is mergeable when its testdata contains all four of:

- `ingress.yaml` — the source Ingress carrying the annotation
- `httproute-ok.yaml` — a correct Gateway API translation
- `httproute-bad.yaml` — a translation that changes behaviour
- `expected-findings.json` — what swapsafe must report for the bad one

Rules may only import `pkg/rule` and `pkg/api`; `golangci-lint` enforces this.

## Pull requests

- One task per PR, small and reviewable.
- Commits follow [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`,
  `docs:`, `ci:`, `build:`, `test:`, `refactor:`, `chore:`). The changelog is generated from them.
- Every commit needs a DCO sign-off: `git commit -s`. There is no CLA.
- CI must be green: lint, `test -race` on Linux and macOS, `govulncheck`, `zizmor`.
- Update the `Unreleased` section of `CHANGELOG.md` and any user-facing docs.
- New dependencies must be justified in the PR description. The standard library comes first.

## Reporting bugs and asking questions

Bugs and feature requests go to Issues; open-ended questions to Discussions. Security
vulnerabilities go through the process in [`SECURITY.md`](SECURITY.md) — never a public issue.
