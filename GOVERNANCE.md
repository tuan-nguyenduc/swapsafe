# Governance

swapsafe follows a bounded BDFL model while it has a single maintainer, and moves to lazy
consensus as the maintainer group grows. The structure is deliberately CNCF-shaped so the project
can be proposed to the CNCF Sandbox later without a governance rewrite.

## Roles

**Contributor** — anyone who opens an issue, a PR or answers a question.

**Maintainer** — merge rights and release rights. Maintainers are listed in
[`MAINTAINERS.md`](MAINTAINERS.md).

## Becoming a maintainer

A contributor is eligible after **5 meaningful merged PRs** and **3 months** of sustained
activity. An existing maintainer nominates them in a public issue; the nomination carries if
nobody objects within **7 days**.

## Decision making

- **One maintainer (today):** that maintainer decides, in public, with reasons recorded in the
  issue or in an ADR under `docs/adr/`.
- **Three or more maintainers:** lazy consensus. A proposal that attracts no objection within
  7 days is accepted. Changes to this document require a **two-thirds** vote of maintainers.

## Substantial changes

A new public API, a new provider, or any backwards-incompatible change goes through an RFC: an
issue labelled `proposal`, open for comment for at least **7 days**, with the outcome written up
as an ADR.

## Code of conduct

Everyone is bound by [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md). Enforcement is the maintainers'
responsibility.
