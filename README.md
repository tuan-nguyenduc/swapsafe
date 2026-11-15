# swapsafe

[![ci](https://github.com/tuan-nguyenduc/swapsafe/actions/workflows/ci.yml/badge.svg)](https://github.com/tuan-nguyenduc/swapsafe/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)

**Verify that your Gateway API setup reproduces ingress-nginx behaviour — before you move traffic.**

ingress-nginx is end of life. [ingress2gateway](https://github.com/kubernetes-sigs/ingress2gateway)
translates the configuration, and its own documentation tells you to review and fix the output by
hand. Nothing checks the *runtime* behaviour. swapsafe is that check.

It sends the same requests to your ingress-nginx endpoint and to your new Gateway endpoint,
normalizes both answers, and reports every difference that matters — with the ingress-nginx
annotation that caused it, and what to change.

> **Status: pre-alpha, no release yet.** `scan`, `run` and `rules` work, against YAML manifests
> or a live cluster, and the end-to-end suite runs against a real ingress-nginx and a real Envoy
> Gateway. There is no release yet. Expect breaking changes.

## Install

```console
$ git clone https://github.com/tuan-nguyenduc/swapsafe && cd swapsafe
$ make build        # -> bin/swapsafe
```

Go 1.26 or newer. No release binaries yet.

## Quickstart

Save your Ingress and HTTPRoute manifests — one file or two, many documents each, other kinds
ignored:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: web
  namespace: shop
  annotations:
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
spec:
  rules:
    - host: shop.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend: {service: {name: web, port: {number: 80}}}
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata: {name: web, namespace: shop}
spec:
  parentRefs: [{name: gateway}]
  hostnames: [shop.example.com]
  rules:
    - matches: [{path: {type: PathPrefix, value: /}}]
      backendRefs: [{name: web, port: 80}]
```

See what would be checked, without sending anything:

```console
$ swapsafe scan --ingress-file manifests.yaml --httproute-file manifests.yaml
ROUTE                       TARGET              RULES
shop/web/shop.example.com/  HTTPRoute shop/web  baseline, nginx.ssl-redirect

1 routes, nothing to flag before probing.
```

Or read the objects straight from a cluster:

```console
$ swapsafe scan --from-cluster --namespace shop --ingress-class nginx
```

swapsafe only ever lists. A kubeconfig with `get` and `list` on Ingress and HTTPRoute is all it
needs, and all it should be given.

Then probe both endpoints:

```console
$ swapsafe run \
    --baseline  https://203.0.113.10 \
    --candidate https://203.0.113.20 \
    --ingress-file manifests.yaml --httproute-file manifests.yaml

SEVERITY  ROUTE                       REQUEST     ASPECT           BASELINE                   CANDIDATE
critical  shop/web/shop.example.com/  GET http /  status           308                        200
major     shop/web/shop.example.com/  GET http /  header:location  https://shop.example.com/  (absent)

2 findings: 1 critical, 1 major.

Hints:
  1. ingress-nginx redirects plain HTTP to HTTPS itself. Gateway API has no equivalent
     annotation: add a second listener on port 80 with an HTTPRoute carrying a
     RequestRedirect filter (scheme: https, statusCode: 301).
```

Exit code `0` when nothing reaches `--fail-on` (default `critical`), `1` when swapsafe could not
finish, `2` when it finished and found something. That is the whole CI integration.

## Safe by default

swapsafe is built to be pointed at production.

- **Read only.** It never applies a manifest, never touches DNS, never writes to a cluster.
- **Safe methods only.** GET, HEAD and OPTIONS. Anything else needs `--allow-unsafe-methods`.
- **Gentle.** 20 requests per second per endpoint and 8 in flight by default.
- **Quiet about secrets.** Response bodies are hashed, not kept, unless you pass
  `--include-bodies`. Cookie values and credential headers are redacted, always.
- **Works before DNS moves.** It connects to the endpoint address you give it while presenting
  each route's own hostname as the Host header and the SNI name, so the Gateway routes the
  request as it will in production.

## How it works

```
manifests ─→ collector ─→ generator ─→ prober ─→ normalize ─→ differ ─→ report
                 │            │          │           │          │
            pair Ingress   ask every   send to    drop the   compare and
            with HTTPRoute  rule what  both ends  noise      rank, then
                            to probe                         attribute
```

The rule registry is the only place that knows about annotations. It decides what to probe and
how severe a difference is, and it supplies the hint. Adding support for an annotation is adding
one file under `rules/ingressnginx/`.

Normalization is what keeps the report trustworthy: `Date`, `Server`, `X-Request-Id`, `Via`,
`X-Envoy-*` and friends are dropped, `Location` is resolved to an absolute canonical URL so
`https://shop.example.com:443/login` and `/login` compare equal, and `Set-Cookie` is reduced to
its name and behavioural attributes.

## Supported annotations

`swapsafe rules` prints the current list. Today:

| Rule | Annotation |
| --- | --- |
| `nginx.ssl-redirect` | `ssl-redirect` |
| `nginx.force-ssl-redirect` | `force-ssl-redirect` |
| `nginx.rewrite-target` | `rewrite-target` |
| `nginx.use-regex` | `use-regex` |
| `nginx.app-root` | `app-root` |
| `nginx.permanent-redirect` | `permanent-redirect` |
| `nginx.temporal-redirect` | `temporal-redirect` |
| `nginx.enable-cors` | `enable-cors`, `cors-allow-*`, `cors-expose-headers`, `cors-max-age` |
| `nginx.proxy-body-size` | `proxy-body-size` (reported, not probeable) |
| `nginx.backend-protocol` | `backend-protocol` (reported, not probeable) |

An annotation with no rule is **never passed over in silence** — it appears in the report as an
`info` finding saying it was not verified.

## Non-goals

swapsafe does not translate configuration — that is ingress2gateway's job. It does not change
traffic, set up mirroring, or load test. It does not replay write requests.

## Contributing

Rules are the easy way in: one Go file and a test. See [CONTRIBUTING.md](CONTRIBUTING.md).

```console
$ make test          # unit tests, with the race detector
$ make lint          # golangci-lint
$ make vuln          # govulncheck
$ hack/kind-up.sh    # kind with ingress-nginx and Envoy Gateway side by side
$ make e2e           # run swapsafe against both and compare to test/golden/
$ hack/kind-down.sh
```

## License

[Apache-2.0](LICENSE)
