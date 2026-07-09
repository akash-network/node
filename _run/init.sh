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

PYTH_WASM="${CONTRACTS_DIR}/artifacts/pyth.wasm"
PYTH_PRO_VERIFIER_WASM="${CONTRACTS_DIR}/artifacts/pyth_pro_verifier.wasm"

HERMES_MNEMONIC="wire museum tragic inmate final lady illegal father whisper margin sea cool soul half moon nut tissue strategy ladder come glory opera device elbow"

GENESIS_PATH="$AKASH_HOME/config/genesis.json"

CHAIN_MIN_DEPOSIT=10000000000000
CHAIN_ACCOUNT_DEPOSIT=$((CHAIN_MIN_DEPOSIT * 10))
CHAIN_VALIDATOR_DELEGATE=$((CHAIN_MIN_DEPOSIT / 2))
CHAIN_TOKEN_DENOM=uakt
# shellcheck disable=SC2206
ACCOUNTS=($KEYS)

# Pyth configuration
AKT_PRICE_FEED_ID="0x4ea5bb4d2f5900cc2e97ba534240950740b4d3b89fe712a94a7304fd2fd92702"
PYTH_EMITTER_CHAIN="26" # Pythnet
PYTH_EMITTER_ADDRESS="e101faedac5851e32b9b23b5f9411a8c2bac4aae3ed4dd7b811dd1a72ea4aa71"

# Pyth Core upgraded router verifier configuration.
PYTH_PRO_ROUTER_SET_INDEX="0"
PYTH_PRO_EXPECTED_EMITTER_CHAIN="26"
PYTH_PRO_EXPECTED_EMITTER_ADDRESS="507974686e6574507974686e6574507974686e6574507974686e657450797468"
PYTH_PRO_ROUTER_ADDRESSES=(
	"41534bb176e461a3fb30479400f210549ecce638"
	"6502987b62f21cab7eb5ccd8f0173084b60d5b41"
	"44a3e8f6a382412cf6bb90a3f8106e68977476c9"
	"d9d7d4529577864352c9a6539a48238fcd447052"
	"1663a5a822336ece48559b1dfb1e93a017a7dac3"
)

log() {
	echo "[$(date -u '+%Y-%m-%d %H:%M:%S UTC')] $*"
}

hex_to_base64() {
	echo -n "$1" | xxd -r -p | base64
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

	# shellcheck disable=SC2002
	cat "${GENESIS_PATH}.orig" | \
		jq -M '.app_state.gov.voting_params.voting_period = "60s"' \
		| jq -M '.app_state.gov.params.voting_period = "60s"' \
		| jq -M '.app_state.gov.params.expedited_voting_period = "30s"' \
		| jq -M '.app_state.gov.params.max_deposit_period = "60s"' \
		| jq -M '.app_state.wasm.params.code_upload_access.permission = "Everybody"' \
		| jq -M '.app_state.wasm.params.instantiate_default_permission = "Everybody"' \
		| jq -M '
			.app_state.oracle.params.min_price_sources = 1 |
			.app_state.oracle.params.max_price_staleness_period = "60s" |
			.app_state.oracle.params.twap_window = "30s" |
			.app_state.oracle.params.max_price_deviation_bps = 1000 |
			.app_state.oracle.params.price_retention = "86400s" |
			.app_state.oracle.params.prune_epoch = "hour" |
			.app_state.oracle.params.max_prune_per_epoch = 1000' \
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
	if [ ! -f "$PYTH_WASM" ]; then
		log "ERROR: Pyth contract not found at $PYTH_WASM"
		log "Skipping contract deployment. Build contracts first with: cd contracts && make build"
		write_hermes_config "CONTRACT_NOT_DEPLOYED"
		return 1
	fi

	if [ ! -f "$PYTH_PRO_VERIFIER_WASM" ]; then
		log "ERROR: Pyth Pro verifier contract not found at $PYTH_PRO_VERIFIER_WASM"
		log "Skipping contract deployment. Build contracts first with: cd contracts && make build"
		write_hermes_config "CONTRACT_NOT_DEPLOYED"
		return 1
	fi

	# Deploy Pyth Pro verifier contract.
	log "Storing Pyth Pro verifier contract..."
	akash tx wasm store "$PYTH_PRO_VERIFIER_WASM" --from $admin_key

	local pyth_pro_verifier_code_id
	pyth_pro_verifier_code_id=$(akash query wasm list-code -o json | jq -r '.code_infos[-1].code_id')
	log "Pyth Pro verifier code ID: $pyth_pro_verifier_code_id"

	local router_json='['
	for i in "${!PYTH_PRO_ROUTER_ADDRESSES[@]}"; do
		if [ "$i" -gt 0 ]; then
			router_json+=','
		fi
		local router_b64
		router_b64=$(hex_to_base64 "${PYTH_PRO_ROUTER_ADDRESSES[$i]}")
		router_json+="{\"bytes\":\"$router_b64\"}"
	done
	router_json+=']'

	local pyth_pro_emitter_b64
	pyth_pro_emitter_b64=$(hex_to_base64 "$PYTH_PRO_EXPECTED_EMITTER_ADDRESS")

	local pyth_pro_verifier_init_msg
	pyth_pro_verifier_init_msg=$(cat <<EOF
{
	"router_set_index": $PYTH_PRO_ROUTER_SET_INDEX,
	"routers": $router_json,
	"expected_emitter_chain": $PYTH_PRO_EXPECTED_EMITTER_CHAIN,
	"expected_emitter_address": "$pyth_pro_emitter_b64"
}
EOF
)

	log "Instantiating Pyth Pro verifier contract..."
	akash tx wasm instantiate "$pyth_pro_verifier_code_id" "$pyth_pro_verifier_init_msg" \
		--label "pyth-pro-verifier" \
		--admin "$admin_addr" \
		--from $admin_key

	local pyth_pro_verifier_addr
	pyth_pro_verifier_addr=$(akash query wasm list-contract-by-code "$pyth_pro_verifier_code_id" -o json | jq -r '.contracts[-1]')
	log "Pyth Pro verifier contract address: $pyth_pro_verifier_addr"

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
	"wormhole_contract": "$pyth_pro_verifier_addr",
	"update_fee": "1000",
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

	# Register Pyth as authorized oracle source and fund BME vault via gov proposal
	register_oracle_source "$pyth_addr" "$admin_addr"

	# Write configuration for Hermes
	write_hermes_config "$pyth_addr"

	log "Contract deployment complete!"
	log "  Pyth Pro verifier: $pyth_pro_verifier_addr"
	log "  Pyth:     $pyth_addr"
}

register_oracle_source() {
	local pyth_addr=$1
	local admin_addr=$2
	log "Registering Pyth contract as authorized oracle source and funding BME vault..."

	# Create proposal JSON with both oracle params and BME vault funding
	cat > /tmp/oracle-params.json <<EOF
{
	"messages": [
		{
			"@type": "/akash.oracle.v2.MsgUpdateParams",
			"authority": "akash10d07y265gmmuvt4z0w9aw880jnsr700jhe7z0f",
			"params": {
				"sources": ["$pyth_addr"],
				"min_price_sources": 1,
				"max_price_staleness_period": "60s",
				"twap_window": "30s",
				"max_price_deviation_bps": 1000,
				"price_retention": "86400s",
				"prune_epoch": "hour",
				"max_prune_per_epoch": 1000
			}
		},
		{
			"@type": "/akash.bme.v1.MsgFundVault",
			"authority": "akash10d07y265gmmuvt4z0w9aw880jnsr700jhe7z0f",
			"amount": {"denom": "${CHAIN_TOKEN_DENOM}", "amount": "1000000000000"},
			"source": "$(akash keys show main -a)"
		}
	],
	"deposit": "10000000uakt",
	"title": "Register Pyth Contract and Fund BME Vault",
	"summary": "Authorize pyth contract as oracle source and seed BME vault with initial AKT"
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
	local pyth_addr=$1

	log "Writing Hermes configuration to $AKASH_RUN_DIR/hermes.env..."

	cat > "$AKASH_RUN_DIR/hermes.env" <<EOF
# Generated by akash-node init script
# Contract deployed at $(date -u '+%Y-%m-%d %H:%M:%S UTC')

CONTRACT_ADDRESS="$pyth_addr"
WALLET_SECRET="mnemonic:$HERMES_MNEMONIC"
EOF

	log "Hermes configuration written successfully"
}

main() {
	log "=== Akash Local Node Initialization ==="
	log "Chain ID: $AKASH_CHAIN_ID"

	# Check if already initialized and running
	if [ -f "$AKASH_HOME/config/genesis.json" ] && [ -f "$AKASH_RUN_DIR/hermes.env" ]; then
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
