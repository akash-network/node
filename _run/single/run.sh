#!/bin/sh
# vim: ts=2 sts=2 sw=2 et

source ./env.sh

do_init() {
  rm -rf "$DATA_ROOT"
  mkdir -p "$AKASH_DIR"
  mkdir -p "$AKASHD_DIR"

  akash key create master > "$DATA_ROOT/master.key"
  akash key create other  > "$DATA_ROOT/other.key"
  akashd init "$(cat "$DATA_ROOT/master.key")"

  akash_provider key create master > "$DATA_ROOT/provider-master.key"
}

case "$1" in
  init)
    do_init
    ;;
  akashd)
    akashd start
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
  provider)
    akash_provider provider create provider.yml -k master > "$DATA_ROOT"/master.dc
    akash_provider provider run "$(cat "$DATA_ROOT/master.dc")" -k master
    ;;
  deploy)
    akash deployment create ../deployment.yml -k master
    ;;
  *)
    echo "USAGE: $0 <init|akashd|send|query|marketplace|provider|deploy>" >&2
    exit 1
    ;;
esac
