#!/bin/sh
# vim: ts=2 sts=2 sw=2 et

source ./env.sh

do_init(){
  rm -rf "$DATA_ROOT"
  mkdir -p "$AKASH_DIR"
  mkdir -p "$AKASHD_DIR"

	eval $(akash key create master -m shell)
	echo $akash_create_key_0_public_key > "$DATA_ROOT/master.key"
	eval $(akash key create other -m shell)
	echo $akash_create_key_0_public_key > "$DATA_ROOT/other.key"

  _akashd init "$(cat "$DATA_ROOT/master.key")" -t helm -n "node-0,node-1,node-2"
}

case "$1" in
  init)
    do_init
    ;;
  send)
    akash send 100 $(cat "$DATA_ROOT/other.key") -k master
    ;;
  query)
    key=${2:-master}
    akash query account $(cat "$DATA_ROOT/$key.key")
    ;;
  marketplace)
    akash marketplace
    ;;
  deploy)
    akash deployment create deployment.yml -k master
    ;;
  minikube-start)
    minikube start --cpus 4 --memory 4096 --kubernetes-version v1.15.4
    minikube addons enable ingress
    minikube addons enable metrics-server
    kubectl create -f rbac.yml
    helm init
    ;;
  *)
    echo "USAGE: $0 <init|send|query|marketplace|deploy|minikube-start>" >&2
    exit 1
    ;;
esac
