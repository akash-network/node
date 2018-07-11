AKASH_ROOT=../..

DATA_ROOT=data
AKASH_DIR=$DATA_ROOT/client
AKASHD_DIR=$DATA_ROOT/node
DEFAULT_AKASH_NODE="http://localhost:26657"

_akash() {
  AKASH_NODE="${AKASH_NODE:-$DEFAULT_AKASH_NODE}" \
    "$AKASH_ROOT/akash" -d "$AKASH_DIR" "$@"
}

_akashd() {
  "$AKASH_ROOT/akashd" -d "$AKASHD_DIR" "$@"
}

stripkey() {
  sed -e 's/.*: //'
}
