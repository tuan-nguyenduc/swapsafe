# Security Policy

## Supported versions

swapsafe is pre-1.0. Only the latest released version receives security fixes.

## Reporting a vulnerability

Report privately through GitHub Private Vulnerability Reporting:
**Security → Report a vulnerability** on
<https://github.com/tuan-nguyenduc/swapsafe/security/advisories/new>.

Please do not open a public issue for a security problem.

We acknowledge every report within **72 hours** and aim to ship a fix or publish a mitigation
within 90 days, crediting the reporter unless they prefer otherwise.

## Scope notes

swapsafe is a read-only client. It needs `get`/`list` on Ingress, HTTPRoute and Gateway and
never writes to a cluster. Reports we are particularly interested in:

- any path where swapsafe writes to a cluster, modifies DNS, or sends a non-idempotent request
  without `--allow-unsafe-methods`
- leakage of response bodies, cookie values or `Authorization` headers into reports or logs
- supply-chain weaknesses in our release pipeline (signing, provenance, SBOM)
