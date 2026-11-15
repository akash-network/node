#!/usr/bin/env bash
# Build on the production OS/architecture; the archive includes native modules.
set -euo pipefail
source "$(dirname "$0")/../scripts/workspace.sh"

if (($# != 2)); then
  echo 'usage: build-services.sh /absolute/manifest.json /absolute/services.tar.gz' >&2
  exit 2
fi
manifest=$1 out=$2
[[ "$manifest" == /* && "$out" == /* && ! -e "$out" ]] || {
  echo 'Use absolute paths and a new output archive.' >&2
  exit 2
}
cd "$solana_repo"
[[ "$(uname -s)" == Linux ]] || { echo "Build service artifacts on the target Linux platform." >&2; exit 1; }
node --import tsx deploy/programs.ts check --manifest "$manifest"
commit=$(node -e 'process.stdout.write(require(process.argv[1]).sourceCommit)' "$manifest")
[[ "$(git rev-parse HEAD)" == "$commit" ]] || { echo 'Checkout must match the launch commit.' >&2; exit 1; }
git -C "$NODE_SINK_DIR" diff --quiet HEAD -- .
[[ -z "$(git -C "$NODE_SINK_DIR" ls-files --others --exclude-standard -- .)" ]] || { echo 'Commit or remove untracked node source first.' >&2; exit 1; }
[[ "$(node --version)" == v24.* ]] || { echo 'Node 24 is required.' >&2; exit 1; }

stage=$(mktemp -d "$SOLANA_CACHE/services.XXXXXX")
candidate=
trap 'rm -rf "$stage"; if [[ -n "$candidate" ]]; then rm -f "$candidate"; fi' EXIT
mkdir -p "$stage/root/.cache/solana/target/idl" "$stage/root/.cache/solana/target/types"
git -C "$NODE_SINK_DIR" archive "$commit" contracts/solana | tar -x -C "$stage/root"
build=$(node -e 'const p=require("node:path"); const m=require(process.argv[1]); process.stdout.write(p.resolve(p.dirname(process.argv[1]),m.buildDirectory))' "$manifest")
cp "$SOLANA_CACHE/chain-sdk.tgz" "$stage/root/.cache/solana/"
cp "$build/akt.json" "$stage/root/.cache/solana/target/idl/"
cp "$build/akt.ts" "$stage/root/.cache/solana/target/types/"
upstream_types=.cache/solana/deps/ibc-contracts/ibc-solana/target/types
mkdir -p "$stage/root/$upstream_types"
cp "$build/ics07_tendermint.ts" \
  "$build/access_manager.ts" "$stage/root/$upstream_types/"
cmp "$build/ics07_tendermint.json" idls/ics07_tendermint.json
cmp "$build/access_manager.json" idls/access_manager.json
mkdir -p "$stage/root/bin"
(
  cd "$NODE_SINK_DIR"
  CGO_ENABLED=0 go build -mod=readonly -trimpath -buildvcs=false \
    -ldflags "-s -w -X main.version=manifest-$commit -X main.commit=$commit" \
    -o "$stage/root/bin/akt-relayer" ./cmd/akt-relayer
)
(
  cd "$stage/root/contracts/solana"
  npm ci --include=dev
  npm run typecheck
  npm run test:deployment
  npm prune --omit=dev
)
{
  echo "commit $commit"
  echo "platform $(uname -sm)"
  node --version
  npm --version
  cat "$build/build-info.txt"
  (cd "$stage/root"; sha256sum bin/akt-relayer)
} > "$stage/root/service-build-info.txt"
cp "$manifest" "$stage/root/launch-manifest.json"
mkdir -p "$(dirname "$out")"
tar -czf "$stage/services.tar.gz" -C "$stage/root" .
# Publish atomically on the destination filesystem without replacing a release.
candidate=$(mktemp "$(dirname "$out")/.akt-services.XXXXXX")
cp "$stage/services.tar.gz" "$candidate"
ln "$candidate" "$out"
if command -v sha256sum > /dev/null; then
  sha256sum "$out"
else
  shasum -a 256 "$out"
fi
