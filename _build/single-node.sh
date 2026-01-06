#!/bin/sh

CHAINID=$1
GENACCT=$2

if [ -z "$1" ]; then
  echo "Need to input chain id..."
  exit 1
fi

if [ -z "$2" ]; then
  echo "Need to input genesis account address..."
  exit 1
fi

# Clean up previous data
rm -rf ~/.akash

# Build genesis file incl account for passed address
coins="100000000000uakt"
akash genesis init "$CHAINID" --chain-id "$CHAINID"
akash keys add validator --keyring-backend="test"
akash genesis add-account "$(akash keys show validator -a --keyring-backend="test")" $coins
akash genesis add-account "$GENACCT" $coins
akash genesis gentx validator 10000000000uakt --keyring-backend="test" --chain-id "$CHAINID" --min-self-delegation="1"
akash genesis collect

# Set proper defaults and change ports
sed -i.bak 's#"tcp://127.0.0.1:26657"#"tcp://0.0.0.0:26657"#g' ~/.akash/config/config.toml
sed -i.bak 's/timeout_commit = "5s"/timeout_commit = "1s"/g' ~/.akash/config/config.toml
sed -i.bak 's/timeout_propose = "3s"/timeout_propose = "1s"/g' ~/.akash/config/config.toml
sed -i.bak 's/index_all_keys = false/index_all_keys = true/g' ~/.akash/config/config.toml
rm -f ~/.akash/config/config.toml.bak

# Start the akash
akash start --pruning=nothing