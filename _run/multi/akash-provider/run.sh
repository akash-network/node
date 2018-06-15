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

if [ ! -f "$masterKey" -o ! -f "$providerKey" ]; then
  ./akash key create master > "$masterKey"
  echo "created account: " $(cat "$masterKey")

  ./akash provider create /config/provider.yml -k master > "$providerKey"
  echo "created provider: " $(cat "$providerKey")
fi

echo "running provider $(cat "$providerKey")..."

./akash provider run "$(cat "$providerKey")" -k master --kube
