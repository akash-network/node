#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

revision=dc78ade828b766b218fb6bcb5417fdaf8e553021
upstream=.deps/ibc-contracts
if [[ ! -f "$upstream/.akt-revision" ]] || [[ "$(cat "$upstream/.akt-revision")" != "$revision" ]]; then
  if [[ -e "$upstream" ]]; then
    echo "Existing upstream directory has no matching revision marker: $upstream" >&2
    echo "Move it aside before fetching the pinned source." >&2
    exit 1
  fi
  archive=$(mktemp "${TMPDIR:-/tmp}/akt-ibc.XXXXXX")
  trap 'rm -f "$archive"' EXIT
  curl -fL --retry 3 "https://api.github.com/repos/cosmos/ibc-contracts/tarball/$revision" -o "$archive"
  mkdir -p "$upstream"
  tar -xzf "$archive" --strip-components=1 -C "$upstream"
  printf '%s\n' "$revision" > "$upstream/.akt-revision"
fi

bash scripts/apply-light-client-patches.sh

(
  cd "$upstream/ibc-solana"
  RUSTUP_TOOLCHAIN=1.89.0 anchor build --program-name ics07_tendermint
  RUSTUP_TOOLCHAIN=1.89.0 anchor build --program-name access_manager
)

# The checked-in IDLs define the CPI ABI. Rebuilds must produce the same files.
cmp idls/ics07_tendermint.json "$upstream/ibc-solana/target/idl/ics07_tendermint.json"
cmp idls/access_manager.json "$upstream/ibc-solana/target/idl/access_manager.json"
echo "Pinned light client and access manager are ready for local tests."
