#!/usr/bin/env bash
# Reproduce the reviewed SDK input without machine-specific build metadata.
set -euo pipefail
umask 022
source "$(dirname "$0")/workspace.sh"
revision=3b977f0785eef4a5cfdd06747a8084f2cd5161ba
stage=$(mktemp -d "$SOLANA_CACHE/sdk.XXXXXX")
trap 'rm -rf "$stage"' EXIT
curl --fail --location --retry 3 --connect-timeout 15 --max-time 180 \
  "https://codeload.github.com/akash-network/chain-sdk/tar.gz/$revision" -o "$stage/source.tar.gz"
tar -xzf "$stage/source.tar.gz" -C "$stage" --strip-components=1
(
  cd "$stage/ts"
  npm ci
  npm run build
)
# npm's portable packer fixes archive metadata. Incremental state is host-specific.
rm "$stage/ts/dist/tsconfig.build.tsbuildinfo"
(
  cd "$stage/ts"
  npm pack --ignore-scripts --json --pack-destination "$stage" > "$stage/pack.json"
)
node - "$stage" "$SOLANA_CACHE/chain-sdk.tgz" "$solana_repo/package-lock.json" <<'JS'
const { readFileSync, renameSync } = require("node:fs");
const { join } = require("node:path");
const { createHash } = require("node:crypto");
const [stage, destination, lock] = process.argv.slice(2);
const [{ filename }] = JSON.parse(readFileSync(join(stage, "pack.json"), "utf8"));
const archive = join(stage, filename);
const actual = "sha512-" + createHash("sha512").update(readFileSync(archive)).digest("base64");
const expected = JSON.parse(readFileSync(lock, "utf8"))
  .packages["node_modules/@akashnetwork/chain-sdk"].integrity;
if (actual !== expected) {
  throw new Error(`SDK archive differs from package-lock.json: ${actual}`);
}
renameSync(archive, destination);
JS
