#!/usr/bin/env bash
# Builds akt, ics07_tendermint and access_manager from a committed
# revision in the pinned linux/amd64 image. Sources are copied to /build in the
# container, so the hashes do not depend on where this repository is checked
# out. The image stays local; never push it.
#
# Usage: bash scripts/verifiable-build.sh [--rev <commit>] [output-dir]
# Writes the three programs, IDLs, TS interfaces, SHA256SUMS and build-info.txt
# (default .cache/solana/verifiable-build/<commit> in the node repository).
set -euo pipefail
source "$(dirname "$0")/workspace.sh"
cd "$solana_repo"

rev=HEAD
if [[ "${1:-}" == --rev ]]; then
  rev=${2:?--rev needs a commit}
  shift 2
fi
commit=$(git rev-parse --verify "$rev^{commit}")
out=${1:-$SOLANA_CACHE/verifiable-build/$commit}
if [[ -e "$out" ]] && [[ -n "$(ls -A "$out")" ]]; then
  echo "Output directory is not empty: $out" >&2
  exit 1
fi
mkdir -p "$out"
out=$(cd "$out" && pwd)

stage=$(mktemp -d "$SOLANA_CACHE/verifiable.XXXXXX")
trap 'rm -rf "$stage"' EXIT
# Only committed files reach the build, never the working tree.
git archive --format=tar -o "$stage/source.tar" "$commit" \
  Anchor.toml Cargo.toml Cargo.lock rust-toolchain.toml programs idls \
  scripts/fetch-light-client.py scripts/workspace.sh scripts/setup-light-client.sh \
  scripts/build-programs.sh tools/header-encoder/Cargo.toml tools/verifiable-build
mkdir "$stage/context"
tar -xf "$stage/source.tar" -C "$stage/context" --strip-components=2 tools/verifiable-build

image=akt-verifiable-build:${commit:0:12}
docker build --platform linux/amd64 -t "$image" "$stage/context"
docker run --rm --platform linux/amd64 \
  -e HOST_UID="$(id -u)" -e HOST_GID="$(id -g)" \
  -v "$stage/source.tar:/input/source.tar:ro" -v "$out:/output" \
  "$image" bash -c 'mkdir -p /build/contracts/solana && tar -xf /input/source.tar -C /build/contracts/solana &&
    exec bash /build/contracts/solana/tools/verifiable-build/build.sh'

{
  echo "commit $commit"
  echo "image $(docker image inspect --format '{{.Id}}' "$image")"
  cat "$out/toolchain.txt"
} > "$out/build-info.txt"
rm "$out/toolchain.txt"
echo "Verifiable build of $commit in $out:"
cat "$out/SHA256SUMS"
