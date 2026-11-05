#!/usr/bin/env bash

set -euo pipefail
source "$(dirname "$0")/workspace.sh"

if [[ -z $solana_repo ]]; then
	echo "solana_repo var is not set"
	exit 1
fi

cd "$solana_repo"

failed=0
for tool in git curl tar python3 cc direnv make lsof node npm go rustup cargo rustc anchor solana solana-keygen solana-test-validator; do
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

versions=$(python3 - <<'PY'
import tomllib
with open("Anchor.toml", "rb") as source:
    anchor = tomllib.load(source)["toolchain"]
with open("rust-toolchain.toml", "rb") as source:
    rust = tomllib.load(source)["toolchain"]["channel"]
print(anchor["anchor_version"], anchor["solana_version"], rust)
PY
)
read -r anchor_version solana_version rust_version <<< "$versions"
check_version "$(node --version)" "v$(cut -d. -f1 .nvmrc)." Node
check_version "$(go version)" "$("$NODE_SINK_DIR/script/tools.sh" gotoolchain) " Go
check_version "$(rustc --version)" "rustc $rust_version " Rust
check_version "$(anchor --version)" "anchor-cli $anchor_version" Anchor
check_version "$(solana --version)" "solana-cli $solana_version " Solana

if [[ ! -f "$NODE_SINK_DIR/x/sink/module.go" ]]; then
	echo "Missing node/x/sink; use the matching node checkout." >&2
	failed=1
fi
test "$failed" == 0
echo 'Toolchain and repositories are ready.'
