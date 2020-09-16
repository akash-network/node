#!/usr/bin/env bash

set -eo pipefail

proto_dirs=$(find ./proto -path -prune -o -name '*.proto' -print0 | xargs -0 -n1 dirname | sort | uniq)
for dir in $proto_dirs; do

  # generate swagger files (filter query files)
  query_file=$(find "${dir}" -maxdepth 1 -name 'query.proto')
  if [[ -n "$query_file" ]]; then
    .cache/bin/protoc  \
    -I "proto" \
    -I "third_party/proto" \
    -I "vendor/github.com/regen-network/cosmos-proto" \
    -I "vendor/github.com/tendermint/tendermint/proto" \
    -I "vendor/github.com/cosmos/cosmos-sdk/proto" \
    -I "vendor/github.com/gogo/protobuf" \
    -I ".cache/include" \
    "$query_file" \
    --swagger_out=logtostderr=true,stderrthreshold=1000,fqn_for_swagger_name=true,simple_operation_ids=true:.
  fi
done

# combine swagger files
# uses nodejs package `swagger-combine`.
# all the individual swagger files need to be configured in `config.json` for merging
swagger-combine ./client/docs/config.json \
-o ./client/docs/swagger.json \
--continueOnConflictingPaths true \
--includeDefinitions true

# clean swagger files
find ./ -name 'query.swagger.json' -exec rm {} \;
