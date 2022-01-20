#!/bin/bash
# microtick and bitcanna contributed significantly here.
set -uxe

# set environment variables
export GOPATH=~/go
export PATH=$PATH:~/go/bin


# Install Akash
go install -tags rocksdb ./...

# MAKE HOME FOLDER AND GET GENESIS
akash init test --home /sstwo/akash 
wget -O /sstwo/akash/config/genesis.json https://github.com/ovrclk/net/raw/master/mainnet/genesis.json

INTERVAL=1000

# GET TRUST HASH AND TRUST HEIGHT

LATEST_HEIGHT=$(curl -s https://akashnet-2.technofractal.com/block | jq -r .result.block.header.height);
BLOCK_HEIGHT=$(($LATEST_HEIGHT-$INTERVAL)) 
TRUST_HASH=$(curl -s "https://akashnet-2.technofractal.com/block?height=$BLOCK_HEIGHT" | jq -r .result.block_id.hash)


# TELL USER WHAT WE ARE DOING
echo "TRUST HEIGHT: $BLOCK_HEIGHT"
echo "TRUST HASH: $TRUST_HASH"


# export state sync vars
export AKASH_STATESYNC_ENABLE=true
export AKASH_P2P_MAX_NUM_OUTBOUND_PEERS=200
export AKASH_STATESYNC_RPC_SERVERS="https://rpc.akash.forbole.com:443,http://akash-sentry01.skynetvalidators.com:26657,https://rpc.akash.smartnodes.one:443,http://akash.c29r3.xyz:80/rpc"
export AKASH_STATESYNC_TRUST_HEIGHT=$BLOCK_HEIGHT
export AKASH_STATESYNC_TRUST_HASH=$TRUST_HASH
export AKASH_P2P_SEEDS="27eb432ccd5e895c5c659659120d68b393dd8c60@35.247.65.183:26656,8e2f56098f182ffe2f6fb09280bafe13c63eb42f@46.101.176.149:26656,fff99a2e8f3c9473e4e5ee9a99611a2e599529fd@46.166.138.218:26656"

akash start --db_backend rocksdb --home /sstwo/akash
