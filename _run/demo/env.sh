export AKASH_HOME=data/akash
export AKASHD_HOME=data/akashd

AKASH_ROOT=../..

akash() {
  $AKASH_ROOT/akash "$@"
}

akashd() {
  $AKASH_ROOT/akashd "$@"
}
