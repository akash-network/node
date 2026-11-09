#!/usr/bin/env bash
# Rehearse a reviewed release with setup first and a final Squads v4 handover.
# The real program keypairs establish the same IDs on an isolated local ledger;
# all authority/member keys and SOL used here are disposable local fixtures.
# Usage: bash deploy/rehearse-ceremony.sh /absolute/manifest.json /absolute/program-keys
set -euo pipefail
umask 077
source "$(dirname "$0")/../scripts/workspace.sh"
cd "$solana_repo"
[[ $# == 2 && $1 == /* && $2 == /* ]] || {
  echo 'Pass an absolute release manifest and program-key directory.' >&2
  exit 2
}
release=$1 program_keys=$2
node --import tsx deploy/programs.ts check --manifest "$release"
build=$(node -e 'const p=require("node:path"),m=require(process.argv[1]);process.stdout.write(p.resolve(p.dirname(process.argv[1]),m.buildDirectory))' "$release")
port=${AKT_LOCALNET_RPC_PORT:-17899}
url=http://127.0.0.1:$port
mkdir -p "$SOLANA_RUN"
run=$(mktemp -d "$SOLANA_RUN/ceremony-rehearsal.XXXXXX")
mkdir "$run/keys" "$run/tx"
echo "Rehearsal evidence: $run"
validator=
cleanup() {
  if [[ -n "$validator" ]]; then
    kill "$validator" 2>/dev/null || true
    wait "$validator" 2>/dev/null || true
  fi
  rm -rf "$run/keys" "$run/ledger"
  rm -f "$run/private-deploy.log"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
newkey() {
  solana-keygen new --config /dev/null --no-bip39-passphrase --silent --outfile "$run/keys/$1.json" >/dev/null
  solana-keygen pubkey "$run/keys/$1.json"
}
ceremony=$(newkey ceremony)
relayer=$(newkey relayer)
squads=SQDS4ep65T869zMMBKyuUq6aD6EgTu8psMjkvj52pCf
config=$(node --import tsx deploy/rehearse-squads.ts fixture --run "$run")
solana --config /dev/null --url https://api.mainnet-beta.solana.com \
  program dump "$squads" "$run/squads.so" >/dev/null
shasum -a 256 "$run/squads.so" > "$run/squads-binary.sha256"
bash scripts/setup-metadata.sh
AKT_LOCALNET_LEDGER="$run/ledger" bash scripts/localnet.sh --mint "$ceremony" --quiet --ticks-per-slot 8 \
  --bpf-program "$squads" "$run/squads.so" \
  --account "$config" "$run/squads-config.json" > "$run/validator.log" 2>&1 &
validator=$!
ready=0
for _ in $(seq 1 120); do
  kill -0 "$validator" 2>/dev/null || { echo "Local validator exited; see $run/validator.log" >&2; exit 1; }
  # Preloaded ProgramData at slot zero becomes executable in a later bank.
  # A listening RPC alone does not make it visible to finalized simulation.
  slot=$(solana --config /dev/null --url "$url" --commitment finalized slot 2>/dev/null || true)
  if [[ $slot =~ ^[0-9]+$ ]] && ((slot > 0)); then ready=1; break; fi
  sleep 1
done
[[ $ready == 1 ]] || { echo 'Local validator did not become ready.' >&2; exit 1; }
genesis=$(solana --config /dev/null --url "$url" genesis-hash)
[[ $genesis != 5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d ]] || exit 1
vault=$(node --import tsx deploy/rehearse-squads.ts create --run "$run" --url "$url")
node - "$release" "$run/manifest.json" "$ceremony" "$vault" "$build" <<'JS'
const fs=require('node:fs');
const [input,output,ceremony,vault,build]=process.argv.slice(2);
const manifest=JSON.parse(fs.readFileSync(input,'utf8'));
manifest.ceremonyAuthority=ceremony;
manifest.upgradeAuthority=vault;
manifest.buildDirectory=build;
fs.writeFileSync(output,JSON.stringify(manifest,null,2)+'\n',{flag:'wx'});
JS
manifest=$run/manifest.json
program_id() { node -e 'process.stdout.write(require(process.argv[1]).programs[process.argv[2]])' "$manifest" "$1"; }
akt_id=$(program_id akt)
client_id=$(program_id ics07_tendermint)
manager_id=$(program_id access_manager)
sol() { solana --config /dev/null --url "$url" --keypair "$run/keys/ceremony.json" --commitment finalized "$@"; }
admin() { node --import tsx local/bridge-admin.ts "$@" --genesis "$genesis" --url "$url" --akt-program "$akt_id"; }
emit() {
  local name=$1
  shift
  admin "$@" --emit "$run/tx/$name.json" --authority "$emit_authority"
}
ceremony_execute() {
  admin decode "$run/tx/$1.json" > "$run/tx/$1.review.txt"
  admin ceremony-execute "$run/tx/$1.json" --manifest "$manifest" --rehearsal \
    --keypair "$run/keys/ceremony.json" > "$run/tx/$1.executed.json"
}
vault_execute() {
  admin decode "$run/tx/$1.json" > "$run/tx/$1.review.txt"
  node --import tsx deploy/rehearse-squads.ts execute --run "$run" --url "$url" \
    --bundle "$run/tx/$1.json" > "$run/tx/$1.executed.json"
}

echo 'Deploying the exact release programs with binary-sized allocation.'
for name in access_manager ics07_tendermint akt; do
  id=$(program_id "$name")
  [[ "$(solana-keygen pubkey "$program_keys/$name-keypair.json")" == "$id" ]] || {
    echo "Wrong program key for $name" >&2; exit 1;
  }
  sol program deploy "$build/$name.so" --program-id "$program_keys/$name-keypair.json" \
    --upgrade-authority "$run/keys/ceremony.json" \
    --max-len "$(( $(wc -c < "$build/$name.so") ))" \
    --with-compute-unit-price 0 > "$run/private-deploy.log" 2>&1
  SOLANA_KEYPAIR="$run/keys/ceremony.json" bash deploy/verify-program.sh \
    --url "$url" "$id" "$build/$name.so" "$ceremony"
done
admin init-access-manager --keypair "$run/keys/ceremony.json" --access-manager "$manager_id" \
  --admin "$ceremony" --cpi-caller "$squads" --relayer "$client_id" --relayer "$relayer" > "$run/access-manager.json"
admin initialize --keypair "$run/keys/ceremony.json" > "$run/initialize.json"
emit_authority=$ceremony
emit metadata set-token-metadata --uri https://raw.githubusercontent.com/akash-network/net/3174df54bb9c4b5c5523c985e43e1405d8cfbb4b/solana/mainnet/akt.json
emit guardian set-guardian --guardian "$vault"
admin bundle --emit "$run/tx/metadata-guardian.json" "$run/tx/metadata.json" "$run/tx/guardian.json"
ceremony_execute metadata-guardian

# Synthetic checkpoint and deliberately small test limits, never launch inputs.
python3 -I - > "$run/checkpoint.json" <<'PY'
import datetime,json,secrets
time=datetime.datetime.now(datetime.timezone.utc)-datetime.timedelta(seconds=5)
micros=int(time.timestamp())*1_000_000+time.microsecond
print(json.dumps({"chainId":"akash-rehearsal-2","height":"1000",
 "time":time.strftime("%Y-%m-%dT%H:%M:%S.%fZ"),"timestamp":str(micros*1000),
 "root":secrets.token_hex(32),"nextValidatorsHash":secrets.token_hex(32),
 "validatorsHash":secrets.token_hex(32),"blockHash":secrets.token_hex(32)}))
PY
emit client init-client --program "$client_id" --access-manager "$manager_id" \
  --checkpoint "$run/checkpoint.json" --trusting-period 86400 --unbonding-period 1814400 --max-clock-drift 10
emit bridge configure-bridge --program "$client_id" --chain-id akash-rehearsal-2 \
  --per-transfer-max 100000000 --daily-mint-cap 1000000000 --checkpoint "$run/checkpoint.json"
admin bundle --emit "$run/tx/client-bridge.json" "$run/tx/client.json" "$run/tx/bridge.json"
ceremony_execute client-bridge
admin show --access-manager "$manager_id" > "$run/before-handover.json"
emit retired set-guardian --guardian "$ceremony"
emit handover ceremony-handover --manifest "$manifest" --rehearsal
ceremony_execute handover
admin show --access-manager "$manager_id" > "$run/after-handover.json"
node --import tsx deploy/rehearse-squads.ts retired --run "$run" --url "$url" \
  --bundle "$run/tx/retired.json" > "$run/retired-onchain.txt"
if admin set-guardian --guardian "$ceremony" --keypair "$run/keys/ceremony.json" > "$run/retired-authority.txt" 2>&1; then
  echo 'Retired ceremony key was accepted for administration.' >&2; exit 1
fi
grep -q 'upgrade authority' "$run/retired-authority.txt"

echo 'Checking actual Squads proposal, two approvals, execution, and guardian pause.'
emit_authority=$vault
emit enable set-bridge-enabled --enabled true
vault_execute enable
emit pause pause
vault_execute pause
admin show --access-manager "$manager_id" > "$run/final-state.json"
python3 -I - "$run" "$ceremony" "$vault" "$client_id" <<'PY'
import json,sys
from pathlib import Path
root=Path(sys.argv[1]); ceremony,vault,client=sys.argv[2:]
before=json.loads((root/'before-handover.json').read_text())
after=json.loads((root/'after-handover.json').read_text())
final=json.loads((root/'final-state.json').read_text())
for field in ['mint','chainId','policy','guardian','lightClientProgram','metadata','token','bridgeEnabled']:
 assert before[field]==after[field], (field,before[field],after[field])
assert before['accessManager']['admins']==[ceremony]
assert after['accessManager']['admins']==[vault]
for state in [before,after,final]:
 assert state['token']['supply']=='0' and state['token']['decimals']==6
 assert state['token']['mintAuthorityIsPda'] and state['token']['freezeAuthority'] is None
 assert state['relaying']['openRelaying'] and client in state['relaying']['relayers']
 assert state['guardian']==vault and state['policy']['vaults']==[]
assert final['bridgeEnabled'] is False
for state in [after,final]:
 for info in state['upgradeAuthorities'].values(): assert info['upgradeAuthority']==vault
print('Late-handover rehearsal passed with the Squads vault as pause guardian.')
PY
