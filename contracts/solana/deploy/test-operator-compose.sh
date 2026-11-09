#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../scripts/workspace.sh"
role=${1:?role required} image=${2:?built image required}
[[ "$role" =~ ^(relayer|reporter)$ ]]
fixture=$(mktemp -d "$SOLANA_CACHE/compose-smoke.XXXXXX")
project=operator-check-$(basename "$fixture" | tr '[:upper:].' '[:lower:]-')
export OPERATOR_IMAGE=$image OPERATOR_NETWORK=testnet
OPERATOR_UID=$(id -u)
OPERATOR_GID=$(id -g)
export OPERATOR_UID OPERATOR_GID
export OPERATOR_CONFIG=$fixture/config.json OPERATOR_KEY=$fixture/key
export OPERATOR_STATE=$fixture/state
compose=(docker compose -p "$project" -f "$NODE_SINK_DIR/_run/akt-$role/docker-compose.yaml")
trap '"${compose[@]}" down --remove-orphans >/dev/null 2>&1; rm -rf "$fixture"' EXIT
mkdir "$OPERATOR_STATE"
address=$(docker run --rm --platform linux/amd64 --network none --user "$OPERATOR_UID:$OPERATOR_GID" \
  --mount "type=bind,src=$fixture,dst=/keys" "$image" keys create --out /keys/key)
python3 - "$NODE_SINK_DIR/_run/akt-$role/config.example.json" "$OPERATOR_CONFIG" "$role" "$address" <<'PY'
import json, sys
source, destination, role, address = sys.argv[1:]
with open(source) as file:
    config = json.load(file)
program = 'GTr2HRp7Bavsc6rzSXq1a6W1p1H9B2v8gTf2UNSiiBvF'
config.update(akashChainId='akash-bridge-v2-compose-test-1', solanaGenesisHash=program, programId=program)
if role == 'reporter':
    config.update(reporter=address, mint=program)
with open(destination, 'w') as file:
    json.dump(config, file)
PY
"${compose[@]}" config --format json > "$fixture/compose.json"
python3 - "$fixture/compose.json" "$role" <<'PY'
import json, sys
with open(sys.argv[1]) as file:
    service = json.load(file)['services'][sys.argv[2]]
assert service['read_only']
assert not service.get('ports')
assert '--broadcast' not in service['command']
assert all(mount['read_only'] for mount in service['volumes'] if mount['target'] != '/data')
PY
"${compose[@]}" run --rm -T "$role" check --network testnet --config /etc/akt/config.json
test "$address" = "$("${compose[@]}" run --rm -T "$role" keys address --key-file /run/keys/key)"
if [[ "$role" == relayer ]]; then
  "${compose[@]}" run --rm -T --entrypoint /opt/akt/runtime/bin/node relayer \
    -e 'require("fs").writeFileSync("/data/checkpoint", "retained")'
  "${compose[@]}" run --rm -T --entrypoint /opt/akt/runtime/bin/node relayer \
    -e 'require("assert").strictEqual(require("fs").readFileSync("/data/checkpoint", "utf8"), "retained")'
else
  "${compose[@]}" -f "$NODE_SINK_DIR/_run/akt-reporter/broadcast.yaml" config --format json > "$fixture/broadcast.json"
  python3 - "$fixture/broadcast.json" <<'PY'
import json, sys
with open(sys.argv[1]) as file:
    assert '--broadcast' in json.load(file)['services']['reporter']['command']
PY
fi
mv "$OPERATOR_KEY" "$fixture/saved-key"
if "${compose[@]}" run --rm -T "$role" --version > "$fixture/missing-key.log" 2>&1; then
  echo 'missing signing-key bind unexpectedly created' >&2; exit 1
fi
test ! -e "$OPERATOR_KEY"
echo 'Compose config, mounts, key ownership and persistence checks passed.'
