#!/usr/bin/env bash

# AKASH_DB_DIR

apt-get update
apt-get install jq curl -y

META=./meta.json

meta_url=https://raw.githubusercontent.com/ovrclk/net/master/mainnet/meta.json

curl -sSfl "$meta_url" > "$META"

export AKASH_HOME="${AKASH_HOME:-home}"
export AKASH_MONIKER="${AKASH_MONIKER:-"todo"}"
export AKASH_CHAIN_ID="${AKASH_CHAIN_ID:-"$(jq -Mr '.chain_id' "$META")"}"
export AKASH_P2P_SEEDS="${AKASH_P2P_SEEDS:-"$(jq -Mr '.peers.seeds[] | .id + "@" + .address' "$META" | paste -sd, -)"}"
export AKASH_P2P_PERSISTENT_PEERS="${AKASH_P2P_PERSISTENT_PEERS:-"$(jq -Mr '.peers.persistent_peers[] | .id + "@" + .address' "$META" | paste -sd, -)"}"

echo "$AKASH_HOME"
echo "$AKASH_CHAIN_ID"
echo "$AKASH_P2P_SEEDS"
echo "$AKASH_P2P_PERSISTENT_PEERS"

# initialize
if [ ! -d "$AKASH_HOME" ]; then
  ./bin/akash init "$AKASH_MONIKER" --home "$AKASH_HOME"
  genesis_url="$(jq -Mr '.genesis.genesis_url' "$META")"
  curl -sSfl "$genesis_url" > "$AKASH_HOME/config/genesis.json"
fi

exec /bin/akash start --p2p.laddr "tcp://0.0.0.0:26656"   \
                      --rpc.laddr "tcp://0.0.0.0:26657"
