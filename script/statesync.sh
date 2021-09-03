#!/bin/bash
# microtick and bitcanna contributed significantly here.


INTERVAL=1000

# GET TRUST HASH AND TRUST HEIGHT

LATEST_HEIGHT=$(curl -s 162.55.132.230:2021/block | jq -r .result.block.header.height);
BLOCK_HEIGHT=$(($ATEST_HEIGHT-INTERVAL)) 
TRUST_HASH=$(curl -s "162.55.132.230:2021/block?height=$BLOCK_HEIGHT" | jq -r .result.block_id.hash)


# TELL USER WHAT WE ARE DOING
echo "TRUST HEIGHT: $BLOCK_HEIGHT"
echo "TRUST HASH: $TRUST_HASH"


# export state sync vars
export AKASH_STATESYNC_ENABLE=true
export AKASH_STATESYNC_RPC_SERVERS="162.55.132.230:2021,95.217.196.54:2021"
export AKASH_STATESYNC_TRUST_HEIGHT=$BLOCK_HEIGHT
export AKASH_STATESYNC_TRUST_HASH=$TRUST_HASH
export AKASH_P2P_PERSISTENT_PEERS="1b44ef48a2d185332ccccdb493b72eb6be4c9c56@162.55.132.230:2020,8be34f94aa567906f13fd681af904bb0aea56299@95.217.196.54:2020"

akash unsafe-reset-all
akash start
