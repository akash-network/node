source ../common.sh

PROVIDER_DIR=$DATA_ROOT/provider

akash() {
  _akash "$@"
}

akashd() {
  _akashd "$@"
}

akash_provider() {
  AKASH_NODE="${AKASH_NODE:-$DEFAULT_AKASH_NODE}" \
    "$AKASH_ROOT/akash" -d "$PROVIDER_DIR" "$@"
}

