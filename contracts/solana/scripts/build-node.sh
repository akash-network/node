#!/usr/bin/env bash
set -euo pipefail

# Builds .cache/bin/akash in the node repository against the local chain-sdk
# checkout. An optional source argument builds another node checkout instead,
# such as the old node used by the upgrade rehearsal.
source "$(dirname "$0")/workspace.sh"
node_repo=$(cd "${1:-$NODE_SINK_DIR}" && pwd)
sdk_repo=$(cd "$CHAIN_SDK_DIR" && pwd)
test -f "$sdk_repo/go/go.mod" || { echo "Missing chain-sdk/go/go.mod" >&2; exit 1; }
cd "$node_repo"
export GOWORK=off CGO_ENABLED=1
mkdir -p .cache/bin
# Keep the committed dependency files unchanged. The SDK replace stays local.
cp go.mod .cache/solana.mod
cp go.sum .cache/solana.sum
go mod edit -modfile=.cache/solana.mod -replace "pkg.akt.dev/go=$sdk_repo/go"
go mod edit -modfile=.cache/solana.mod -replace "pkg.akt.dev/go/cli=$sdk_repo/go/cli"
# Same version flags as the Makefile, so `akash version` identifies the build.
version=$(git describe --tags --always --dirty 2>/dev/null || echo unknown)
commit=$(git rev-parse HEAD 2>/dev/null || echo unknown)
ldflags="-X github.com/cosmos/cosmos-sdk/version.Name=akash"
ldflags+=" -X github.com/cosmos/cosmos-sdk/version.AppName=akash"
ldflags+=" -X github.com/cosmos/cosmos-sdk/version.Version=${version#v}"
ldflags+=" -X github.com/cosmos/cosmos-sdk/version.Commit=$commit"
# Test networks need refunds in seconds, so dev builds lower the 72-hour
# challenge-period floor (chain-sdk challenge_floor_dev.go). Set
# SINK_BUILD_TAGS= (empty) to build with the release floor, as a mainnet fork must.
tags=${SINK_BUILD_TAGS-sinkdev}
go build -mod=readonly -modfile=.cache/solana.mod -tags "$tags" -ldflags "$ldflags" -o .cache/bin/akash.new ./cmd/akash
mv .cache/bin/akash.new .cache/bin/akash
go list -modfile=.cache/solana.mod -m -json pkg.akt.dev/go > .cache/solana-sdk.json
echo "Built $node_repo/.cache/bin/akash $version using $sdk_repo/go (tags: ${tags:-none})"
