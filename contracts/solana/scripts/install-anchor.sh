#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/workspace.sh"
# Match the official release used by AVM, including its IDL builder.
# cargo install --locked 0.32.1 uses an older IDL dependency and fails with
# procmacro2_semver_exempt when compiling solana-packet's serde derives.
case "$(uname -s)-$(uname -m)" in
  Linux-x86_64) target=x86_64-unknown-linux-gnu; digest=5f25b850ce80278507a98947833fcd48423391f6d145046ffb0c5fd130dec436 ;;
  Darwin-arm64) target=aarch64-apple-darwin; digest=10c63cc7af8aaa3e0dde9c4bfad17c6a47797e7685d5f25236b14d6d5642a02a ;;
  Darwin-x86_64) target=x86_64-apple-darwin; digest=beec62b617ab8afce6776a9234b85ab203959cee25d179e8aab8cec48b468234 ;;
  *) echo 'No pinned Anchor release for this platform.' >&2; exit 1 ;;
esac
stage=$(mktemp -d "$SOLANA_CACHE/anchor.XXXXXX")
candidate=
trap 'rm -rf "$stage"; if [[ -n "$candidate" ]]; then rm -f "$candidate"; fi' EXIT
download=$stage/download
curl -fL --retry 3 "https://github.com/otter-sec/anchor/releases/download/v0.32.1/anchor-0.32.1-$target" -o "$download"
python3 - "$download" "$digest" <<'PY'
import hashlib, pathlib, sys
actual = hashlib.sha256(pathlib.Path(sys.argv[1]).read_bytes()).hexdigest()
if actual != sys.argv[2]:
    raise SystemExit('Anchor release checksum mismatch')
PY
mkdir -p "$HOME/.cargo/bin"
candidate=$(mktemp "$HOME/.cargo/bin/.anchor.XXXXXX")
install -m 755 "$download" "$candidate"
cmp "$download" "$candidate"
"$candidate" --version
# Replaces an AVM symlink itself if present, never the shared AVM executable.
mv -f "$candidate" "$HOME/.cargo/bin/anchor"
