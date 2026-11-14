# swapsafe

Verify that your Gateway API setup reproduces ingress-nginx behaviour — before you move traffic.

swapsafe sends the same probe requests to your ingress-nginx endpoint and to your new Gateway
endpoint, normalizes both responses, and reports every meaningful difference together with the
ingress-nginx annotation that caused it.

> **Status:** pre-alpha. Nothing works yet; see [docs/PLAN.md](docs/PLAN.md) for the roadmap.

## Why

ingress-nginx is end-of-life. [ingress2gateway](https://github.com/kubernetes-sigs/ingress2gateway)
translates the configuration, but its own docs tell you to review and fix the output by hand. No
tool verifies the *runtime* behaviour. swapsafe is that check.

## Install

Not released yet.

## Quickstart

```console
$ swapsafe run \
    --baseline  https://203.0.113.10 \
    --candidate https://203.0.113.20 \
    --from-cluster --namespace shop \
    --fail-on major
```

swapsafe is read-only: it never applies manifests, never changes DNS, and by default only sends
GET, HEAD and OPTIONS.

## Documentation

- [Plan](docs/PLAN.md) — vision, architecture, roadmap
- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md)

## License

Apache-2.0
