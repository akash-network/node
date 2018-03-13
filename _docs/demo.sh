#!/bin/sh
# vim: ts=4 sts=4 sw=4 et

DIR=devdata
AKASH_DIR=$DIR/client
AKASHD_DIR=$DIR/node

akash() {
  ./akash -d "$AKASH_DIR" "$@"
}

akashd() {
  ./akashd -d "$AKASH_DIR" "$@"
}

init() {
  rm -rf "$DIR"
  mkdir -p "$AKASH_DIR"
  mkdir -p "$AKASHD_DIR"

  akash key create master > "$DIR/master.key"
  akash key create dc-1   > "$DIR/dc-1.key"
  akashd init "$(cat "$DIR/master.key")"
}

case "$1" in
  init)
    init
    ;;
  node)
    akashd start
    ;;
  monitor)
    akash marketplace
    ;;
  dc-1)
    akash provider create unused.yml -k dc-1 | \
      sed -e 's/.*: //' > "$DIR"/dc-1.dc
    akash provider run "$(cat "$DIR/dc-1.dc")" -k dc-1
    ;;
  deploy)
    akash deploy unused.yml -k master
    ;;
esac
