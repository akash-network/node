# set -x
env

echo "found seeds: $AKASHD_P2P_SEEDS"
mkdir -p "$AKASHD_DATA/config"
cp /config/genesis.json        "$AKASHD_DATA/config"
cp /config/priv_validator_key.json "$AKASHD_DATA/config"
cp /config/node_key.json       "$AKASHD_DATA/config"
cp /config/config.toml       "$AKASHD_DATA/config"
cp /config/app.toml       "$AKASHD_DATA/config"

mkdir -p "$AKASHD_DATA/data"

cp /config/priv_validator_state.json "$AKASHD_DATA/data"

rm -rf "$AKASHD_DATA/config/addrbook.json"

/akashd start --home $AKASHD_DATA
