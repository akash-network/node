#!/bin/sh

set -e

env | sort

mkdir -p "$AKASHD_HOME/data"
mkdir -p "$AKASHD_HOME/config"

# XXX it's not reading all of the env variables.

cp "$AKASHD_BOOT_KEYS/priv_validator_state.json"   "$AKASHD_HOME/data/"
cp "$AKASHD_BOOT_DATA/genesis.json" "$AKASHD_HOME/config/"
cp "$AKASHD_BOOT_KEYS/node_key.json" "$AKASHD_HOME/config/"
cp "$AKASHD_BOOT_KEYS/priv_validator_key.json" "$AKASHD_HOME/config/"

/akashd start
