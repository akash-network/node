#!/usr/bin/env bash

set -euo pipefail

export GOWORK=off
source "$(dirname "$0")/workspace.sh"

if [[ -z $solana_repo ]]; then
	echo "solana_repo var is not set"
	exit 1
fi

cd "$solana_repo"

node_repo="$NODE_SINK_DIR"
node_repo=$(cd "$node_repo" && pwd)

mkdir -p "$SOLANA_RUN"
fixture_run=$(mktemp -d "$SOLANA_RUN/sink-proof-v2.XXXXXX")
export SINK_PROOF_FIXTURE="$fixture_run/proof.json"
export SINK_REPORTER_FIXTURE="$fixture_run/reporter.json"
echo "Sink fixture evidence: $fixture_run"
go_args=(-mod=readonly)

if [[ -n ${AKT_NODE_MODFILE:-} ]]; then
  go_args+=(-modfile="$AKT_NODE_MODFILE")
elif [[ -f "$node_repo/.cache/solana/node.mod" ]]; then
  go_args+=(-modfile=.cache/solana/node.mod)
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
