#!/usr/bin/env bash
# Source this from scripts; paths are independent of the caller's directory.
solana_repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
# This directory is contracts/solana inside the node repository.
NODE_SINK_DIR=${NODE_SINK_DIR:-$(cd "$solana_repo/../.." && pwd)}
CHAIN_SDK_DIR=${CHAIN_SDK_DIR:-$(cd "$NODE_SINK_DIR/.." && pwd)/chain-sdk}
export NODE_SINK_DIR CHAIN_SDK_DIR
