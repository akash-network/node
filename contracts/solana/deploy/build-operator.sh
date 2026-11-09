#!/usr/bin/env bash
# Build an installable operator without changing the manifest-based service build.
set -euo pipefail
source "$(dirname "$0")/../scripts/workspace.sh"
[[ $# == 3 && "$1" =~ ^(relayer|reporter)$ && "$2" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$ && "$3" == /* && ! -e "$3" ]] || {
  echo 'usage: build-operator.sh relayer|reporter VERSION /new/absolute/directory' >&2; exit 2
}
role=$1 version=$2 out=$3
[[ "$(uname -sm)" == 'Linux x86_64' ]] || { echo 'Build on Linux x86-64.' >&2; exit 1; }
cd "$NODE_SINK_DIR"
[[ -z "$(git status --porcelain)" ]] || { echo 'Release source must be committed and clean.' >&2; exit 1; }
commit=$(git rev-parse HEAD)
bash "$solana_repo/deploy/prepare-services.sh"
stage=$(mktemp -d "$SOLANA_CACHE/operator-package.XXXXXX")
trap 'rm -rf "$stage"' EXIT
package=$stage/package
mkdir -p "$package/bin" "$package/.cache/solana/target/idl" "$package/.cache/solana/target/types"
git archive "$commit" contracts/solana/local contracts/solana/reporter contracts/solana/shared \
  contracts/solana/deploy/operator.ts contracts/solana/deploy/keys.ts \
  contracts/solana/deploy/service-config.ts contracts/solana/idls contracts/solana/package.json \
  contracts/solana/package-lock.json contracts/solana/.npmrc LICENSE | tar -x -C "$package"
cp "$SOLANA_CACHE/chain-sdk.tgz" "$package/.cache/solana/"
cp "$CARGO_TARGET_DIR/idl/akt.json" "$package/.cache/solana/target/idl/"
cp "$CARGO_TARGET_DIR/types/akt.ts" "$package/.cache/solana/target/types/"
types=.cache/solana/deps/ibc-contracts/ibc-solana/target/types
mkdir -p "$package/$types"
cp "$SOLANA_DEPS/ibc-contracts/ibc-solana/target/types/"{ics07_tendermint,access_manager}.ts "$package/$types/"
find "$package/contracts/solana" -name '*.test.ts' -delete
rm -rf "$package/contracts/solana/reporter/testdata"
(
  cd "$package/contracts/solana"
  npm ci --omit=dev --ignore-scripts
)
rm "$package/.cache/solana/chain-sdk.tgz"
bash "$solana_repo/deploy/node-runtime.sh" "$package/runtime"
if [[ "$role" == relayer ]]; then
  (cd "$solana_repo"; cargo build --locked --release --manifest-path tools/header-encoder/Cargo.toml)
  install -m 755 "$CARGO_TARGET_DIR/release/akt-header-encoder" "$package/bin/"
fi
install -m 755 "$solana_repo/deploy/operator.sh" "$package/bin/akt-$role"
cp -R "_run/akt-$role" "$package/deploy"
sed -i "s|^OPERATOR_IMAGE=.*|OPERATOR_IMAGE=ghcr.io/akash-network/akt-$role:$version|" "$package/deploy/.env.example"
cp contracts/solana/docs/operator-releases.md "$package/README.md"
cp contracts/solana/docs/operator-configuration.md "$package/"
for doc in sdk-integration operators reporter; do
  sed -i "s|($doc.md|(https://github.com/akash-network/node/blob/$commit/contracts/solana/docs/$doc.md|g" \
    "$package/README.md" "$package/operator-configuration.md"
done
{
  printf 'service akt-%s\nversion %s\ncommit %s\nplatform linux_amd64\n' "$role" "$version" "$commit"
  sha256sum "$SOLANA_CACHE/chain-sdk.tgz"
  cd "$package"
  sha256sum bin/* runtime/bin/node .cache/solana/target/idl/akt.json
} > "$package/service-build-info.txt"
chmod -R a+rX,go-w "$package"
bash "$solana_repo/deploy/test-operator.sh" "$package" "$role"
mkdir -p "$(dirname "$out")"
mv "$package" "$out"
