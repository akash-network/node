#!/usr/bin/env bash
# Checks that a deployed upgradeable program holds exactly one build: the
# first bytes of its program data hash to the binary's SHA-256 and the rest is
# zero padding. Optionally checks the upgrade authority. Read-only.
#
# Usage: verify-program.sh --url RPC PROGRAM_ID BINARY [EXPECTED_AUTHORITY]
# Prints: PROGRAM_ID SHA256 AUTHORITY
set -euo pipefail

if [[ "${1:-}" != --url ]] || (($# < 4 || $# > 5)); then
  echo 'usage: verify-program.sh --url RPC PROGRAM_ID BINARY [EXPECTED_AUTHORITY]' >&2
  exit 2
fi
url=$2 id=$3 binary=$4 expected_authority=${5:-}
test -f "$binary" || { echo "missing $binary" >&2; exit 1; }

sha256() {
  if command -v sha256sum > /dev/null; then
    sha256sum | cut -d' ' -f1
  else
    shasum -a 256 | cut -d' ' -f1
  fi
}

# Hosts without a default CLI signer pass any keypair for these read-only calls.
signer=()
[[ -z "${SOLANA_KEYPAIR:-}" ]] || signer=(--keypair "$SOLANA_KEYPAIR")
work=$(mktemp -d "${TMPDIR:-/tmp}/akt-verify-program.XXXXXX")
trap 'rm -rf "$work"' EXIT
solana program dump "${signer[@]}" --url "$url" "$id" "$work/program.so" > /dev/null
size=$(wc -c < "$binary" | tr -d ' ')
dumped=$(wc -c < "$work/program.so" | tr -d ' ')
if ((dumped < size)); then
  echo "$id holds $dumped bytes, fewer than the $size-byte build" >&2
  exit 1
fi
expected=$(sha256 < "$binary")
actual=$(head -c "$size" "$work/program.so" | sha256)
if [[ "$actual" != "$expected" ]]; then
  echo "$id does not hold $binary: $actual != $expected" >&2
  exit 1
fi
padding=$(tail -c +"$((size + 1))" "$work/program.so" | LC_ALL=C tr -d '\000' | wc -c | tr -d ' ')
if [[ "$padding" != 0 ]]; then
  echo "$id has $padding non-zero bytes after the build" >&2
  exit 1
fi
authority=$(solana program show "${signer[@]}" --url "$url" "$id" --output json |
  python3 -I -c 'import json, sys; print(json.load(sys.stdin).get("authority") or "none")')
if [[ -n "$expected_authority" && "$authority" != "$expected_authority" ]]; then
  echo "$id upgrade authority is $authority, not $expected_authority" >&2
  exit 1
fi
echo "$id $actual $authority"
