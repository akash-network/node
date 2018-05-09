#!/bin/sh
# vim: ts=2 sts=2 sw=2 et

source ./env.sh

do_init(){
  rm -rf "$DATA_ROOT"
  mkdir -p "$AKASH_DIR"
  mkdir -p "$AKASHD_DIR"

  _akash key create master > "$DATA_ROOT/master.key"
  _akash key create other  > "$DATA_ROOT/other.key"

  _akashd init "$(cat "$DATA_ROOT/master.key")" -t helm -c "${HELM_NODE_COUNT:-4}"
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
    akash deployment create ../deployment.yml -k master
    ;;
  *)
    echo "USAGE: $0 <init|send|query|marketplace|deploy>" >&2
    exit 1
    ;;
esac
