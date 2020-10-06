#!/bin/sh

set -e

env | sort

mkdir -p "$AKASH_HOME/data"
mkdir -p "$AKASH_HOME/config"

# XXX it's not reading all of the env variables.

cp "$AKASH_BOOT_KEYS/priv_validator_state.json"   "$AKASH_HOME/data/"
cp "$AKASH_BOOT_DATA/genesis.json" "$AKASH_HOME/config/"
cp "$AKASH_BOOT_KEYS/node_key.json" "$AKASH_HOME/config/"
cp "$AKASH_BOOT_KEYS/priv_validator_key.json" "$AKASH_HOME/config/"

/akash start --home=$AKASH_HOME
