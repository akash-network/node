#!/usr/bin/env bash

set -euo pipefail
source "$(dirname "$0")/workspace.sh"

if [[ -z $solana_repo ]]; then
	echo "solana_repo var is not set"
	exit 1
fi

cd "$solana_repo"

bash scripts/doctor.sh
# The reviewed SDK archive is an explicit input until its package is published.
test -f "$SOLANA_CACHE/chain-sdk.tgz" || {
	echo 'Supply the locked SDK archive; see docs/sdk-integration.md.' >&2
	exit 1
}
npm ci
make -C "$NODE_SINK_DIR" bins

npm run setup:light-client
npm run setup:metadata
npm run build
npm run build:header-encoder
npm run typecheck

echo 'Build complete. See docs/setup.md for the explicit accelerated-testnet build.'
