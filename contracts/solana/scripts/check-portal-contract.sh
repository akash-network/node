#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/workspace.sh"
portal_repo=${SINK_PORTAL_DIR:-$(dirname "$NODE_SINK_DIR")/sink-portal}
cmp "$portal_repo/contracts.ts" "$solana_repo/local/team-contracts.ts"
