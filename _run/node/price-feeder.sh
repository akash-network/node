#!/usr/bin/env bash

################################################################################
# Akash Price Feeder Service
#
# Continuously fetches AKT/USD price from Pyth Network and submits to
# Akash testnet oracle module via transaction.
################################################################################

set -euo pipefail

# Configuration
AKASH_CHAIN_ID="${AKASH_CHAIN_ID:=testnet-8}"
AKASH_NODE="${AKASH_NODE:=https://testnetrpc.akashnet.net:443}"
AKASH_KEYRING_BACKEND="${AKASH_KEYRING_BACKEND:=test}"
AKASH_FROM="${AKASH_FROM:=price-feeder}"
UPDATE_INTERVAL=10  # seconds between updates

# Pyth configuration
AKT_PYTH_FEED_ID="4ea5bb4d2f5900cc2e97ba534240950740b4d3b89fe712a94a7304fd2fd92702"
PYTH_API="https://hermes.pyth.network/api/latest_price_feeds"

# Logging
LOG_FILE="$AKASH_RUN_DIR/price-feeder.log"
MAX_LOG_SIZE=10485760  # 10MB

################################################################################
# Functions
################################################################################

log() {
	local level="$1"
	shift
	# shellcheck disable=SC2124
	local message="$@"
	local timestamp

	timestamp=$(date -u '+%Y-%m-%d %H:%M:%S UTC')

	echo "[$timestamp] [$level] $message" | tee -a "$LOG_FILE"

	# Rotate log if too large
	if [ -f "$LOG_FILE" ] && [ "$(stat -c%s "$LOG_FILE" 2>/dev/null || stat -f%z "$LOG_FILE" 2>/dev/null)" -gt $MAX_LOG_SIZE ]; then
		mv "$LOG_FILE" "${LOG_FILE}.old"
		log "INFO" "Log rotated"
	fi
}

check_dependencies() {
	local missing=()

	for cmd in akash curl jq bc; do
		if ! command -v "$cmd" &> /dev/null; then
			missing+=("$cmd")
		fi
	done

	if [ ${#missing[@]} -ne 0 ]; then
		log "ERROR" "Missing dependencies: ${missing[*]}"
		exit 1
	fi
}

check_key_exists() {
	if ! akash keys show "$AKASH_FROM" &> /dev/null; then
		log "ERROR" "Key '$AKASH_FROM' not found in keyring"
		exit 1
	fi

	local address
	address=$(akash keys show "$AKASH_FROM" -a)
	log "INFO" "Using feeder address: $address"
}

check_balance() {
	local address
	local balance

	address=$(akash keys show "$AKASH_FROM" -a )
	balance=$(akash query bank balances "$address" -o json 2>/dev/null | jq -r '.balances[] | select(.denom=="uakt") | .amount // "0"')

	if [ -z "$balance" ] || [ "$balance" -lt 100000 ]; then
		log "WARN" "Low balance: ${balance:-0}uakt (recommend >100000uakt for gas)"
	else
		log "INFO" "Balance: ${balance}uakt"
	fi
}

fetch_pyth_price() {
	local url="${PYTH_API}?ids[]=${AKT_PYTH_FEED_ID}"
	local response

	response=$(curl -s --max-time 10 "$url" 2>/dev/null)

	if [ -z "$response" ]; then
		log "ERROR" "Empty response from Pyth API"
		return 1
	fi

	if ! echo "$response" | jq -e '.[0].price' &> /dev/null; then
		log "ERROR" "Invalid response from Pyth API: $response"
		return 1
	fi

	local price_raw expo
	price_raw=$(echo "$response" | jq -r '.[0].price.price')
	expo=$(echo "$response" | jq -r '.[0].price.expo')

	if [ -z "$price_raw" ] || [ -z "$expo" ]; then
		log "ERROR" "Failed to extract price data"
		return 1
	fi

	# Calculate price: price_raw * 10^expo
	local price
	price=$(echo "scale=10; $price_raw * (10 ^ $expo)" | bc | sed 's/^\./0./' | sed 's/0*$//' | sed 's/\.$//')

	echo "$price"
}

get_block_time() {
	local block_time
	block_time=$(curl -s --max-time 10 "${AKASH_NODE}/status" | jq -r '.result.sync_info.latest_block_time')

	if [ -z "$block_time" ] || [ "$block_time" == "null" ]; then
		log "ERROR" "Failed to fetch block time"
		return 1
	fi

	echo "$block_time"
}

submit_price_to_oracle() {
	local price="$1"
	local timestamp="$2"

	log "INFO" "Submitting price to oracle: \$${price} USD at ${timestamp}"

	# Submit transaction with price and timestamp
	local tx_result
	tx_result=$(akash tx oracle feed akt usd "$price" "$timestamp" \
		--gas auto \
		--gas-adjustment 1.5 \
		--gas-prices 0.025uakt \
		--yes \
		-o json 2>&1)

	local exit_code=$?

	if [ $exit_code -ne 0 ]; then
		log "ERROR" "Transaction failed: $tx_result"
		return 1
	fi

	# Check for error in response
	local code
	code=$(echo "$tx_result" | jq -r '.code // 0')
	if [ "$code" != "0" ]; then
		local raw_log
		raw_log=$(echo "$tx_result" | jq -r '.raw_log // "unknown error"')
		log "ERROR" "Transaction failed with code $code: $raw_log"
		return 1
	fi

	# Extract tx hash
	local tx_hash
	tx_hash=$(echo "$tx_result" | jq -r '.txhash // empty')

	if [ -n "$tx_hash" ]; then
		log "INFO" "Transaction submitted: $tx_hash"
	else
		log "WARN" "Transaction submitted but no hash returned"
	fi

	return 0
}

handle_shutdown() {
	log "INFO" "Received shutdown signal, exiting gracefully..."
	exit 0
}

################################################################################
# Main Loop
################################################################################

main() {
	log "INFO" "Starting Akash Price Feeder Service"
	log "INFO" "Chain: $AKASH_CHAIN_ID"
	log "INFO" "Node: $AKASH_NODE"
	log "INFO" "Update interval: ${UPDATE_INTERVAL}s"

	# Startup checks
	check_dependencies
	check_key_exists
	check_balance

	# Trap signals for graceful shutdown
	trap handle_shutdown SIGTERM SIGINT

	# Main loop
	local iteration=0
	local consecutive_failures=0
	local max_consecutive_failures=5

	while true; do
		iteration=$((iteration + 1))
		log "INFO" "=== Iteration $iteration ==="

		# Fetch price from Pyth
		local price
		if price=$(fetch_pyth_price); then
			log "INFO" "Fetched AKT price: \$${price} USD"

			# Get current block time for timestamp
			local block_time
			if block_time=$(get_block_time); then
				log "INFO" "Block time: $block_time"

				# Submit to oracle
				if submit_price_to_oracle "$price" "$block_time"; then
					consecutive_failures=0
					log "INFO" "Price update successful"
				else
					consecutive_failures=$((consecutive_failures + 1))
					log "ERROR" "Failed to submit price (failure $consecutive_failures/$max_consecutive_failures)"
				fi
			else
				consecutive_failures=$((consecutive_failures + 1))
				log "ERROR" "Failed to get block time (failure $consecutive_failures/$max_consecutive_failures)"
			fi
		else
			consecutive_failures=$((consecutive_failures + 1))
			log "ERROR" "Failed to fetch price from Pyth (failure $consecutive_failures/$max_consecutive_failures)"
		fi

		# Exit if too many consecutive failures
		if [ $consecutive_failures -ge $max_consecutive_failures ]; then
			log "ERROR" "Too many consecutive failures ($consecutive_failures), exiting"
			exit 1
		fi

		# Wait before next iteration
		log "INFO" "Waiting ${UPDATE_INTERVAL}s until next update..."
		sleep "$UPDATE_INTERVAL"
	done
}

# Run main function
main "$@"
