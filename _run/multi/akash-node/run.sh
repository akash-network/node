# set -x

env

findseeds() {
  cat /config/peers.txt | while read entry; do
    name="${entry% *}-akash-node"
    id="${entry#* }"

    env_name="$(echo "$name" | tr '[[:lower:]]' '[[:upper:]]' | tr '-' '_' )"

    port="$(printenv "${env_name}_SERVICE_PORT_AKASHD_P2P")"
    if [ $? -eq 0 ]; then
      echo "${id}@${name}:${port}"
    fi
  done
}

makeseeds() {
  findseeds | paste -sd ',' -
}

seeds=$(makeseeds)

echo "found seeds: $seeds"

export AKASHD_P2P_SEEDS="$seeds"

mkdir -p "$AKASHD_DATA/config"

cp /config/genesis.json        "$AKASHD_DATA/config"
cp /config/priv_validator.json "$AKASHD_DATA/config"
cp /config/node_key.json       "$AKASHD_DATA/config"

/akashd start
