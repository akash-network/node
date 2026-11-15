#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/workspace.sh"

node_repo=$(cd "$solana_repo/../.." && pwd)
mkdir -p "$node_repo/.cache/bin"
cd "$node_repo"
CGO_ENABLED=0 go build -mod=readonly -trimpath -o .cache/bin/akt-relayer ./cmd/akt-relayer
