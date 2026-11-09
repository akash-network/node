#!/usr/bin/env bash
# Check the actual installed command without RPC calls or private-key output.
set -euo pipefail
source "$(dirname "$0")/../scripts/workspace.sh"
package=${1:?package directory required} role=${2:?role required}
[[ "$role" =~ ^(relayer|reporter)$ ]]
if find "$package/contracts/solana/node_modules/bigint-buffer" -name '*.node' -print -quit | grep -q .; then
  echo 'Native bigint-buffer binding must not be shipped.' >&2; exit 1
fi
fixture=$(mktemp -d "$SOLANA_CACHE/operator-smoke.XXXXXX")
trap 'rm -rf "$fixture"' EXIT
command=$package/bin/akt-$role
"$command" --help > /dev/null
"$command" --version | grep -Fx "service akt-$role"
address=$("$command" keys create --out "$fixture/key")
[[ -n "$address" && "$address" == "$("$command" keys address --key-file "$fixture/key")" ]]
if "$command" keys create --out "$fixture/key" > /dev/null 2>&1; then
  echo 'key overwrite unexpectedly succeeded' >&2; exit 1
fi
chmod 644 "$fixture/key"
if "$command" keys address --key-file "$fixture/key" > /dev/null 2>&1; then
  echo 'insecure key permissions unexpectedly accepted' >&2; exit 1
fi
"$package/runtime/bin/node" - "$package" "$fixture" "$role" "$address" <<'JS'
const { readFileSync, writeFileSync } = require('node:fs');
const [root, fixture, role, address] = process.argv.slice(2);
const config = JSON.parse(readFileSync(`${root}/deploy/config.example.json`, 'utf8'));
const program = 'GTr2HRp7Bavsc6rzSXq1a6W1p1H9B2v8gTf2UNSiiBvF';
config.programId = program;
if (role === 'reporter') {
  config.reporter = address;
  config.mint = program;
}
writeFileSync(`${fixture}/mainnet.json`, JSON.stringify(config));
writeFileSync(`${fixture}/testnet.json`, JSON.stringify({ ...config,
  akashChainId: 'akash-bridge-v2-release-test-1', solanaGenesisHash: program,
}));
JS
# Neither check loads the deliberately unreadable signing key nor contacts RPC.
"$command" check --config "$fixture/mainnet.json"
"$command" check --network testnet --config "$fixture/testnet.json"
if "$command" check --network testnet --config "$fixture/mainnet.json" > /dev/null 2>&1; then
  echo 'mainnet config accepted as testnet' >&2; exit 1
fi
if "$command" check --config "$fixture/testnet.json" > /dev/null 2>&1; then
  echo 'testnet config accepted as mainnet' >&2; exit 1
fi
echo 'Installed command and key custody checks passed.'
