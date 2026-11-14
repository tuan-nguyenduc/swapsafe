# swapsafe — instructions for Claude Code

## Project
A Go CLI that verifies Gateway API reproduces ingress-nginx behaviour.
Full plan: docs/PLAN.md (local only, gitignored). Architecture decisions: docs/adr/.

## Commands
- make build | make test | make lint | make e2e | make golden
- Run a single test: go test ./internal/differ -run TestName -race

## Rules
- Do only the assigned task (IDs look like M1-T3). Do not widen the scope.
- Read the relevant section of docs/PLAN.md before writing code.
- Respect the package dependency rules in PLAN section 4.
- Every I/O function takes context.Context. Wrap errors with %w. No panic outside main.
- Write tests before or alongside the code; table-driven; run with -race.
- A new rule needs testdata: ingress.yaml, httproute-ok.yaml,
  httproute-bad.yaml, expected-findings.json.
- Do not add a dependency without asking first.
- Do not change the JSON output in pkg/api without updating the golden files
  and the CHANGELOG.
- Pin every GitHub Action to a full commit SHA.
- Never log a response body or a cookie value.

## Finishing a task
1. make lint test is green.
2. Update the Unreleased section of CHANGELOG.md.
3. Update the docs if user-visible behaviour changed.
4. Commit with Conventional Commits and a sign-off (-s).
5. Summarise: what was done, what was not, what risk remains.
