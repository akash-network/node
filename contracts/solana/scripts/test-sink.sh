#!/usr/bin/env bash
set -euo pipefail
export GOWORK=off
source "$(dirname "$0")/workspace.sh"
cd "$solana_repo"
node_repo="$NODE_SINK_DIR"
node_repo=$(cd "$node_repo" && pwd)
export SINK_PROOF_FIXTURE="$node_repo/.cache/sink-proof.json"
export SINK_REPORTER_FIXTURE="$node_repo/.cache/sink-reporter.json"
mkdir -p "$node_repo/.cache"
go_args=(-mod=readonly)
if [[ -f "$node_repo/.cache/solana.mod" ]]; then
  go_args+=(-modfile=.cache/solana.mod)
fi
(
  cd "$node_repo"
  go test "${go_args[@]}" ./app -run '^TestSinkSignedDepositAndProof$' -count=1 -v
)
npm test
(
  cd "$node_repo"
  go test "${go_args[@]}" ./app -run '^TestSinkAcceptsSolanaObserverReports$' -count=1 -v
)
