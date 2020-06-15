#!/bin/sh
# shellcheck disable=SC2181
# set -x

env

findseeds() {
  while read -r entry; do
    name="${entry% *}-akash-node"
    id="${entry#* }"

    env_name="$(echo "$name" | tr '[:lower:]' '[:upper:]' | tr '-' '_' )"

    port="$(printenv "${env_name}_SERVICE_PORT_AKASHD_P2P")"
    if [ $? -eq 0 ]; then
      echo "${id}@${name}:${port}"
    fi
  done < /config/peers.txt
}

makeseeds() {
  findseeds | paste -sd ',' -
}

seeds=$(makeseeds)

echo "found seeds: $seeds"

export AKASHD_P2P_SEEDS="$seeds"

mkdir -p "$AKASHD_DATA/config"
mkdir -p "$AKASHD_DATA/data"

cp /config/genesis.json        "$AKASHD_DATA/config"
cp /config/priv_validator_key.json "$AKASHD_DATA/config"
cp /config/priv_validator_state.json "$AKASHD_DATA/data"
cp /config/node_key.json       "$AKASHD_DATA/config"

/akashd start
