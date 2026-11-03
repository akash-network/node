#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/workspace.sh"
cd "$solana_repo"
test "$(cat "$SOLANA_DEPS/ibc-contracts/.akt-revision")" = dc78ade828b766b218fb6bcb5417fdaf8e553021
anchor build -i "$CARGO_TARGET_DIR/idl" -t "$CARGO_TARGET_DIR/types" "$@"
