#!/usr/bin/env bash
# Deletes the end-to-end cluster.
set -euo pipefail
kind delete cluster --name "${CLUSTER:-swapsafe-e2e}"
