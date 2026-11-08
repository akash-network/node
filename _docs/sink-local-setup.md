# Set up the local Sink test

Prepare the chains for [CLI mint and refund tests](sink-local-e2e.md).
Run the steps in order, from the node root, in one Bash session.

## Contents

- [Prerequisites](#prerequisites)
- [Run the Akash upgrade](#run-the-akash-upgrade)
- [Build the images](#build-the-images)
- [Start the bridge](#start-the-bridge)
- [Fund reporters](#fund-reporters)
- [Enable Sink](#enable-sink)

## Prerequisites

- Linux x86_64, Docker Compose 2.24.4 or newer, Bash and jq.
- Matching Linux node binary, programs, IDLs and header encoder from
  [Solana setup](../contracts/solana/docs/setup.md), plus the locked
  [SDK archive](../contracts/solana/docs/sdk-integration.md).
- The Metaplex fixture: run `npm run setup:metadata` from `contracts/solana`.
- The tracked upgrade fixture. The harness always reads `tests/upgrade/config.json`
  and, unless `STATE_CONFIG` names another absolute path, `tests/upgrade/testnet.json`.
  The native runner imports the configured keys; it does not generate them.
- Free native-test ports: 26656, 26657, 9090, 9091, 9092, 6060 and 1317.

Allow roughly 8 CPUs, 16–32 GiB RAM and 60 GiB disk for builds and small ledgers.
Budget native preparation separately: it downloads the sandbox snapshot and
builds multi-platform binaries. Apple Silicon emulation is not qualified.

## Run the Akash upgrade

Create an isolated workspace:

```bash
export SINK_SOURCE="$PWD"
mkdir -p .cache
export SINK_RUN=$(mktemp -d "$PWD/.cache/sink-compose-XXXXXX")
export COMPOSE_PROJECT_NAME=$(basename "$SINK_RUN" | tr '[:upper:]' '[:lower:]')
export SINK_UID=$(id -u) SINK_GID=$(id -g)
export SINK_SOLANA_CACHE="$PWD/.cache/solana"
mkdir -p "$SINK_RUN/node_modules" "$SINK_SOURCE/contracts/solana/node_modules"
```

Use the tracked test keys and the single-validator `tests/upgrade/test-config.json`.
Do not copy a deployed validator home or reuse an old numeric-ID Sink ledger.
Run the [existing upgrade tests](../contracts/solana/docs/reproduce.md#5-existing-chain-upgrade)
from `tests/upgrade` so the runner finds its configuration:

```bash
(cd tests/upgrade && make test AP_RUN_DIR="$SINK_RUN/upgrade" \
  SNAPSHOT_SOURCE=sandbox UPGRADE_TO=v2.2.0 GENESIS_BINARY_VERSION=v2.1.2 \
  UPGRADE_BINARY_VERSION=local)
export AKASH_HOME="$SINK_RUN/upgrade/validators/.akash0"
```

Require a passing upgrade from v2.1.2 to registered v2.2.0. Retain the result
and binary hash. Stop all native Cosmovisor and validator processes before
Compose opens the same home. The isolated directory does not change native
ports or permit concurrent access to the database and validator key.

## Build the images

The standard build requires a 72-hour refund challenge. For a shorter local
test, build a separate [sinkdev binary](../contracts/solana/docs/setup.md#accelerated-testnet).
Use `.cache/solana/bin/akash-sinkdev` as the `cp` source below and set
`export SINK_CHALLENGE=20s`. Run only the linked build command, not `testnet:up`.
Keep its image and hash distinct. Restore a release-valid challenge through
governance before restarting that state with a standard binary.

Build the Akash image from the upgraded Linux binary:

```bash
export AKASH_NODE_IMAGE="$COMPOSE_PROJECT_NAME-node:local"
mkdir "$SINK_RUN/node-image"
cp "$AKASH_HOME/cosmovisor/current/bin/akash" "$SINK_RUN/node-image/akash"
docker build -f _build/akash.Dockerfile -t "$AKASH_NODE_IMAGE" "$SINK_RUN/node-image"
rm -rf "$SINK_RUN/node-image"
export SINK_SERVICES_IMAGE="$COMPOSE_PROJECT_NAME-services:local"
docker build --platform linux/amd64 -t "$COMPOSE_PROJECT_NAME-toolchain:local" \
  contracts/solana/tools/verifiable-build
docker build --platform linux/amd64 \
  --build-arg SINK_TOOLCHAIN_IMAGE="$COMPOSE_PROJECT_NAME-toolchain:local" \
  -t "$SINK_SERVICES_IMAGE" tests/upgrade/sink
dc() {
  docker compose --env-file /dev/null -p "$COMPOSE_PROJECT_NAME" \
    -f "$SINK_SOURCE/tests/upgrade/docker-compose.yaml" \
    -f "$SINK_SOURCE/tests/upgrade/docker-compose.sink.yaml" "$@"
}
tools() { dc run --rm --no-deps sink-tools "$@"; }
```

## Start the bridge

```bash
dc config --quiet
dc up -d --wait validator
tools prepare
dc up -d --wait solana
tools bootstrap
dc up -d relayer
dc ps
```

`prepare` installs locked dependencies and creates test keys. `bootstrap`
initializes Solana from a real Akash checkpoint without changing Akash state.
Only Akash RPC is published: `dc port validator 26657` shows its loopback port.
Use `tools` for Solana CLI commands inside the containers.

Bootstrap runs once. If interrupted, `bootstrap-started` blocks a blind rerun.
Resume the recorded operation using the [program guide](../contracts/solana/docs/deploy-program.md),
or preserve the failed run and start a fresh pair. Do not reset only one ledger.

## Fund reporters

Use the fixture's funded key. Change `SINK_KEY` if its name is not `validator0`:

```bash
export SINK_KEY=validator0
export SINK_CHAIN=$(jq -r .chainId "$SINK_RUN/network.json")
a() { dc exec -T validator akash "$@" --home /root/.akash; }
tx() {
  a tx "$@" --from "$SINK_KEY" --keyring-backend test \
    --chain-id "$SINK_CHAIN" --gas auto --gas-adjustment 2 \
    --gas-prices 0.025uakt --yes -o json
}
a query sink params -o json
a query sink reporters 0 -o json
for reporter in $(jq -r '.[]' "$SINK_RUN/reporters.json"); do
  tx bank send "$SINK_KEY" "$reporter" 10000000uakt > "$SINK_RUN/funding.json"
  jq -e '.code == 0' "$SINK_RUN/funding.json" >/dev/null || break
  hash=$(jq -r .txhash "$SINK_RUN/funding.json")
  for attempt in {1..60}; do
    a query tx "$hash" -o json > "$SINK_RUN/funding-committed.json" 2>/dev/null && break
    sleep 1
  done
  jq -e '.code == 0' "$SINK_RUN/funding-committed.json" >/dev/null || break
done
```

Stop on any funding error or timeout. Confirm all seven transfers committed.

## Enable Sink

Merge the test limits into current params, preserving other fields and native
epochs. Seven reporters with threshold `0.67` require five matching votes.

```bash
a query sink params -o json | jq --slurpfile reporters "$SINK_RUN/reporters.json" \
  --arg challenge "${SINK_CHALLENGE:-259200s}" '
  .params | .enabled = true | .refunds_enabled = true |
  .reporters = $reporters[0] | .reporters_threshold = "0.67" |
  .reporter_challenge_period = $challenge |
  .min_amount_per_transfer = {denom:"uakt", amount:"1"} |
  .max_amount_per_transfer = {denom:"uakt", amount:"1000000000"} |
  .max_epoch_amount_per_account = {denom:"uakt", amount:"1000000000"} |
  .max_transfer_amount_per_epoch = {denom:"uakt", amount:"10000000000"} |
  .max_epoch_refund = {denom:"uakt", amount:"1000000000"}' > "$SINK_RUN/params.json"
dc cp "$SINK_RUN/params.json" validator:/root/.akash/sink-params.json
a tx sink update-params /root/.akash/sink-params.json --generate-only \
  --from "$SINK_KEY" --keyring-backend test --chain-id "$SINK_CHAIN" \
  --gas 200000 -o json > "$SINK_RUN/params-tx.json"
a query gov params -o json
```

Keep fixed gas for the unsigned inner message; auto-gas tries to simulate the
governance module as a signer. Submitted proposals use normal automatic gas.

Set `GOV_DEPOSIT` from the governance query, as a coin string. Review the params,
then submit the proposal:

```bash
jq --arg deposit "$GOV_DEPOSIT" '{messages:.body.messages, metadata:"",
  deposit:$deposit, title:"Enable local Sink", summary:"Local seven-reporter test",
  expedited:false}' "$SINK_RUN/params-tx.json" > "$SINK_RUN/proposal.json"
dc cp "$SINK_RUN/proposal.json" validator:/root/.akash/sink-proposal.json
tx gov submit-proposal /root/.akash/sink-proposal.json
```

Wait for submission to commit. Set `PROPOSAL_ID` from that transaction, then vote:

```bash
tx gov vote "$PROPOSAL_ID" yes
a query gov proposal "$PROPOSAL_ID" -o json
a query sink params -o json
a query sink reporters 0 -o json
```

Wait for passage and confirm the intended params and reporter policy before starting reporters:

```bash
dc up -d reporter0 reporter1 reporter2 reporter3 reporter4 reporter5 reporter6
```

Continue with [the CLI tests](sink-local-e2e.md#test-a-mint) in this shell.
These reporters share one Solana validator; use the [operator guide](https://github.com/akash-network/sink-operators/blob/v0.1.0/docs/install.md)
for separate service hosts and independent observation infrastructure.
