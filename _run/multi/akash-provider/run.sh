#!/bin/sh

set -e
set -o pipefail

#set -x

rm -rf provider.key

./akash provider create unused.yml -k dc | tail -1 > provider.key

echo "created datacenter: " $(cat provider.key)

./akash provider run "$(cat provider.key)" -k dc
