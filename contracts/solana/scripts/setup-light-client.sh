#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/workspace.sh"
cd "$solana_repo"

python3 scripts/fetch-light-client.py
upstream=$SOLANA_DEPS/ibc-contracts

(
  cd "$upstream/ibc-solana"
  export CARGO_TARGET_DIR="$PWD/target" RUSTUP_TOOLCHAIN=1.89.0
  mkdir -p target/idl target/types
  for program in ics07_tendermint access_manager; do
    anchor build --program-name "$program" --no-idl -- -- --locked
    anchor idl build --program-name "$program" \
      --out "target/idl/$program.json" --out-ts "target/types/$program.ts" -- --locked
  done
)

# The checked-in IDLs define the CPI ABI. Rebuilds must produce the same files.
cmp idls/ics07_tendermint.json "$upstream/ibc-solana/target/idl/ics07_tendermint.json"
cmp idls/access_manager.json "$upstream/ibc-solana/target/idl/access_manager.json"
echo "Pinned light client and access manager are ready for local tests."
