source ../common.sh

PROVIDER_DIR=$DATA_ROOT/provider

akash() {
  _akash "$@"
}

akashd() {
  _akashd "$@"
}

akash_provider() {
  "$AKASH_ROOT/akash" -d "$PROVIDER_DIR" "$@"
}

