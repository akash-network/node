#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

# Read only: download the publicly deployed Metaplex executable. Never deploy
# our programs or send private accounts to this RPC. Refuse upstream changes.
program=metaqbxxUerdq28cj1RbAWkYQm3ybzjb6a8bt518x1s
expected=31f0a627dba051a938de650464e55cc5397a4be0fd496929c1f9cf02fe5e9011
target=.deps/metaplex/mpl_token_metadata.so
mkdir -p .deps/metaplex
if [[ ! -f "$target" ]]; then
  temp=$(mktemp .deps/metaplex/download.XXXXXX)
  trap 'rm -f "$temp"' EXIT
  solana --url https://api.mainnet-beta.solana.com program dump "$program" "$temp"
  actual=$(shasum -a 256 "$temp" | cut -d ' ' -f 1)
  [[ "$actual" == "$expected" ]] || { echo 'Metaplex binary changed; review and pin it before use.' >&2; exit 1; }
  mv "$temp" "$target"
fi
actual=$(shasum -a 256 "$target" | cut -d ' ' -f 1)
[[ "$actual" == "$expected" ]] || { echo 'Metaplex checksum mismatch.' >&2; exit 1; }
echo "Verified Metaplex fixture: $expected"
