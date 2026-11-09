#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/workspace.sh"
cd "$solana_repo"
# Anchor 0.32.1 syncs IDs on discovery if localnet and target/deploy are absent.
# Prepare its directory in the workspace cache before it discovers this config.
mkdir -p "$CARGO_TARGET_DIR/deploy"
if [[ ! -e target && ! -L target ]]; then
  ln -s "$CARGO_TARGET_DIR" target
fi
mkdir -p target/deploy
anchor build --provider.cluster mainnet --no-idl "$@" -- -- --locked
mkdir -p "$CARGO_TARGET_DIR/idl" "$CARGO_TARGET_DIR/types"
anchor idl build --provider.cluster mainnet --program-name akt \
  --out "$CARGO_TARGET_DIR/idl/akt.json" \
  --out-ts "$CARGO_TARGET_DIR/types/akt.ts" -- --locked
