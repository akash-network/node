#!/bin/sh
# vim: ts=4 sts=4 sw=4 et

DIR=devdata
PHOTON_DIR=$DIR/client
PHOTOND_DIR=$DIR/node

photon() {
  ./photon -d "$PHOTON_DIR" "$@"
}

photond() {
  ./photond -d "$PHOTON_DIR" "$@"
}

init() {
  rm -rf "$DIR"
  mkdir -p "$PHOTON_DIR"
  mkdir -p "$PHOTOND_DIR"

  photon key create master > "$DIR/master.key"
  photon key create dc-1   > "$DIR/dc-1.key"
  photond init "$(cat "$DIR/master.key")"
}

case "$1" in
  init)
    init
    ;;
  node)
    photond start
    ;;
  monitor)
    photon marketplace
    ;;
  dc-1)
    photon datacenter create unused.yml -k dc-1 | \
      sed -e 's/.*: //' > "$DIR"/dc-1.dc
    photon datacenter run "$(cat "$DIR/dc-1.dc")" -k dc-1
    ;;
  deploy)
    photon deploy unused.yml -k master
    ;;
esac
