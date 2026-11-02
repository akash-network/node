#!/usr/bin/env bash
# Runs inside the pinned image, started by scripts/verifiable-build.sh. The
# committed sources are already unpacked at /build; everything is built there,
# so the output does not depend on the host checkout path.
set -euo pipefail
revision=dc78ade828b766b218fb6bcb5417fdaf8e553021
# Content hash of the upstream tarball's files and symlinks (tree_hash below).
# GitHub may recompress tarballs; the content must not change.
upstream_tree=6dd68161cdeee00d8dee67cb7fee48c76d14c38f8e75d0f48f64418f062c115f
upstream=.deps/ibc-contracts
out=/output

tree_hash() {
  (
    cd "$1"
    find . -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum
    find . -type l -print0 | LC_ALL=C sort -z | while IFS= read -r -d '' link; do
      printf 'link %s %s\n' "$link" "$(readlink "$link")"
    done
  ) | sha256sum | cut -d' ' -f1
}

if [[ "${1:-}" == --tree-hash ]]; then
  tree_hash "$2"
  exit 0
fi

cd /build

mkdir -p "$upstream"
curl -fsSL --retry 3 "https://api.github.com/repos/cosmos/ibc-contracts/tarball/$revision" |
  tar -xz --strip-components=1 -C "$upstream"
actual=$(tree_hash "$upstream")
if [[ "$actual" != "$upstream_tree" ]]; then
  echo "Upstream source content $actual does not match the pinned $upstream_tree" >&2
  exit 1
fi
printf '%s\n' "$revision" > "$upstream/.akt-revision"
bash scripts/apply-light-client-patches.sh

locks=(Cargo.lock "$upstream/ibc-solana/Cargo.lock")
before=$(sha256sum "${locks[@]}")
(
  cd "$upstream/ibc-solana"
  anchor build --program-name ics07_tendermint
  anchor build --program-name access_manager
)
anchor build --program-name akt_solana
# Builds must not resolve dependencies differently from the committed locks,
# and must reproduce the committed CPI interfaces.
if [[ "$(sha256sum "${locks[@]}")" != "$before" ]]; then
  echo "A build changed a committed Cargo.lock" >&2
  exit 1
fi
cmp idls/ics07_tendermint.json "$upstream/ibc-solana/target/idl/ics07_tendermint.json"
cmp idls/access_manager.json "$upstream/ibc-solana/target/idl/access_manager.json"

cp target/deploy/akt_solana.so \
  "$upstream/ibc-solana/target/deploy/ics07_tendermint.so" \
  "$upstream/ibc-solana/target/deploy/access_manager.so" "$out/"
(cd "$out" && sha256sum akt_solana.so ics07_tendermint.so access_manager.so > SHA256SUMS)
{
  echo "upstream $revision"
  echo "upstream-tree $actual"
  cat "$upstream/.akt-patch-sha256"
  solana --version
  cargo-build-sbf --version
  anchor --version
  rustc --version
} > "$out/toolchain.txt"
if [[ -n "${HOST_UID:-}" ]]; then
  chown "$HOST_UID:${HOST_GID:-$HOST_UID}" "$out"/*
fi
cat "$out/SHA256SUMS"
