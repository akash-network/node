#!/usr/bin/env bash
# Ship the same pinned Node runtime on every operator host.
set -euo pipefail
source "$(dirname "$0")/../scripts/workspace.sh"
[[ $# == 1 && "$1" == /* && ! -e "$1" ]] || { echo 'usage: node-runtime.sh /new/absolute/directory' >&2; exit 2; }
[[ "$(uname -s)" == Linux ]] || { echo 'Linux runtime required' >&2; exit 1; }
case "$(uname -m)" in
  x86_64) arch=x64; checksum=6c172a9ab6718f7b801fb5a7bd486af2ee5e32758086a04b84fcce2aa77095e6 ;;
  aarch64) arch=arm64; checksum=dbabf7a1cbf947e6059ffad2c27b80041db97dea1e553b62afdee7f13fca336e ;;
  *) echo 'Supported architectures: x86_64, aarch64' >&2; exit 1 ;;
esac
stage=$(mktemp -d "$SOLANA_CACHE/node-runtime.XXXXXX")
trap 'rm -rf "$stage"' EXIT
curl --fail --location --retry 3 --connect-timeout 15 --max-time 180 \
  "https://nodejs.org/dist/v24.22.1/node-v24.22.1-linux-$arch.tar.xz" -o "$stage/node.tar.xz"
echo "$checksum  $stage/node.tar.xz" | sha256sum -c -
tar -xJf "$stage/node.tar.xz" -C "$stage" --strip-components=1 \
  "node-v24.22.1-linux-$arch/bin/node" "node-v24.22.1-linux-$arch/LICENSE"
install -d -m 755 "$1/bin"
install -m 755 "$stage/bin/node" "$1/bin/node"
install -m 644 "$stage/LICENSE" "$1/LICENSE"
[[ "$("$1/bin/node" --version)" == v24.22.1 ]]
