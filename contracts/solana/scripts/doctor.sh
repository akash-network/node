#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/workspace.sh"
cd "$solana_repo"
failed=0
for tool in git curl tar patch python3 cc make lsof node npm go rustup cargo rustc anchor solana solana-keygen solana-test-validator; do
  if ! command -v "$tool" >/dev/null; then
    echo "Missing command: $tool" >&2
    failed=1
  fi
done
if [[ "$failed" == 1 ]]; then
  echo "Install prerequisites from docs/setup.md, then rerun npm run doctor." >&2
  exit 1
fi
check_version() {
  local actual="$1" expected="$2" name="$3"
  if [[ "$actual" != *"$expected"* ]]; then
    echo "$name: expected $expected, found $actual" >&2
    failed=1
  else
    echo "$actual"
  fi
}
check_version "$(node --version)" 'v24.' Node
check_version "$(go version)" 'go1.26.2 ' Go
check_version "$(rustc --version)" 'rustc 1.89.0 ' Rust
check_version "$(anchor --version)" 'anchor-cli 0.32.1' Anchor
check_version "$(solana --version)" 'solana-cli 2.3.13 ' Solana
for file in "$NODE_SINK_DIR/x/sink/module.go" "$CHAIN_SDK_DIR/go/go.mod" "$CHAIN_SDK_DIR/proto/node/akash/sink/v1/msgs.proto" "$CHAIN_SDK_DIR/go/cli/sink_tx.go"; do
  if [[ ! -f "$file" ]]; then
    echo "Missing $file; see README.md for the chain-sdk checkout." >&2
    failed=1
  fi
done
test "$failed" == 0
echo 'Toolchain and repositories are ready.'
