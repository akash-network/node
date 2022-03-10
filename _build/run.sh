#!/usr/bin/env bash

lvcreate -s --name akash-1 storage/akash-0
lvchange -ay -K storage/akash-1
mount /dev/storage/akash-1 /mnt/storage/akash-1

# lvcreate -V500G -T storage/node-tp -n akash-1
# mkfs.ext4 /dev/storage/akash-1
# mount /dev/storage/akash-1 /mnt/storage/akash-1

IMAGE=ghcr.io/ovrclk/akash:0.14.2-rc0
STORAGE=/mnt/storage/akash-1
AKASH_HOME=/akash

LOCAL_IP=10.88.134.47

exec docker run --detach --rm --name "akash-1" \
  -e AKASH_MONIKER=akash-1 \
  -e "AKASH_HOME=$AKASH_HOME" \
  -p "$LOCAL_IP:26657:26657" \
  -p 0.0.0.0:26656:26656 \
  -v "$STORAGE:$AKASH_HOME/data" \
  --mount type=bind,src=/root/bootstrap.sh,target=/bootstrap.sh \
  "$IMAGE" \
  /bootstrap.sh

