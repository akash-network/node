#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/workspace.sh"
cd "$solana_repo"
bash scripts/doctor.sh
npm ci --prefix "$CHAIN_SDK_DIR/ts"
npm run build --prefix "$CHAIN_SDK_DIR/ts"
mkdir -p .cache
sdk_package=$(cd "$CHAIN_SDK_DIR/ts" && npm pack --silent --pack-destination "$solana_repo/.cache")
mv ".cache/$sdk_package" .cache/chain-sdk.tgz
npm ci
bash scripts/build-node.sh
npm run setup:light-client
npm run setup:metadata
npm run build
npm run build:header-encoder
npm run typecheck
echo 'Build complete. Run npm run testnet:up, then npm run testnet:demo.'
