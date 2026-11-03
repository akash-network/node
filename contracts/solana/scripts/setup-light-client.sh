#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/workspace.sh"
cd "$solana_repo"

case "$*" in
  '') programs=(ics07_tendermint access_manager) ;;
  --test-proxy) programs=(test_cpi_proxy) ;;
  *) echo 'usage: setup-light-client.sh [--test-proxy]' >&2; exit 2 ;;
esac

python3 scripts/fetch-light-client.py
upstream=$SOLANA_DEPS/ibc-contracts

(
  cd "$upstream/ibc-solana"
  CARGO_TARGET_DIR="$PWD/target"
  RUSTUP_TOOLCHAIN=$(python3 -c 'import sys, tomllib; print(tomllib.load(open(sys.argv[1], "rb"))["toolchain"]["channel"])' "$solana_repo/rust-toolchain.toml")
  export CARGO_TARGET_DIR RUSTUP_TOOLCHAIN
  mkdir -p target/idl target/types
  for program in "${programs[@]}"; do
    anchor build --program-name "$program" --no-idl -- -- --locked
    [[ "$program" == test_cpi_proxy ]] && continue
    anchor idl build --program-name "$program" \
      --out "target/idl/$program.json" --out-ts "target/types/$program.ts" -- --locked
  done
)

[[ ${1:-} == --test-proxy ]] && exit 0

# The checked-in IDLs define the CPI ABI. Rebuilds must produce the same files.
cmp idls/ics07_tendermint.json "$upstream/ibc-solana/target/idl/ics07_tendermint.json"
cmp idls/access_manager.json "$upstream/ibc-solana/target/idl/access_manager.json"
echo "Pinned light client and access manager are ready for local tests."
