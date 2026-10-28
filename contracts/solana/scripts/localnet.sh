#!/usr/bin/env bash
# Starts an empty local Solana validator for rehearsing deployments and admin
# commands. Only Metaplex is preloaded, at its mainnet address; the AKT program,
# verifier and access manager are deployed with `solana program deploy` as in
# docs/deploy-program.md. Use `npm test` for the contract suite and
# `npm run local:up` for both chains; they start their own validators.
#
# Usage: bash scripts/localnet.sh [solana-test-validator options...]
# AKT_LOCALNET_RPC_PORT (default 17899) selects the RPC port; WebSocket is the
# next port and the faucet, gossip and dynamic ports follow it.
# AKT_LOCALNET_LEDGER (default .local-ledger) selects the ledger directory.
set -euo pipefail
cd "$(dirname "$0")/.."

port=${AKT_LOCALNET_RPC_PORT:-17899}
ledger=${AKT_LOCALNET_LEDGER:-.local-ledger}
if [[ ! "$port" =~ ^[0-9]+$ ]] || ((port < 1025 || port > 65480)); then
  echo 'AKT_LOCALNET_RPC_PORT must leave room for the next 50 ports.' >&2
  exit 1
fi
if lsof -nP -iTCP:"$port-$((port + 50))" -sTCP:LISTEN > /dev/null 2>&1; then
  echo "Ports $port-$((port + 50)) are partly in use. Choose AKT_LOCALNET_RPC_PORT." >&2
  exit 1
fi
metaplex=.deps/metaplex/mpl_token_metadata.so
if [[ ! -f "$metaplex" ]]; then
  echo 'Missing Metaplex. Run npm run setup:metadata first.' >&2
  exit 1
fi
exec solana-test-validator --ledger "$ledger" --bind-address 127.0.0.1 \
  --rpc-port "$port" --faucet-port "$((port + 2))" --gossip-port "$((port + 3))" \
  --dynamic-port-range "$((port + 4))-$((port + 50))" \
  --bpf-program metaqbxxUerdq28cj1RbAWkYQm3ybzjb6a8bt518x1s "$metaplex" \
  "$@"
