<p align="center">
  <img src=".github/logo.png" width="84" alt="">
</p>

<h1 align="center">swapsafe</h1>

<p align="center">
  <strong>Verify that your Gateway API setup reproduces ingress-nginx behaviour —<br>
  before you move traffic.</strong>
</p>

<p align="center">
  <a href="https://github.com/tuan-nguyenduc/swapsafe/actions/workflows/ci.yml"><img src="https://github.com/tuan-nguyenduc/swapsafe/actions/workflows/ci.yml/badge.svg" alt="ci"></a>
  <a href="https://github.com/tuan-nguyenduc/swapsafe/releases/latest"><img src="https://img.shields.io/github/v/release/tuan-nguyenduc/swapsafe" alt="release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache--2.0-blue" alt="license"></a>
</p>

ingress-nginx is end of life. [ingress2gateway](https://github.com/kubernetes-sigs/ingress2gateway)
translates the configuration, and its own documentation tells you to review and fix the output by
hand. Nothing checks the *runtime* behaviour. swapsafe is that check.

It sends the same requests to your ingress-nginx endpoint and to your new Gateway endpoint,
normalizes both answers, and reports every difference that matters — with the ingress-nginx
annotation that caused it, and what to change.

![swapsafe finding the routes a migration broke](.github/demo.gif)

> **Status: v0.1.0, early.** `scan`, `run` and `rules` work, against YAML manifests or a live
> cluster, and the end-to-end suite runs against a real ingress-nginx and a real Envoy Gateway.
> Linux and macOS binaries, not yet signed, and no SBOM yet. The JSON output carries `apiVersion:
> swapsafe.dev/v1alpha1` and will change before v1.

## Install

Linux and macOS, amd64 and arm64:

```bash
VERSION=v0.1.2
OS=$(uname -s | tr A-Z a-z)
ARCH=$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')
ARCHIVE=swapsafe_${VERSION#v}_${OS}_${ARCH}.tar.gz

curl -sSLO https://github.com/tuan-nguyenduc/swapsafe/releases/download/$VERSION/$ARCHIVE
curl -sSLO https://github.com/tuan-nguyenduc/swapsafe/releases/download/$VERSION/checksums.txt
shasum -a 256 --check checksums.txt --ignore-missing
tar -xzf $ARCHIVE swapsafe && sudo install swapsafe /usr/local/bin/
```

On macOS, download with `curl` as above rather than through a browser: the binaries are not
signed or notarized yet, and a browser download would be quarantined by Gatekeeper.

Or build it, on any platform Go runs on:

```bash
git clone https://github.com/tuan-nguyenduc/swapsafe && cd swapsafe
make build        # -> bin/swapsafe
```

Go 1.26.11 or newer. Windows, Homebrew and krew are not released yet; build from source in the
meantime.

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

```bash
swapsafe scan --from-cluster --namespace shop --ingress-class nginx
```

swapsafe only ever lists. A kubeconfig with `get` and `list` on Ingress and HTTPRoute is all it
needs, and all it should be given.

Then probe both endpoints:

```console
$ swapsafe run \
    --baseline  203.0.113.10 \
    --candidate 203.0.113.20 \
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

The table is coloured when it is going to a terminal, and plain when it is going anywhere else —
a pipe, a file, a CI log. `NO_COLOR=1` turns it off everywhere.

## The two addresses

`--baseline` is what is serving the traffic today. `--candidate` is what is asking to replace it.
swapsafe never decides which one is right: it reports where the candidate answers differently
from the baseline, and that is all.

Where to find them:

```bash
# ingress-nginx
kubectl -n ingress-nginx get svc ingress-nginx-controller \
    -o jsonpath='{.status.loadBalancer.ingress[0].ip}'

# the Gateway reports its own address
kubectl -n shop get gateway my-gateway -o jsonpath='{.status.addresses[0].value}'
```

No external address — kind, or bare metal without a load balancer — so forward the ports:

```bash
kubectl -n ingress-nginx port-forward svc/ingress-nginx-controller 18080:80 &
kubectl -n envoy-gateway-system port-forward svc/envoy-shop-my-gateway-abc123 18081:80 &
swapsafe run --baseline 127.0.0.1:18080 --candidate 127.0.0.1:18081 \
    --from-cluster --namespace shop
```

**Only the host and the port are used.** A host, a `host:port` or a full URL all work, but the
scheme is ignored — and that is deliberate, not an oversight:

| You write | The probe is | It connects to |
| --- | --- | --- |
| `203.0.113.10` | `GET http /` | `203.0.113.10:80` |
| `203.0.113.10` | `GET https /` | `203.0.113.10:443` |
| `https://203.0.113.10` | `GET http /` | `203.0.113.10:80` |
| `203.0.113.10:8080` | either | `203.0.113.10:8080` |

The scheme of each request comes from the rule that asked for it. Checking `ssl-redirect` means
sending **plain HTTP** to the very endpoint you may have written `https://` in front of — that
request is the whole test. If the flag's scheme won, the rule could not run at all.

The address is only where the TCP connection goes. The request itself carries the route's own
hostname:

```
connect to   203.0.113.20:443        the candidate's address
Host:        shop.example.com        from the Ingress rule
SNI:         shop.example.com        so the Gateway picks the right listener and certificate
```

That is what lets you ask the new Gateway what it would do for `shop.example.com` while DNS for
`shop.example.com` still points at ingress-nginx.

## Safe by default

swapsafe is built to be pointed at production.

- **Read only.** It never applies a manifest, never touches DNS, never writes to a cluster.
- **Safe methods only.** GET, HEAD and OPTIONS. Anything else needs `--allow-unsafe-methods`.
- **Gentle.** 20 requests per second per endpoint and 8 in flight by default.
- **Quiet about secrets.** Response bodies are hashed, not kept, unless you pass
  `--include-bodies`. Cookie values and credential headers are redacted, always.
- **Honest about what it did not check.** An application that puts a counter or a pod name in
  every response cannot be compared on its body. swapsafe notices and says so rather than
  blaming your migration; `--check-bodies` makes it look for that on every request, at one
  extra request each.
- **Works before DNS moves.** It connects to the endpoint address you give it while presenting
  each route's own hostname as the Host header and the SNI name, so the Gateway routes the
  request as it will in production.

## How it works

The idea is one request, two answers:

```
                                 ┌─────────────────────┐
                                 │  --baseline         │──▶ 308
   GET http://shop.example.com/  │  ingress-nginx      │    Location: https://shop.example.com/
   ────────────────────────────▶ └─────────────────────┘
   the same request, twice       ┌─────────────────────┐
   ────────────────────────────▶ │  --candidate        │──▶ 200
                                 │  Gateway API        │    (no Location)
                                 └─────────────────────┘
                                                              │
                                                              ▼
                                        critical  status      308 vs 200
                                        major     location    present vs absent
                                        caused by  nginx.ingress.kubernetes.io/ssl-redirect
```

swapsafe connects to the two addresses you give it, but every request carries the route's own
hostname as the `Host` header and the SNI name. The Gateway therefore routes it exactly as it
will in production, while DNS still points at ingress-nginx.

Seven stages turn a pair of manifests into that verdict:

```
  Ingress + HTTPRoute
  (files, or a cluster)
          │
          ▼
    ┌───────────┐  pair by namespace, host and path; a route with no
    │ collector │  HTTPRoute is already a critical finding
    └───────────┘
          │  []RouteSpec
          ▼
    ┌───────────┐  ask every rule that applies what it wants probed,
    │ generator │  and send each distinct request only once
    └───────────┘
          │  []TestCase                    ┌────────────────────────┐
          ▼                                │      rule registry     │
    ┌───────────┐  both endpoints, in      │  one rule per nginx    │
    │  prober   │  parallel, rate limited  │  annotation: when it   │
    └───────────┘                          │  applies, what to      │
          │  []Observation                 │  probe, how bad a      │
          ▼                                │  difference is, and    │
    ┌───────────┐  drop Date, Server,      │  what to change        │
    │ normalize │  X-Request-Id, hop-by-   └────────────────────────┘
    └───────────┘  hop; canonicalize            │         │
          │        Location; redact cookies     │         │
          ▼                                     ▼         │
    ┌───────────┐  compare status, headers, body ◀────────┘
    │  differ   │  and rank: critical, major, minor, info
    └───────────┘
          │  []Finding
          ▼
    ┌─────────────┐  name the annotation behind each difference
    │ attribution │  and attach the rule's hint
    └─────────────┘
          │
          ▼
    ┌───────────┐  table for a human, json for everything else
    │  report   │  exit 0, 1 or 2
    └───────────┘
```

Two things carry most of the weight, and neither is the annotation list:

**normalize.** Two proxies disagree about `Date`, `Server`, `X-Request-Id`, `Connection` and a
dozen other headers on every single request. A raw diff is all noise, and a tool that cries wolf
on a correct route gets closed after one run. Reading an annotation tells you what *should*
happen; probing tells you what *does* — and ingress-nginx answers 308 where a `RequestRedirect`
filter defaults to 302, which no annotation will ever tell you.

**the rule registry.** It is the only place that knows about annotations: it decides what to
probe, how severe a difference is, and what to suggest. Adding support for an annotation is
adding one file under `rules/ingressnginx/`, and an annotation nobody has written a rule for is
still reported — as `info`, never as silence.

Package layout follows the stages, one directory each under `internal/`. The public types and
the `Rule` interface live in `pkg/`, and a linter enforces that nothing under `pkg/` or `rules/`
imports anything private, so rules stay extractable into their own module.

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

```bash
python3 hack/demo/servers.py &   # two fake endpoints, no cluster needed
swapsafe run --baseline http://127.0.0.1:18080 --candidate http://127.0.0.1:18081 \
    --ingress-file hack/demo/manifests.yaml --httproute-file hack/demo/manifests.yaml

make test          # unit tests, with the race detector
make lint          # golangci-lint
make vuln          # govulncheck
hack/kind-up.sh    # kind with ingress-nginx and Envoy Gateway side by side
make e2e           # run swapsafe against both and compare to test/golden/
hack/kind-down.sh
```

## License

[Apache-2.0](LICENSE)
