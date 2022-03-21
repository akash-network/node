#!/usr/bin/env bash

set -euo pipefail

###
#
#  Script to bootstrap a node within docker.
#  Example use:
#
#  exec docker run --detach --rm --name "akash-1" \
#    -e AKASH_MONIKER=akash-1 \
#    -e "AKASH_HOME=$AKASH_HOME" \
#    -p "$LOCAL_IP:26657:26657" \
#    -p 0.0.0.0:26656:26656 \
#    -v "$STORAGE:$AKASH_HOME/data" \
#    "$IMAGE" \
#    /bootstrap.sh
#
###


apt-get update
apt-get install jq curl -y

DEFAULT_META_URL=https://raw.githubusercontent.com/ovrclk/net/master/mainnet/meta.json
META=./meta.json

meta_url="${1:-$DEFAULT_META_URL}"

curl -sSfl "$meta_url" > "$META"

export AKASH_HOME="${AKASH_HOME:-/akash}"
export AKASH_MONIKER="${AKASH_MONIKER:-"akash-node"}"
export AKASH_CHAIN_ID="${AKASH_CHAIN_ID:-"$(jq -Mr '.chain_id' "$META")"}"
export AKASH_P2P_SEEDS="${AKASH_P2P_SEEDS:-"$(jq -Mr '.peers.seeds[] | .id + "@" + .address' "$META" | paste -sd, -)"}"
export AKASH_P2P_PERSISTENT_PEERS="${AKASH_P2P_PERSISTENT_PEERS:-"$(jq -Mr '.peers.persistent_peers[] | .id + "@" + .address' "$META" | paste -sd, -)"}"

echo "$AKASH_HOME"
echo "$AKASH_MONIKER"
echo "$AKASH_CHAIN_ID"
echo "$AKASH_P2P_SEEDS"
echo "$AKASH_P2P_PERSISTENT_PEERS"

# initialize
if [ ! -d "$AKASH_HOME/config" ]; then
  ./bin/akash init "$AKASH_MONIKER" --home "$AKASH_HOME"
  genesis_url="$(jq -Mr '.genesis.genesis_url' "$META")"
  curl -sSfl "$genesis_url" > "$AKASH_HOME/config/genesis.json"
fi

exec /bin/akash start --p2p.laddr "tcp://0.0.0.0:26656" \
                      --rpc.laddr "tcp://0.0.0.0:26657" \
                      --home "$AKASH_HOME"
