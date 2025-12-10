#!/usr/bin/env bash

set -euo pipefail

if [[ -z "$AKASH_HOME" ]]; then
	echo "AKASH_HOME is not set"
	exit 1
fi

if [[ -z "$KEYS" ]]; then
	echo "KEYS is not set"
	exit 1
fi

if [[ -z "$MNEMONIC" ]]; then
	echo "MNEMONIC is not set"
	exit 1
fi

if [[ -z "$CONTRACTS_DIR" ]]; then
	echo "CONTRACTS_DIR is not set"
	exit 1
fi

WORMHOLE_WASM="${CONTRACTS_DIR}/wormhole/artifacts/wormhole.wasm"
PYTH_WASM="${CONTRACTS_DIR}/pyth/artifacts/pyth.wasm"

HERMES_MNEMONIC="wire museum tragic inmate final lady illegal father whisper margin sea cool soul half moon nut tissue strategy ladder come glory opera device elbow"

GENESIS_PATH="$AKASH_HOME/config/genesis.json"

CHAIN_MIN_DEPOSIT=10000000000000
CHAIN_ACCOUNT_DEPOSIT=$((CHAIN_MIN_DEPOSIT * 10))
CHAIN_VALIDATOR_DELEGATE=$((CHAIN_MIN_DEPOSIT / 2))
CHAIN_TOKEN_DENOM=uakt
mapfile -t ACCOUNTS <<< "$KEYS"

# Pyth configuration
AKT_PRICE_FEED_ID="0x4ea5bb4d2f5900cc2e97ba534240950740b4d3b89fe712a94a7304fd2fd92702"
PYTH_EMITTER_CHAIN="26"  # Pythnet
PYTH_EMITTER_ADDRESS="e101faedac5851e32b9b23b5f9411a8c2bac4aae3ed4dd7b811dd1a72ea4aa71"


# Wormhole Mainnet Guardian Set 4 (19 guardians)
GUARDIAN_ADDRESSES=(
	"5893B5A76c3f739645648885bDCcC06cd70a3Cd3"
	"fF6CB952589BDE862c25Ef4392132fb9D4A42157"
	"114De8460193bdf3A2fCf81f86a09765F4762fD1"
	"107A0086b32d7A0977926A205131d8731D39cbEB"
	"8C82B2fd82FaeD2711d59AF0F2499D16e726f6b2"
	"11b39756C042441BE6D8650b69b54EbE715E2343"
	"54Ce5B4D348fb74B958e8966e2ec3dBd4958a7cd"
	"15e7cAF07C4e3DC8e7C469f92C8Cd88FB8005a20"
	"74a3bf913953D695260D88BC1aA25A4eeE363ef0"
	"000aC0076727b35FBea2dAc28fEE5cCB0fEA768e"
	"AF45Ced136b9D9e24903464AE889F5C8a723FC14"
	"f93124b7c738843CBB89E864c862c38cddCccF95"
	"D2CC37A4dc036a8D232b48f62cDD4731412f4890"
	"DA798F6896A3331F64b48c12D1D57Fd9cbe70811"
	"71AA1BE1D36CaFE3867910F99C09e347899C19C3"
	"8192b6E7387CCd768277c17DAb1b7a5027c0b3Cf"
	"178e21ad2E77AE06711549CFBB1f9c7a9d8096e8"
	"5E1487F35515d02A92753504a8D75471b9f49EdB"
	"6FbEBc898F403E4773E95feB15E80C9A99c8348d"
)

log() {
	echo "[$(date -u '+%Y-%m-%d %H:%M:%S UTC')] $*"
}

wait_for_block() {
	local target=${1:-1}
	log "Waiting for block $target..."
	while true; do
		local height
		height=$(curl -s http://localhost:26657/status 2>/dev/null | jq -r '.result.sync_info.latest_block_height // "0"') || height="0"
		if [ "$height" -ge "$target" ] 2>/dev/null; then
			log "Block $height reached"
			return 0
		fi
		sleep 1
	done
}

configure_genesis() {
	log "Configuring genesis..."

	cp "${GENESIS_PATH}" "${GENESIS_PATH}.orig"

	# Build guardian addresses JSON array
	local guardian_json="["
	for i in "${!GUARDIAN_ADDRESSES[@]}"; do
		if [ "$i" -gt 0 ]; then
			guardian_json+=","
		fi
		guardian_json+="\"${GUARDIAN_ADDRESSES[$i]}\""
	done
	guardian_json+="]"

	cat "${GENESIS_PATH}.orig" | \
		jq -M '.app_state.gov.voting_params.voting_period = "60s"' \
		| jq -M '.app_state.gov.params.voting_period = "60s"' \
		| jq -M '.app_state.gov.params.expedited_voting_period = "30s"' \
		| jq -M '.app_state.gov.params.max_deposit_period = "60s"' \
		| jq -M '.app_state.wasm.params.code_upload_access.permission = "Everybody"' \
		| jq -M '.app_state.wasm.params.instantiate_default_permission = "Everybody"' \
		| jq -M --argjson guardians "$guardian_json" --arg feed_id "$AKT_PRICE_FEED_ID" '
			.app_state.oracle.params.min_price_sources = 1 |
			.app_state.oracle.params.max_price_staleness_blocks = 100 |
			.app_state.oracle.params.twap_window = 50 |
			.app_state.oracle.params.max_price_deviation_bps = 1000 |
			.app_state.oracle.params.feed_contracts_params = [
				{
					"@type": "/akash.oracle.v1.PythContractParams",
					"akt_price_feed_id": $feed_id
				},
				{
					"@type": "/akash.oracle.v1.WormholeContractParams",
					"guardian_addresses": $guardians
				}
			]' \
	> "${GENESIS_PATH}"

	log "Genesis configuration complete"
}

init_node() {
	log "Initializing Akash node..."

	akash genesis init "node0"

	configure_genesis

	for i in "${!ACCOUNTS[@]}"; do
		echo "$MNEMONIC" | akash keys add "${ACCOUNTS[$i]}" --index "$i" --recover
		akash genesis add-account "$(akash keys show "${ACCOUNTS[$i]}" -a)" "${CHAIN_ACCOUNT_DEPOSIT}${CHAIN_TOKEN_DENOM}"
	done

	echo "$HERMES_MNEMONIC" | akash keys add hermes --recover
	akash genesis add-account "$(akash keys show hermes -a)" "${CHAIN_MIN_DEPOSIT}${CHAIN_TOKEN_DENOM}"

	akash genesis gentx validator "${CHAIN_VALIDATOR_DELEGATE}${CHAIN_TOKEN_DENOM}" --min-self-delegation=1 --gas=auto --gas-prices=0.025${CHAIN_TOKEN_DENOM}

	akash genesis collect
	akash genesis validate

	log "Genesis initialized successfully"
}

start_node_background() {
	log "Starting Akash node in background..."
	akash start --home "$AKASH_HOME" --pruning=nothing &
	NODE_PID=$!
	log "Node started with PID $NODE_PID"
}

deploy_contracts() {
	log "Deploying contracts..."

	# Wait for node to be ready
	wait_for_block 3

	local admin_key=main
	local admin_addr
	admin_addr=$(akash keys show $admin_key -a)

	# Check if contract files exist
	if [ ! -f "$WORMHOLE_WASM" ]; then
		log "ERROR: Wormhole contract not found at $WORMHOLE_WASM"
		log "Skipping contract deployment. Build contracts first with: cd contracts && make build"
		write_hermes_config "CONTRACT_NOT_DEPLOYED"
		return 1
	fi

	if [ ! -f "$PYTH_WASM" ]; then
		log "ERROR: Pyth contract not found at $PYTH_WASM"
		log "Skipping contract deployment. Build contracts first with: cd contracts && make build"
		write_hermes_config "CONTRACT_NOT_DEPLOYED"
		return 1
	fi

	# Deploy Wormhole contract
	log "Storing Wormhole contract..."
	akash tx wasm store "$WORMHOLE_WASM" --from $admin_key -o json

	local wormhole_code_id
	wormhole_code_id=$(akash query wasm list-code -o json | jq -r '.code_infos[-1].code_id')
	log "Wormhole code ID: $wormhole_code_id"

	# Instantiate Wormhole contract
	# Note: Guardian addresses are loaded from x/oracle params by the contract
	local wormhole_init_msg='{
		"gov_chain": 1,
		"gov_address": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQ=",
		"chain_id": 29,
		"fee_denom": "uakt"
	}'

	log "Instantiating Wormhole contract..."
	akash tx wasm instantiate "$wormhole_code_id" "$wormhole_init_msg" \
		--label "wormhole-local" \
		--admin "$admin_addr" \
		--from $admin_key \

	local wormhole_addr
	wormhole_addr=$(akash query wasm list-contract-by-code "$wormhole_code_id" -o json | jq -r '.contracts[-1]')
	log "Wormhole contract address: $wormhole_addr"

	# Deploy Pyth contract
	log "Storing Pyth contract..."
	akash tx wasm store "$PYTH_WASM" --from $admin_key

	local pyth_code_id
	pyth_code_id=$(akash query wasm list-code -o json | jq -r '.code_infos[-1].code_id')
	log "Pyth code ID: $pyth_code_id"

	# Instantiate Pyth contract
	local pyth_init_msg
	pyth_init_msg=$(cat <<EOF
{
	"admin": "$admin_addr",
	"wormhole_contract": "$wormhole_addr",
	"update_fee": "1000000",
	"price_feed_id": "$AKT_PRICE_FEED_ID",
	"data_sources": [
		{
			"emitter_chain": $PYTH_EMITTER_CHAIN,
			"emitter_address": "$PYTH_EMITTER_ADDRESS"
		}
	]
}
EOF
)

	log "Instantiating Pyth contract..."
	akash tx wasm instantiate "$pyth_code_id" "$pyth_init_msg" \
		--label "pyth" \
		--admin "$admin_addr" \
		--from $admin_key

	local pyth_addr
	pyth_addr=$(akash query wasm list-contract-by-code "$pyth_code_id" -o json | jq -r '.contracts[-1]')
	log "Pyth contract address: $pyth_addr"

	# Register Pyth as authorized oracle source
	register_oracle_source "$pyth_addr"

	# Write configuration for Hermes
	write_hermes_config "$pyth_addr"

	log "Contract deployment complete!"
	log "  Wormhole: $wormhole_addr"
	log "  Pyth:     $pyth_addr"
}

register_oracle_source() {
	local pyth_addr=$1
	log "Registering Pyth contract as authorized oracle source..."

	# Build guardian addresses JSON array for the proposal
	local guardian_json="["
	for i in "${!GUARDIAN_ADDRESSES[@]}"; do
		if [ "$i" -gt 0 ]; then
			guardian_json+=","
		fi
		guardian_json+="\"${GUARDIAN_ADDRESSES[$i]}\""
	done
	guardian_json+="]"

	# Create proposal JSON
	cat > /tmp/oracle-params.json <<EOF
{
	"messages": [
		{
			"@type": "/akash.oracle.v1.MsgUpdateParams",
			"authority": "akash10d07y265gmmuvt4z0w9aw880jnsr700jhe7z0f",
			"params": {
				"sources": ["$pyth_addr"],
				"min_price_sources": 1,
				"max_price_staleness_blocks": 100,
				"twap_window": 50,
				"max_price_deviation_bps": 1000,
				"feed_contracts_params": [
					{
						"@type": "/akash.oracle.v1.PythContractParams",
						"akt_price_feed_id": "$AKT_PRICE_FEED_ID"
					},
					{
						"@type": "/akash.oracle.v1.WormholeContractParams",
						"guardian_addresses": $guardian_json
					}
				]
			}
		}
	],
	"deposit": "10000000uakt",
	"title": "Register Pyth Contract",
	"summary": "Authorize pyth contract as oracle source"
}
EOF

	# Submit proposal
	log "Submitting oracle params proposal..."
	akash tx gov submit-proposal /tmp/oracle-params.json --from main \

	sleep 3

	# Vote yes on the proposal
	log "Voting yes on proposal..."
	akash tx gov vote 1 yes --from validator

	# Wait for proposal to pass (30s voting period)
	log "Waiting for governance proposal to pass..."
	sleep 70

	log "Oracle source registration complete"
}

write_hermes_config() {
	local oracle_addr=$1

	log "Writing Hermes configuration to $AKASH_RUN_DIR/hermes.env..."

	cat > "$AKASH_RUN_DIR/hermes.env" <<EOF
# Generated by akash-node init script
# Contract deployed at $(date -u '+%Y-%m-%d %H:%M:%S UTC')

CONTRACT_ADDRESS="$oracle_addr"
MNEMONIC="$HERMES_MNEMONIC"
EOF

	log "Hermes configuration written successfully"
}

main() {
	log "=== Akash Local Node Initialization ==="
	log "Chain ID: $AKASH_CHAIN_ID"

	# Check if already initialized and running
	if [ -f "$AKASH_HOME/config/genesis.json" ] && [ -f "$SHARED_DIR/hermes.env" ]; then
		log "Node already initialized, starting..."
		exec akash start --home "$AKASH_HOME" --pruning=nothing
	fi

	# Initialize node
	init_node

	# Start node in background for contract deployment
	start_node_background

	# Deploy contracts (runs after node starts producing blocks)
	deploy_contracts || log "Contract deployment failed or skipped"

	log "=== Initialization Complete ==="
	log "Node is running. Hermes can now connect."

	kill -SIGINT $NODE_PID
	# Keep the script running (wait for node process)
	wait $NODE_PID
}

main "$@"
