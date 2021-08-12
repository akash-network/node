#!/bin/bash

#
# Set up a kubernetes environment with kind.
#
# * Install Akash CRD
# * Install `akash-services` Namespace
# * Install Network Policies
# * Optionally install metrics-server

set -xe

rootdir="$(dirname "$0")/.."

install_ns() {
  kubectl apply -f "$rootdir/_docs/kustomize/networking/"
}

install_network_policies() {
  kubectl kustomize "$rootdir/_docs/kustomize/akash-services/" | kubectl apply -f-
}

install_crd() {
  kubectl apply -f "$rootdir/pkg/apis/akash.network/v1/crd.yaml"
  kubectl apply -f "$rootdir/pkg/apis/akash.network/v1/provider_hosts_crd.yaml"
  kubectl apply -f "$rootdir/_docs/kustomize/storage/storageclass.yaml"
  kubectl patch node "${KIND_NAME}-control-plane" -p '{"metadata":{"labels":{"akash.network/storageclasses":"beta2"}}}'
}

install_metrics() {
  # https://github.com/kubernetes-sigs/kind/issues/398#issuecomment-621143252
  kubectl apply -f "$(dirname "$0")/kind-metrics-server.yaml"

#  kubectl wait pod --namespace kube-system \
#    --for=condition=ready \
#    --selector=k8s-app=metrics-server \
#    --timeout=90s

  echo "metrics initialized"
}

usage() {
  cat <<EOF
  Install k8s dependencies for integration tests against "KinD"

  Usage: $0 [crd|ns|metrics]

  crd:        install the akash CRDs
  ns:         install akash namespace
  metrics:    install CRDs, NS, metrics-server and wait for metrics to be available
  calico-metrics: install CRDs, NS, Network Policies, metrics-server and wait for metrics to be available
  networking: install essential k8s namespace and network policies for Akash services
EOF
  exit 1
}

case "${1:-metrics}" in
  crd)
    install_crd
    ;;
  ns)
    install_ns
    ;;
  metrics)
    install_crd
    install_ns
    install_metrics
    ;;
  calico-metrics)
    install_crd
    install_ns
    install_metrics
    install_network_policies
    ;;
  networking)
    install_ns
    install_network_policies
    ;;
  *) usage;;
esac
