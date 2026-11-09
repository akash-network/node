#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
role=$(basename "$0")
exec "$root/runtime/bin/node" --import "$root/contracts/solana/node_modules/tsx/dist/loader.mjs" \
  "$root/contracts/solana/deploy/operator.ts" "${role#akt-}" "$@"
