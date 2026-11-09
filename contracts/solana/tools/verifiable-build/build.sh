#!/usr/bin/env bash
# Runs inside the pinned image, started by scripts/verifiable-build.sh. The
# committed sources are already unpacked at /build; everything is built there,
# so the output does not depend on the host checkout path.
set -euo pipefail
upstream=/build/.cache/solana/deps/ibc-contracts
out=/output
cd /build/contracts/solana
source scripts/workspace.sh
python3 scripts/fetch-light-client.py > "$out/upstream.txt"

inputs=(
  Cargo.lock Anchor.toml programs/akt/src/lib.rs
  "$upstream/ibc-solana/Cargo.lock"
  "$upstream/ibc-solana/Anchor.toml"
  "$upstream/ibc-solana/programs/ics07-tendermint/src/lib.rs"
  "$upstream/ibc-solana/programs/access-manager/src/lib.rs"
)
before=$(sha256sum "${inputs[@]}")
bash scripts/setup-light-client.sh
bash scripts/build-programs.sh
# Builds must preserve the committed dependency locks and program identities,
# and must reproduce the committed CPI interfaces.
if [[ "$(sha256sum "${inputs[@]}")" != "$before" ]]; then
  echo "A build changed a committed lockfile or program identity" >&2
  exit 1
fi
cmp idls/ics07_tendermint.json "$upstream/ibc-solana/target/idl/ics07_tendermint.json"
cmp idls/access_manager.json "$upstream/ibc-solana/target/idl/access_manager.json"

cp "$CARGO_TARGET_DIR/deploy/akt.so" \
  "$upstream/ibc-solana/target/deploy/ics07_tendermint.so" \
  "$upstream/ibc-solana/target/deploy/access_manager.so" "$out/"
cp "$CARGO_TARGET_DIR/idl/akt.json" \
  "$upstream/ibc-solana/target/idl/ics07_tendermint.json" \
  "$upstream/ibc-solana/target/idl/access_manager.json" "$out/"
cp "$CARGO_TARGET_DIR/types/akt.ts" \
  "$upstream/ibc-solana/target/types/ics07_tendermint.ts" \
  "$upstream/ibc-solana/target/types/access_manager.ts" "$out/"
(cd "$out" && sha256sum akt.so ics07_tendermint.so access_manager.so \
  akt.json ics07_tendermint.json access_manager.json \
  akt.ts ics07_tendermint.ts access_manager.ts > SHA256SUMS)
{
  cat "$out/upstream.txt"
  solana --version
  cargo-build-sbf --version
  anchor --version
  rustc --version
} > "$out/toolchain.txt"
rm "$out/upstream.txt"
if [[ -n "${HOST_UID:-}" ]]; then
  chown "$HOST_UID:${HOST_GID:-$HOST_UID}" "$out"/*
fi
cat "$out/SHA256SUMS"
