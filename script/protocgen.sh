#!/usr/bin/env bash

set -eo pipefail

proto_dirs=$(find ./proto -path -prune -o -name '*.proto' -print0 | xargs -0 -n1 dirname | sort | uniq)
#shellcheck disable=SC2046
for dir in $proto_dirs; do
  .cache/bin/protoc \
  -I "proto" \
  -I "vendor/github.com/cosmos/cosmos-sdk/proto" \
  -I "vendor/github.com/cosmos/cosmos-sdk/third_party/proto" \
  --gocosmos_out=plugins=interfacetype+grpc,\
Mgoogle/protobuf/any.proto=github.com/cosmos/cosmos-sdk/codec/types:. \
  $(find "${dir}" -maxdepth 1 -name '*.proto')
done

# move proto files to the right places
cp -r github.com/ovrclk/akash/* ./
rm -rf github.com
