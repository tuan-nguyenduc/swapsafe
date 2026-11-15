#!/usr/bin/env bash
# Stands up a kind cluster running ingress-nginx and Envoy Gateway side by
# side, with the end-to-end fixtures applied. Both controllers serve the same
# hostnames, which is exactly the situation swapsafe exists to check.
#
#   hack/kind-up.sh        create and wait
#   hack/kind-down.sh      delete
set -euo pipefail

CLUSTER="${CLUSTER:-swapsafe-e2e}"
# ingress-nginx is end of life; v1.15.1 (2026-03-19) is the last release.
INGRESS_NGINX_VERSION="${INGRESS_NGINX_VERSION:-controller-v1.15.1}"
ENVOY_GATEWAY_VERSION="${ENVOY_GATEWAY_VERSION:-v1.9.3}"

here="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

say() { printf '\n== %s\n' "$*"; }

if kind get clusters 2>/dev/null | grep -qx "$CLUSTER"; then
  say "cluster $CLUSTER already exists, reusing it"
else
  say "creating cluster $CLUSTER"
  kind create cluster --name "$CLUSTER" --config=- <<'EOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    kubeadmConfigPatches:
      - |
        kind: InitConfiguration
        nodeRegistration:
          kubeletExtraArgs:
            node-labels: "ingress-ready=true"
EOF
fi

kubectl config use-context "kind-$CLUSTER" >/dev/null

say "installing ingress-nginx $INGRESS_NGINX_VERSION"
kubectl apply -f "https://raw.githubusercontent.com/kubernetes/ingress-nginx/${INGRESS_NGINX_VERSION}/deploy/static/provider/kind/deploy.yaml"

say "installing Envoy Gateway $ENVOY_GATEWAY_VERSION"
kubectl apply --server-side -f "https://github.com/envoyproxy/gateway/releases/download/${ENVOY_GATEWAY_VERSION}/install.yaml"

say "waiting for the controllers"
kubectl wait --namespace ingress-nginx \
  --for=condition=Ready pod --selector=app.kubernetes.io/component=controller --timeout=300s
kubectl wait --namespace envoy-gateway-system \
  --for=condition=Available deployment/envoy-gateway --timeout=300s

say "applying fixtures"
kubectl apply -f "$here/test/fixtures/"

say "waiting for the backend and the Gateway"
kubectl -n swapsafe-e2e wait --for=condition=Available deployment/backend --timeout=180s
kubectl -n swapsafe-e2e wait --for=condition=Accepted gateway/swapsafe --timeout=300s
# The Gateway never reports Programmed here: its Service is a LoadBalancer and
# kind has nothing to assign an address. The proxy itself is what matters, and
# the tests reach both endpoints by port-forward.
kubectl -n envoy-gateway-system wait --for=condition=Available deployment \
  --selector=gateway.envoyproxy.io/owning-gateway-name=swapsafe --timeout=300s

say "ready"
kubectl -n swapsafe-e2e get ingress,httproute,gateway
