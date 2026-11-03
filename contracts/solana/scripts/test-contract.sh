#!/usr/bin/env bash

set -euo pipefail
source "$(dirname "$0")/workspace.sh"
cd "$solana_repo"
port=${AKT_TEST_RPC_PORT:-19899}
port_base=${AKT_TEST_PORT_BASE:-19800}
if [[ ! "$port_base" =~ ^[0-9]+$ ]] || ((port_base < 1025 || port_base > 65400)); then
  echo "Invalid AKT_TEST_PORT_BASE" >&2; exit 1
fi
if lsof -nP -iTCP:"$port_base-$((port_base + 50))" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Test auxiliary ports are occupied; choose AKT_TEST_PORT_BASE" >&2; exit 1
fi

if [[ ! "$port" =~ ^[0-9]+$ ]] || ((port < 1025 || port > 65534)); then
	echo 'AKT_TEST_RPC_PORT must be an unprivileged port with room for WebSocket.' >&2
	exit 1
fi

if lsof -nP -iTCP:"$port" -iTCP:"$((port + 1))" -sTCP:LISTEN >/dev/null 2>&1; then
	echo 'Test RPC or WebSocket port is occupied. Choose AKT_TEST_RPC_PORT.' >&2
	exit 1
fi

bash scripts/setup-metadata.sh
if [[ ${AKT_TEST_SKIP_BUILD:-0} != 1 ]]; then
  bash scripts/build-programs.sh
fi
test -f "$CARGO_TARGET_DIR/deploy/akt_solana.so"
test -f "$CARGO_TARGET_DIR/idl/akt_solana.json"

upstream="$SOLANA_DEPS/ibc-contracts/ibc-solana"
artifacts="$upstream/target/deploy"

for file in "$artifacts/ics07_tendermint.so" "$artifacts/access_manager.so"; do
	test -f "$file" || {
		echo 'Run npm run setup first.' >&2
		exit 1
	}
done

# Upstream's generic CPI proxy, at the pinned revision, calls mint and void
# from another program.
if [[ ! -f "$artifacts/test_cpi_proxy.so" ]]; then
	(cd "$upstream" && CARGO_TARGET_DIR="$upstream/target" RUSTUP_TOOLCHAIN=1.89.0 anchor build --program-name test_cpi_proxy)
fi

mkdir -p "$SOLANA_RUN"
run=$(mktemp -d "$SOLANA_RUN/contract-test.XXXXXX")
solana-keygen new --no-bip39-passphrase --silent --outfile "$run/payer-keypair.json"
authority=$(solana-keygen pubkey "$run/payer-keypair.json")
# Re-pin tests need more verifier deployments, each with its own client PDA.
repin_client=$(node_modules/.bin/tsx tests/relocate-program.ts "$artifacts/ics07_tendermint.so" "$run/ics07_repin.so")
frozen_client=$(node_modules/.bin/tsx tests/relocate-program.ts "$artifacts/ics07_tendermint.so" "$run/ics07_frozen.so" frozen)

solana-test-validator --ledger "$run/ledger" --bind-address 127.0.0.1 \
	--rpc-port "$port" --gossip-port "$((port_base + 1))" --dynamic-port-range "$((port_base + 2))-$((port_base + 50))" --faucet-port "$port_base" --mint "$authority" \
	--bpf-program metaqbxxUerdq28cj1RbAWkYQm3ybzjb6a8bt518x1s "$SOLANA_DEPS/metaplex/mpl_token_metadata.so" \
	--bpf-program CtQLLKbDMt1XVNXtLKJEt1K8cstbckjqE6zyFqR37KTc "$artifacts/test_cpi_proxy.so" \
	--upgradeable-program FfFAPUKqjf68ggj1HvpyzMHFDRnT2cFAdZYYQb7mK9i5 "$CARGO_TARGET_DIR/deploy/akt_solana.so" "$authority" \
	--upgradeable-program HqPcGpVHxNNFfVatjhG78dFVMwjyZixoKPdZSt3d3TdD "$artifacts/ics07_tendermint.so" "$authority" \
	--upgradeable-program 4fMih2CidrXPeRx77kj3QcuBZwREYtxEbXjURUgadoe1 "$artifacts/access_manager.so" "$authority" \
	--upgradeable-program "$repin_client" "$run/ics07_repin.so" "$authority" \
	--upgradeable-program "$frozen_client" "$run/ics07_frozen.so" "$authority" \
	--quiet >"$run/validator.log" 2>&1 &
validator_pid=$!

cleanup() {
	kill "$validator_pid" 2>/dev/null || true
	wait "$validator_pid" 2>/dev/null || true
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
ready=0

# shellcheck disable=SC2034
for attempt in $(seq 1 120); do
	if ! kill -0 "$validator_pid" 2>/dev/null; then
		echo "Validator exited; inspect $run/validator.log" >&2
		exit 1
	fi
	if solana --url "http://127.0.0.1:$port" cluster-version >/dev/null 2>&1; then
		ready=1
		break
	fi
	sleep 1
done

test "$ready" == 1 || {
	echo "Validator not ready; inspect $run/validator.log" >&2
	exit 1
}

export ANCHOR_PROVIDER_URL="http://127.0.0.1:$port"
export ANCHOR_WALLET="$run/payer-keypair.json"

echo "Contract test evidence: $run"

node_modules/.bin/tsx --test tests/*.test.ts
