#!/bin/sh

set -e

##
# Configuration sanity check
##

# shellcheck disable=SC2015
[ -f "$AKASH_BOOT_KEYS/key.txt" ] && [ -f "$AKASH_BOOT_KEYS/key-pass.txt" ] || {
  echo "Key information not found; AKASH_BOOT_KEYS is not configured properly"
  exit 1
}

env | sort

##
# Import key
##
/bin/akash --home="$AKASH_HOME" keys import --keyring-backend="$AKASH_KEYRING_BACKEND"  "$AKASH_FROM" \
  "$AKASH_BOOT_KEYS/key.txt" < "$AKASH_BOOT_KEYS/key-pass.txt"

##
# Run daemon
##
#/akash --home=$AKASH_HOME provider run --cluster-k8s
/bin/akash --home="$AKASH_HOME" --node="$AKASH_NODE" --chain-id="$AKASH_CHAIN_ID" --keyring-backend="$AKASH_KEYRING_BACKEND" --from="$AKASH_FROM" provider run --cluster-k8s
