#!/bin/bash

#
# Set up a kubernetes environment with kind.
#
# * Install Akash CRD
# * Optionally install metrics-server

rootdir="$(dirname "$0")/.."

install_crd() {
  kubectl apply -f "$rootdir/pkg/apis/akash.network/v1/crd.yaml"
}

install_metrics() {
  # https://github.com/kubernetes-sigs/kind/issues/398#issuecomment-621143252
  kubectl apply -f "$(dirname "$0")/kind-metrics-server.yaml"

  kubectl wait pod --namespace kube-system \
    --for=condition=ready \
    --selector=k8s-app=metrics-server \
    --timeout=90s

  count=1
  while ! kubectl top nodes; do
    echo "[$((count++))] waiting for metrics..."
    sleep 1
  done

  echo "metrics available"
}

usage() {
  cat <<EOF
  Install k8s dependencies for integration tests against "KinD"

  Usage: $0 [crd|metrics]

  crd:     install the akash CRDs
  metrics: install CRDs, metrics-server and wait for metrics to be available
EOF
  exit 1
}

case "${1:-metrics}" in
  crd)
    install_crd
    ;;
  metrics)
    install_crd
    install_metrics
    ;;
  *) usage;;
esac
