#!/usr/bin/env bash
# Generate operator interfaces from the checkout without building SBF programs.
set -euo pipefail
source "$(dirname "$0")/../scripts/workspace.sh"
cd "$solana_repo"
test -f "$SOLANA_CACHE/chain-sdk.tgz"
npm ci
python3 scripts/fetch-light-client.py
out=$SOLANA_CACHE/service-inputs
mkdir -p "$out" "$CARGO_TARGET_DIR/idl" "$CARGO_TARGET_DIR/types"
anchor idl build --program-name akt --out "$out/akt.json" --out-ts "$out/akt.ts" -- --locked
cp "$out/akt.json" "$CARGO_TARGET_DIR/idl/"
cp "$out/akt.ts" "$CARGO_TARGET_DIR/types/"
upstream=$SOLANA_DEPS/ibc-contracts/ibc-solana
for program in ics07_tendermint access_manager; do
  (
    cd "$upstream"
    anchor idl build --program-name "$program" \
      --out "$out/$program.json" --out-ts "$out/$program.ts" -- --locked
  )
  cmp "idls/$program.json" "$out/$program.json"
done
mkdir -p "$upstream/target/types"
cp "$out/ics07_tendermint.ts" "$out/access_manager.ts" "$upstream/target/types/"
printf 'commit %s\n' "$(git rev-parse HEAD)" > "$out/build-info.txt"
npm run typecheck
npm run test:services
npm run test:deployment
