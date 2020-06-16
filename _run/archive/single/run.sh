#!/bin/sh
# vim: ts=2 sts=2 sw=2 et

#shellcheck disable=SC2039
source ./env.sh

do_init() {
  rm -rf "$DATA_ROOT"
  mkdir -p "$AKASH_DIR"
  mkdir -p "$AKASHD_DIR"

	eval "$(akash key create master -m shell)"
  #shellcheck disable=SC2154
	echo "$akash_create_key_0_public_key" > "$DATA_ROOT/master.key"

	eval "$(akash key create other -m shell)"
	echo "$akash_create_key_0_public_key" > "$DATA_ROOT/other.key"

	eval "$(akash_provider key create master -m shell)"
	echo "$akash_create_key_0_public_key" > "$DATA_ROOT/provider/master.key"

  akashd init "$(cat "$DATA_ROOT/master.key")"
}

case "$1" in
  init)
    do_init
    ;;
  akashd)
    akashd start
    ;;
  send)
    akash send 100 "$(cat "$DATA_ROOT/other.key")" -k master
    ;;
  query)
    key=${2:-master}
    akash query account "$(cat "$DATA_ROOT/$key.key")"
    ;;
  marketplace)
    akash marketplace
    ;;
  provider)
    address_loc="$DATA_ROOT/provider.addr"
    if [ ! -f "$address_loc" ]; then
      # shellcheck disable=SC2154
      eval "$(akash_provider provider add provider.yml -k master -m shell)" &&
        echo "$akash_add_provider_0_key" > "$address_loc"
    fi
    akash_provider provider run "$(cat "$address_loc")" -k master
    ;;
  deploy)
    akash deployment create ../deployment.yaml -k master
    ;;
  *)
    echo "USAGE: $0 <init|akashd|send|query|marketplace|provider|deploy>" >&2
    exit 1
    ;;
esac
