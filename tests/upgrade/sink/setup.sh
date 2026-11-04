#!/usr/bin/env bash
set -euo pipefail
umask 077

: "${SINK_STATE:?Set SINK_STATE to the isolated Compose state directory}"
support=/workspace/tests/upgrade/sink
cache=/workspace/.cache/solana
rpc=http://127.0.0.1:8899

program_id() {
  node -e 'console.log(require(process.argv[1]).address)' "$1"
}

case "${1:-}" in
  prepare)
    npm ci
    test ! -e "$SINK_STATE/payer.json"
    solana-keygen new --no-bip39-passphrase --silent --outfile "$SINK_STATE/payer.json" >/dev/null
    solana-keygen new --no-bip39-passphrase --silent --outfile "$SINK_STATE/recipient.json" >/dev/null
    node "$support/identities.cjs" keys
    printf 'Prepared seven local reporter accounts. Public addresses are in reporters.json.\n'
    ;;
  solana)
    payer=$(solana-keygen pubkey "$SINK_STATE/payer.json")
    export AKT_LOCALNET_RPC_PORT=8899
    export AKT_LOCALNET_LEDGER="$SINK_STATE/ledger"
    exec bash scripts/localnet.sh --limit-ledger-size 1000000 \
      --upgradeable-program "$(program_id "$cache/target/idl/akt_solana.json")" \
        "$cache/target/deploy/akt_solana.so" "$payer" \
      --upgradeable-program "$(program_id "$PWD/idls/ics07_tendermint.json")" \
        "$cache/deps/ibc-contracts/ibc-solana/target/deploy/ics07_tendermint.so" "$payer" \
      --upgradeable-program "$(program_id "$PWD/idls/access_manager.json")" \
        "$cache/deps/ibc-contracts/ibc-solana/target/deploy/access_manager.so" "$payer" \
      --quiet
    ;;
  bootstrap)
    test ! -e "$SINK_STATE/bootstrap-started"
    node -e 'require("node:fs").writeFileSync(process.argv[1], "started\n", {flag:"wx", mode:0o600})' \
      "$SINK_STATE/bootstrap-started"
    akt=$(program_id "$cache/target/idl/akt_solana.json")
    client=$(program_id "$PWD/idls/ics07_tendermint.json")
    manager=$(program_id "$PWD/idls/access_manager.json")
    payer=$(solana-keygen pubkey "$SINK_STATE/payer.json")
    genesis=$(solana --url "$rpc" genesis-hash)
    admin() {
      node --import tsx local/bridge-admin.ts "$@" --url "$rpc" --genesis "$genesis" \
        --keypair "$SINK_STATE/payer.json"
    }
    height=$(node -e 'fetch("http://127.0.0.1:26657/status").then(r=>r.json()).then(s=>console.log((BigInt(s.result.sync_info.latest_block_height)-1n).toString()))')
    admin checkpoint --height "$height" --akash-rpc http://127.0.0.1:26657 --out "$SINK_STATE/checkpoint.json"
    chain=$(node -e 'console.log(require(process.argv[1]).chainId)' "$SINK_STATE/checkpoint.json")
    test "$chain" != akashnet-2
    solana --url "$rpc" --keypair "$SINK_STATE/payer.json" --commitment finalized airdrop 100 >/dev/null
    admin init-access-manager --access-manager "$manager" --admin "$payer" --relayer "$payer"
    admin initialize --akt-program "$akt"
    admin init-client --program "$client" --access-manager "$manager" \
      --checkpoint "$SINK_STATE/checkpoint.json" --trusting-period 86400 \
      --unbonding-period 1814400 --max-clock-drift 10
    admin configure-bridge --akt-program "$akt" --program "$client" --chain-id "$chain" \
      --per-transfer-max 1000000000000 --daily-mint-cap 10000000000000 \
      --checkpoint "$SINK_STATE/checkpoint.json"
    admin set-bridge-enabled --akt-program "$akt" --enabled true
    node "$support/identities.cjs" configs "$akt"
    admin show --akt-program "$akt" --access-manager "$manager"
    ;;
  reporter)
    [[ "${2:-}" =~ ^[0-6]$ ]] || { echo 'Reporter index must be 0 through 6.' >&2; exit 1; }
    exec node --import tsx reporter/run.ts --config "$SINK_STATE/reporter$2.json" \
      --key-file "$SINK_STATE/reporter$2.key" --watch --broadcast
    ;;
  *)
    exec "$@"
    ;;
esac
