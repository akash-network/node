#!/bin/sh

set -xe

##
# Configuration sanity check
##

# shellcheck disable=SC2015
[ -f "$AKASH_BOOT_KEYS/key.txt" ] && [ -f "$AKASH_BOOT_KEYS/key-pass.txt" ] || {
  echo "Key information not found; AKASH_BOOT_KEYS is not configured properly"
  exit 1
}



set -e

env | sort

##
# Import key
##
/akash --home=$AKASH_HOME keys import --keyring-backend=$AKASH_KEYRING_BACKEND  "$AKASH_FROM" \
  "$AKASH_BOOT_KEYS/key.txt" < "$AKASH_BOOT_KEYS/key-pass.txt"

##
# Run daemon
##
#/akash --home=$AKASH_HOME provider run --cluster-k8s
/akash --home=$AKASH_HOME --node tcp://$AKASH_SERVICE_HOST:$AKASH_SERVICE_PORT --keyring-backend=$AKASH_KEYRING_BACKEND provider run --chain-id=$AKASH_CHAIN_ID --from "$AKASH_FROM"  --cluster-k8s
