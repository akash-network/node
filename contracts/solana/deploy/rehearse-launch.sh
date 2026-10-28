#!/usr/bin/env bash
# Rehearses the launch in docs/deploy-program.md on a temporary local validator.
# A ceremony key deploys and initializes the programs, and a vault keypair
# stands in for the multisig: every multisig step is emitted, decoded and then
# executed with that keypair through upstream's test CPI proxy, so it arrives
# by CPI as from a multisig program. The access manager lists the proxy as its
# CPI caller. It ends with a re-pin, a program upgrade from a buffer and a
# guardian pause. Never point this script at a shared cluster.
#
# Usage: bash deploy/rehearse-launch.sh
# By default the programs get fresh IDs by relocating the local builds
# (tests/relocate-program.ts); a real launch rebuilds with new declare_id!
# values instead. For a dress rehearsal of the launch artifacts, set
#   AKT_REHEARSAL_PROGRAMS  an `npm run build:verifiable` output directory
#   AKT_REHEARSAL_KEYPAIRS  the directory with NAME-keypair.json program keys
# and run it from the launch commit, built with `npm run setup`.
# AKT_LOCALNET_RPC_PORT (default 17899) selects the validator's ports.
set -euo pipefail
cd "$(dirname "$0")/.."

port=${AKT_LOCALNET_RPC_PORT:-17899}
url=http://127.0.0.1:$port
artifacts=.deps/ibc-contracts/ibc-solana/target/deploy
names=(akt_solana ics07_tendermint access_manager)
mkdir -p .cache
run=$(mktemp -d "$PWD/.cache/launch-rehearsal.XXXXXX")
mkdir "$run/keys" "$run/tx" "$run/programs"
echo "Rehearsal evidence: $run"

newkey() {
  solana-keygen new --no-bip39-passphrase --silent --outfile "$run/keys/$1.json" > /dev/null
  solana-keygen pubkey "$run/keys/$1.json"
}
sha256() {
  if command -v sha256sum > /dev/null; then sha256sum "$1"; else shasum -a 256 "$1"; fi | cut -d' ' -f1
}
# The program ID each build declares, from the IDLs it was built with.
declared() {
  local idl=idls/$1.json
  [[ $1 == akt_solana ]] && idl=target/idl/akt_solana.json
  python3 -I -c 'import json, sys; print(json.load(open(sys.argv[1]))["address"])' "$idl"
}
# Rewrites a build's own program ID (tests/relocate-program.ts).
relocate() {
  node_modules/.bin/tsx -e '
    const { readFileSync, writeFileSync } = require("node:fs");
    const { web3 } = require("@coral-xyz/anchor");
    const { relocate } = require("./tests/relocate-program");
    const [input, from, to, output] = process.argv.slice(-4);
    writeFileSync(output, relocate(readFileSync(input),
      new web3.PublicKey(from), new web3.PublicKey(to)));
  ' "$@"
}

ceremony=$(newkey ceremony)
vault=$(newkey vault)
relayer=$(newkey relayer)
relayer2=$(newkey relayer2)
guardian=$(newkey guardian)
# Stand-in for the multisig program: upstream's generic CPI proxy, as in
# scripts/test-contract.sh.
multisig_program=CtQLLKbDMt1XVNXtLKJEt1K8cstbckjqE6zyFqR37KTc
if [[ ! -f "$artifacts/test_cpi_proxy.so" ]]; then
  (cd .deps/ibc-contracts/ibc-solana && RUSTUP_TOOLCHAIN=1.89.0 anchor build --program-name test_cpi_proxy)
fi
repin_id=$(newkey ics07_repin-keypair)

if [[ -n "${AKT_REHEARSAL_PROGRAMS:-}" ]]; then
  [[ -n "${AKT_REHEARSAL_KEYPAIRS:-}" ]] || { echo 'Set AKT_REHEARSAL_KEYPAIRS too.' >&2; exit 1; }
  (cd "$AKT_REHEARSAL_PROGRAMS" && if command -v sha256sum > /dev/null; then
    sha256sum -c SHA256SUMS; else shasum -a 256 -c SHA256SUMS; fi)
  for name in "${names[@]}"; do
    cp "$AKT_REHEARSAL_PROGRAMS/$name.so" "$run/programs/"
    cp "$AKT_REHEARSAL_KEYPAIRS/$name-keypair.json" "$run/keys/"
    id=$(solana-keygen pubkey "$run/keys/$name-keypair.json")
    [[ "$id" == "$(declared "$name")" ]] ||
      { echo "$name keypair is $id; the build declares $(declared "$name")" >&2; exit 1; }
  done
else
  for name in "${names[@]}"; do
    source=$artifacts/$name.so
    [[ $name == akt_solana ]] && source=target/deploy/akt_solana.so
    test -f "$source" || { echo "Missing $source. Run npm run setup first." >&2; exit 1; }
    relocate "$source" "$(declared "$name")" "$(newkey "$name-keypair")" "$run/programs/$name.so"
  done
fi
akt_id=$(solana-keygen pubkey "$run/keys/akt_solana-keypair.json")
client_id=$(solana-keygen pubkey "$run/keys/ics07_tendermint-keypair.json")
manager_id=$(solana-keygen pubkey "$run/keys/access_manager-keypair.json")
relocate "$run/programs/ics07_tendermint.so" "$client_id" "$repin_id" "$run/programs/ics07_repin.so"
# A second proxy the access manager does not list must be refused.
unlisted_proxy=$(newkey unlisted-proxy)
relocate "$artifacts/test_cpi_proxy.so" "$multisig_program" "$unlisted_proxy" "$run/programs/unlisted_proxy.so"

AKT_LOCALNET_LEDGER="$run/ledger" bash scripts/localnet.sh --mint "$ceremony" --quiet \
  --bpf-program "$multisig_program" "$artifacts/test_cpi_proxy.so" \
  --bpf-program "$unlisted_proxy" "$run/programs/unlisted_proxy.so" > "$run/validator.log" 2>&1 &
validator=$!
cleanup() {
  kill "$validator" 2> /dev/null || true
  wait "$validator" 2> /dev/null || true
}
trap cleanup EXIT
for _ in $(seq 1 120); do
  kill -0 "$validator" 2> /dev/null || { echo "Validator exited; see $run/validator.log" >&2; exit 1; }
  solana --url "$url" cluster-version > /dev/null 2>&1 && break
  sleep 1
done
genesis=$(solana --url "$url" genesis-hash)
echo "Genesis $genesis"

sol() { solana --url "$url" --keypair "$run/keys/ceremony.json" --commitment finalized "$@"; }
admin() { node_modules/.bin/tsx local/bridge-admin.ts "$@" --genesis "$genesis" --url "$url"; }
verify() { bash deploy/verify-program.sh --url "$url" "$@"; }
# Writes one multisig step to tx/NAME.json.
emit() {
  local name=$1
  shift
  admin "$@" --emit "$run/tx/$name.json" --authority "$vault" --akt-program "$akt_id"
}
# Decodes a step as a reviewer would, then executes it by CPI with the vault
# key.
execute() {
  admin decode "$run/tx/$1.json" > "$run/tx/$1.review.txt"
  admin execute "$run/tx/$1.json" --keypair "$run/keys/vault.json" \
    --cpi-proxy "$multisig_program" | tee "$run/tx/$1.executed.json"
}
multisig() {
  emit "$@"
  execute "$1"
}
checkpoint() {
  python3 -I - "$1" "$2" > "$run/$3" << 'PY'
import datetime, json, secrets, sys
time = datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(seconds=5)
micros = int(time.timestamp()) * 1_000_000 + time.microsecond
print(json.dumps({
    "chainId": sys.argv[1],
    "height": sys.argv[2],
    "time": time.strftime("%Y-%m-%dT%H:%M:%S.%fZ"),
    "timestamp": str(micros * 1000),
    "root": secrets.token_hex(32),
    "nextValidatorsHash": secrets.token_hex(32),
    "validatorsHash": secrets.token_hex(32),
    "blockHash": secrets.token_hex(32),
}, indent=2))
PY
}

for key in "$vault" "$guardian"; do
  sol transfer --allow-unfunded-recipient "$key" 10 > /dev/null
done

echo '== 4. Deploy with the ceremony key'
for name in "${names[@]}"; do
  id=$(solana-keygen pubkey "$run/keys/$name-keypair.json")
  sol program deploy "$run/programs/$name.so" --program-id "$run/keys/$name-keypair.json" \
    --upgrade-authority "$run/keys/ceremony.json" \
    --max-len "$((2 * $(wc -c < "$run/programs/$name.so")))"
  verify "$id" "$run/programs/$name.so" "$ceremony"
done

echo '== 5. Access manager handover and genesis'
admin init-access-manager --keypair "$run/keys/ceremony.json" --access-manager "$manager_id" \
  --admin "$vault" --cpi-caller "$multisig_program" --relayer "$relayer" | tee "$run/access-manager.json"
admin initialize --keypair "$run/keys/ceremony.json" --akt-program "$akt_id" |
  tee "$run/initialize.json"

echo '== 6. Upgrade authorities to the vault'
for name in "${names[@]}"; do
  id=$(solana-keygen pubkey "$run/keys/$name-keypair.json")
  sol program set-upgrade-authority "$id" --upgrade-authority "$run/keys/ceremony.json" \
    --new-upgrade-authority "$vault" --skip-new-upgrade-authority-signer-check
  verify "$id" "$run/programs/$name.so" "$vault"
done

echo '== 7. Multisig: metadata and guardian in one transaction'
emit metadata set-token-metadata --uri https://example.invalid/token/akt-v1.json
emit guardian set-guardian --guardian "$guardian"
admin bundle --emit "$run/tx/launch-metadata.json" "$run/tx/metadata.json" "$run/tx/guardian.json"
execute launch-metadata

echo '== 8. Multisig: checkpoint client and bridge in one transaction'
checkpoint akash-rehearsal-2 1000 checkpoint.json
emit client init-client --program "$client_id" --access-manager "$manager_id" \
  --checkpoint "$run/checkpoint.json" --trusting-period 86400 --unbonding-period 1814400 \
  --max-clock-drift 10
# Launch policy: 1,000,000 AKT per transfer, 10,000,000 AKT per UTC day.
emit bridge configure-bridge --program "$client_id" --chain-id akash-rehearsal-2 \
  --per-transfer-max 1000000000000 --daily-mint-cap 10000000000000 --checkpoint "$run/checkpoint.json"
admin bundle --emit "$run/tx/launch-bridge.json" "$run/tx/client.json" "$run/tx/bridge.json"
execute launch-bridge
emit grant grant-relayer --key "$relayer2"
if admin execute "$run/tx/grant.json" --keypair "$run/keys/vault.json" \
  --cpi-proxy "$unlisted_proxy" > "$run/tx/grant.unlisted.txt" 2>&1; then
  echo 'The access manager accepted an unlisted CPI caller' >&2
  exit 1
fi
grep -q CpiNotAllowed "$run/tx/grant.unlisted.txt"
execute grant
multisig open open-relaying
admin relayers --akt-program "$akt_id" | tee "$run/relayers-open.json"
multisig close close-relaying
multisig revoke revoke-relayer --key "$relayer2"

echo '== 9. Multisig: policy and activation'
# Rehearses the policy instruction; it restates the launch policy.
multisig policy set-mint-policy --per-transfer-max 1000000000000 --daily-mint-cap 10000000000000
multisig enable set-bridge-enabled --enabled true

echo '== Guardian pause, signed by the guardian key'
admin pause --keypair "$run/keys/guardian.json" --akt-program "$akt_id" | tee "$run/pause.json"

echo '== Re-pin to a new verifier deployment'
sol program deploy "$run/programs/ics07_repin.so" --program-id "$run/keys/ics07_repin-keypair.json" \
  --upgrade-authority "$run/keys/ceremony.json"
sol program set-upgrade-authority "$repin_id" --upgrade-authority "$run/keys/ceremony.json" \
  --new-upgrade-authority "$vault" --skip-new-upgrade-authority-signer-check
checkpoint akash-rehearsal-2 2000 checkpoint-repin.json
emit repin-client init-client --program "$repin_id" --access-manager "$manager_id" \
  --checkpoint "$run/checkpoint-repin.json" --trusting-period 86400 --unbonding-period 1814400 \
  --max-clock-drift 10
emit repin repin --program "$repin_id" --checkpoint "$run/checkpoint-repin.json"
admin bundle --emit "$run/tx/repin-bridge.json" "$run/tx/repin-client.json" "$run/tx/repin.json"
execute repin-bridge

echo '== Multisig program upgrade from a buffer'
buffer=$(sol program write-buffer "$run/programs/akt_solana.so" --output json |
  python3 -I -c 'import json, sys; print(json.load(sys.stdin)["buffer"])')
sol program set-buffer-authority "$buffer" --new-buffer-authority "$vault"
multisig upgrade upgrade-program --program "$akt_id" --buffer "$buffer" --spill "$vault" \
  --expect-sha256 "$(sha256 "$run/programs/akt_solana.so")"
verify "$akt_id" "$run/programs/akt_solana.so" "$vault"

echo '== Events, read by invocation frame from the CPI executions'
signature=$(python3 -I -c 'import json, sys; print(json.load(open(sys.argv[1]))["signature"])' "$run/tx/repin-bridge.executed.json")
admin events --signature "$signature" --akt-program "$akt_id" | tee "$run/repin-events.json"
python3 -I - "$run" << 'PY'
import json, sys
run = sys.argv[1]
expected = {
    "tx/launch-metadata.executed.json": {"TokenMetadataSet", "PauseGuardianSet"},
    "tx/launch-bridge.executed.json": {"BridgeConfigured"},
    "tx/policy.executed.json": {"MintPolicySet"},
    "tx/enable.executed.json": {"BridgeEnabledSet"},
    "tx/repin-bridge.executed.json": {"LightClientRepinned"},
    "repin-events.json": {"LightClientRepinned"},
    "pause.json": {"GuardianPaused"},
}
for path, names in expected.items():
    found = {event["name"] for event in json.load(open(f"{run}/{path}"))["events"]}
    assert found == names, (path, found)
PY

echo '== Final state'
admin show --akt-program "$akt_id" --access-manager "$manager_id" | tee "$run/show.json"
python3 -I - "$run/show.json" "$vault" "$repin_id" "$relayer" "$guardian" << 'PY'
import json, sys
show = json.load(open(sys.argv[1]))
vault, repin, relayer, guardian = sys.argv[2:]
token = show["token"]
assert token["decimals"] == 6 and token["supply"] == "0", token
assert token["freezeAuthority"] is None and token["mintAuthorityIsPda"], token
assert "treasury" not in token, token
assert show["bridgeEnabled"] is False, "the guardian pause did not hold"
assert show["guardian"] == guardian
assert show["lightClientProgram"] == repin and show["client"]["status"] == "active"
assert show["policy"]["perTransferMax"] == "1000000000000"
assert show["policy"]["dailyMintCap"] == "10000000000000"
assert show["metadata"]["uri"] == "https://example.invalid/token/akt-v1.json"
relaying = show["relaying"]
assert relaying["relayers"] == [relayer] and not relaying["openRelaying"], relaying
assert relaying["admins"] == [vault], relaying
for name, info in show["upgradeAuthorities"].items():
    assert info["upgradeAuthority"] == vault, (name, info)
print("Launch rehearsal passed.")
PY
