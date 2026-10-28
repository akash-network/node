#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
upstream=.deps/ibc-contracts
# Applied in order as a series. A later patch may change lines an earlier one
# added, so the tree is checked against each prefix of the series.
patches=(
  patches/ics07-signature-batches.patch
  patches/ics07-monotonic-latest-height.patch
  patches/ics07-consensus-pruning.patch
  patches/ics07-open-relaying.patch
)
test "$(cat "$upstream/.akt-revision")" = dc78ade828b766b218fb6bcb5417fdaf8e553021
# Exact context only. --forward also stops --batch from silently treating a
# half-applied file as reversed, so a reverse run proves every hunk applied.
strict=(--batch --forward --fuzz=0 -p1)

scratch=$(mktemp -d "${TMPDIR:-/tmp}/akt-patches.XXXXXX")
trap 'rm -rf "$scratch"' EXIT
files=$(sed -n 's|^+++ b/||p' "${patches[@]}" | sort -u)

# Copies the files the series touches, as they are now, into a scratch tree.
snapshot() {
  local file
  for file in $files; do
    if [[ -f "$upstream/$file" ]]; then
      mkdir -p "$1/$(dirname "$file")"
      cp "$upstream/$file" "$1/$file"
    fi
  done
}

# True when the tree holds exactly the first $1 patches: they reverse in order
# to the base, and the whole series then applies to that base.
holds() {
  local tree="$scratch/$1" i
  snapshot "$tree"
  for ((i = $1 - 1; i >= 0; i--)); do
    patch "${strict[@]}" --reverse -d "$tree" < "${patches[i]}" > /dev/null 2>&1 || return 1
  done
  # BSD patch leaves a file emptied by reversing its creation in place.
  find "$tree" -type f -empty -delete
  for ((i = 0; i < ${#patches[@]}; i++)); do
    patch "${strict[@]}" -d "$tree" < "${patches[i]}" > /dev/null 2>&1 || return 1
  done
}

applied=-1
for ((count = ${#patches[@]}; count >= 0; count--)); do
  if holds "$count"; then
    applied=$count
    break
  fi
done
if ((applied < 0)); then
  echo "Pinned light-client sources differ from the base and from every prefix of the patch series; preserve and inspect them before rebuilding." >&2
  exit 1
fi
for ((i = applied; i < ${#patches[@]}; i++)); do
  patch "${strict[@]}" -d "$upstream" < "${patches[i]}"
done
if command -v shasum > /dev/null; then
  shasum -a 256 "${patches[@]}" > "$upstream/.akt-patch-sha256"
else
  sha256sum "${patches[@]}" > "$upstream/.akt-patch-sha256"
fi
