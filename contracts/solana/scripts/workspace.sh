#!/usr/bin/env bash
# Source this from scripts; paths are independent of the caller's directory.
solana_repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
# This directory is contracts/solana inside the node repository.
NODE_SINK_DIR=${NODE_SINK_DIR:-$(cd "$solana_repo/../.." && pwd)}
SOLANA_CACHE=$solana_repo/../../.cache/solana
mkdir -p "$SOLANA_CACHE"
SOLANA_CACHE=$(cd "$SOLANA_CACHE" && pwd)
SOLANA_DEPS=$SOLANA_CACHE/deps
SOLANA_RUN=$SOLANA_CACHE/run
CARGO_TARGET_DIR=$SOLANA_CACHE/target
export NODE_SINK_DIR SOLANA_CACHE SOLANA_DEPS SOLANA_RUN CARGO_TARGET_DIR
