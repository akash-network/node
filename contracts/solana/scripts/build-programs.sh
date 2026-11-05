#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/workspace.sh"
cd "$solana_repo"
anchor build --no-idl "$@" -- -- --locked
mkdir -p "$CARGO_TARGET_DIR/idl" "$CARGO_TARGET_DIR/types"
anchor idl build --program-name akt \
  --out "$CARGO_TARGET_DIR/idl/akt.json" \
  --out-ts "$CARGO_TARGET_DIR/types/akt.ts" -- --locked
