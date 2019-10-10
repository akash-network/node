#!/bin/sh

#set -x
# set -e
# set -o pipefail

getnodes() {
  env | grep "AKASH_NODE_SERVICE_HOST=" | while read entry; do
    prefix="${entry%_HOST=*}"
    host="${entry#*=}"
    port="$(printenv "${prefix}_PORT_AKASHD_RPC")"
    if [ $? -eq 0 ]; then
      echo "http://${host}:${port}"
    fi
  done
}

node="$(getnodes | head -1)"

echo "found node: $node"

export AKASH_NODE="$node"

mkdir -p "$AKASH_DATA"

masterKey="$AKASH_DATA/master.key"
providerKey="$AKASH_DATA/provider.key"

if [ ! -s "$masterKey" ] || [ ! -s "$providerKey" ]; then
	eval $(./akash key create master -m shell)
	echo $akash_create_key_0_public_key > "$masterKey"
  echo "created master key: " $(cat "$masterKey")

  echo "adding provider"
  eval $(./akash provider create /config/provider.yml -k master -m shell)
  echo $akash_add_provider_0_data > "$providerKey"
  echo "added provider: " $(cat "$providerKey")
fi

echo "running provider $(cat "$providerKey")..."

./akash provider run "$(cat "$providerKey")" -k master --kube --manifest-ns "$1"
