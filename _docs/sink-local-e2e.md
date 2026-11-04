# Exercise Sink locally with the upgrade-test Compose stack

Attach a Solana validator, the bridge programs, a relayer and seven reporters to
the single Akash validator prepared by the existing upgrade tests. Send tokens
with the CLI. All funds and keys are local test fixtures. No portal is required.

This procedure requires a successful native upgrade first. Compose does not run
or certify the upgrade. Keep its test result separate from the mint and refund
results below. An old numeric-ID Sink ledger is not an input to this procedure.

This is an attach procedure for a separately prepared fresh native upgrade
fixture. Supply its matching private `config.json` and public `STATE_CONFIG`
before running the commands below. The upgrade runner imports configured keys;
it does not generate this configuration pair.

Container preparation, typechecking and Solana program preload/readback have
passed. The complete local attachment, bridge bootstrap, mint, refund and
paired restart have not been verified. The separate mainnet-fork rehearsal
does not establish those local results.

## Prepare the matching builds and upgraded home

Use a Linux x86_64 host with Docker Compose 2.24.4 or newer, Bash and jq. Install
the pinned tools and build the matching node, SDK archive, programs, IDLs and
Linux header encoder using [Solana setup](../contracts/solana/docs/setup.md) and
[SDK integration](../contracts/solana/docs/sdk-integration.md). A macOS header
encoder cannot run in these Linux containers. Agave's image is AMD64; Apple
Silicon emulation is not qualified by this guide. As a planning estimate, allow
8 CPUs, 16 to 32 GiB RAM and 60 GiB free disk for builds and small ledgers.
Default native preparation downloads the full sandbox snapshot and builds the
normal multi-platform test binaries. Budget that preparation separately; the
small-ledger estimate does not cover it.

From the node root, choose an isolated directory and Compose project:

```bash
export SINK_SOURCE="$PWD"
mkdir -p .cache
export SINK_RUN=$(mktemp -d "$PWD/.cache/sink-compose-XXXXXX")
export COMPOSE_PROJECT_NAME=$(basename "$SINK_RUN" | tr '[:upper:]' '[:lower:]')
export SINK_UID=$(id -u) SINK_GID=$(id -g)
export SINK_SOLANA_CACHE="$PWD/.cache/solana"
mkdir -p "$SINK_RUN/node_modules" "$SINK_SOURCE/contracts/solana/node_modules"
```

Run the [normal upgrade flow](../contracts/solana/docs/reproduce.md#5-existing-chain-upgrade)
with `AP_RUN_DIR="$SINK_RUN/upgrade"`. For the current rehearsal select the real
`v2.1.2` old binary and registered `v2.2.0` handler. Use the matching sandbox
snapshot, `STATE_CONFIG` and private `config.json` inputs already required by
`make/test-upgrade.mk`. Use fresh local test keys in the supplied fixture.
Put `config.json` in `tests/upgrade` and use an absolute `STATE_CONFIG` path.
Do not copy a deployed host's validator home or signing material into Docker.
Keep the single-validator `tests/upgrade/test-config.json`.
Do not change the native Make targets or use an incompatible old Sink release.
Before running the native upgrade tests, free host ports 26656, 26657, 9090,
9091, 9092, 6060 and 1317. Do not run another upgrade test or local Akash node
on those ports. The isolated run directory does not change the native runner's
fixed ports.
For example, after selecting those normal inputs in the node build environment:

```bash
(cd tests/upgrade && make test AP_RUN_DIR="$SINK_RUN/upgrade" \
  SNAPSHOT_SOURCE=sandbox UPGRADE_TO=v2.2.0 GENESIS_BINARY_VERSION=v2.1.2 \
  UPGRADE_BINARY_VERSION=local STATE_CONFIG="$STATE_CONFIG")
export AKASH_HOME="$SINK_RUN/upgrade/validators/.akash0"
```

Wait for the test to finish and all its Cosmovisor/validator processes to stop.
Verify that no process has this home open before continuing. Never let the
native runner and Compose open the same database or sign with the same validator
key concurrently. Retain the passing upgrade result and exact binary hash.

Build a local Akash image from that upgraded Linux binary, using the repository's
existing image recipe. These commands do not publish an image or a release:

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

The normal build enforces a minimum 72-hour reporter challenge. To exercise a
short refund only on this disposable chain, build a separate `sinkdev` binary
with the [documented build tags](../contracts/solana/docs/setup.md#accelerated-testnet)
and use that binary in the local Akash image above. Keep its image name and hash
distinct. Set `SINK_CHALLENGE=20s` before the parameter proposal below. This is
test-only evidence. After lowering the challenge, do not restart that state with
a standard binary until governance restores a release-valid period.

## Start both chains and initialize the bridge

```bash
dc config --quiet
dc up -d --wait validator
tools prepare
dc up -d --wait solana
tools bootstrap
dc up -d relayer
dc ps
```

`prepare` installs the locked npm package and creates new keys without printing
them. `bootstrap` uses `bridge-admin.ts` to initialize the preloaded development
programs, trust one real canonical Akash checkpoint and enable the bridge. It
creates `network.json`, `checkpoint.json`, `relayer.json` and seven reporter
configs. It does not alter Akash state. The relayer authenticates subsequent
signed headers and verifies native proofs through the normal program path.

Initialization runs once. An interrupted bootstrap leaves `bootstrap-started`
and refuses a blind rerun. Inspect the named admin operation and resume through
the [program instructions](../contracts/solana/docs/deploy-program.md), or retain
the failed run and prepare a fresh paired test. Do not erase an expired
checkpoint or reset one ledger under the other chain's retained state.

All Solana traffic stays inside the validator's network namespace. Run Solana
CLI commands through `tools`. Only Akash RPC is published, on a random loopback
port shown by `dc port validator 26657`. The overlay removes the base file's
fixed container names and ports and leaves Hermes behind its optional profile.

## Fund reporters and authorize deposits

Use the existing upgrade-test keyring. `validator0` is the default test signer.
Choose another funded key if your fixture uses a different name.

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

Stop on any funding error or timeout. Confirm all seven transfers before continuing.

Prepare a governance update from the current complete params. Preserve native
epoch configuration. The example uses seven members and a decimal threshold of
`0.67`, which requires five votes. The amounts below are local test limits.

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

The fixed gas above only generates the governance inner message; auto-gas
would try to simulate it as the governance module. Actual submitted proposals
use the normal `tx` helper and automatic gas estimation.

Set `GOV_DEPOSIT` to the required deposit from that query, expressed as a coin
string. Submit the generated message through governance, then vote with the
fixture's validator key. Wait for each transaction to commit.

```bash
jq --arg deposit "$GOV_DEPOSIT" '{messages:.body.messages, metadata:"",
  deposit:$deposit, title:"Enable local Sink", summary:"Local seven-reporter test",
  expedited:false}' "$SINK_RUN/params-tx.json" > "$SINK_RUN/proposal.json"
dc cp "$SINK_RUN/proposal.json" validator:/root/.akash/sink-proposal.json
tx gov submit-proposal /root/.akash/sink-proposal.json
```

Set `PROPOSAL_ID` from the committed submit transaction, then vote:

```bash
tx gov vote "$PROPOSAL_ID" yes
a query gov proposal "$PROPOSAL_ID" -o json
a query sink params -o json
a query sink reporters 0 -o json
```

Wait until the proposal passes and the queries show the intended params and
policy. Start the reporters:

```bash
dc up -d reporter0 reporter1 reporter2 reporter3 reporter4 reporter5 reporter6
```

These separate funded keys share one Solana validator. Production custody and
independent operators remain in [service deployment](../contracts/solana/docs/deploy-services.md).

## Deposit and inspect a mint

```bash
recipient=$(tools solana-keygen pubkey /workspace/.cache/sink-compose/recipient.json)
tx sink deposit "$recipient" 10000000uakt
# After inclusion, obtain owner/height/sequence from MsgSinkResponse or history.
sender=$(a keys show "$SINK_KEY" -a --keyring-backend test)
a query sink transfers --owner "$sender" -o json
```

Set `TRANSFER_ID` to this deposit's composite ID, then inspect it:

```bash
a query sink transfer "$TRANSFER_ID" -o json
a query sink state -o json
dc logs --tail 30 relayer reporter0
mint=$(jq -r .mint "$SINK_RUN/network.json")
tools node -e 'const {Connection,PublicKey}=require("@solana/web3.js");
  new Connection("http://127.0.0.1:8899","finalized").getTokenSupply(new PublicKey(process.argv[1])).then(r=>console.log(r.value))' "$mint"
```

Expect `STATUS_TRANSFER_PENDING`, then `STATUS_MINTED` after finalized Solana
execution and five agreeing reports. Compare
before/after values. This deposit adds exactly 10,000,000 base units to SPL
supply and Akash's minted backing. Other deposits or burns change the totals.

## Exercise a void and refund

Create an on-curve SPL token account using the same `createAccount` path as
`local/demo.ts`. It passes Akash's destination-format check but cannot receive
the bridge mint as a wallet. The real proof causes a permanent Solana void.

```bash
invalid=$(tools node --import tsx /workspace/tests/upgrade/sink/identities.cjs invalid-recipient)
tx sink deposit "$invalid" 3000000uakt
a query sink transfers --owner "$sender" -o json
```

Set `REFUND_ID` to the new record ID, then inspect it:

```bash
a query sink transfer "$REFUND_ID" -o json
a query bank balances "$sender" -o json
```

Expect a void report quorum and `STATUS_REFUND_PENDING`.
Wait until its recorded challenge deadline using chain time.
The relayer must keep the light client fresh throughout the standard 72-hour
wait. Reporters automatically finalize mature refunds when no hold or refund
epoch cap blocks payment. Expect `STATUS_REFUNDED`, no SPL supply increase for this
record, and 3,000,000 uakt returned to the original sender.

The explicit CLI instruction is also available:

```bash
tx sink finalize-refund "$REFUND_ID"
a query sink transfer "$REFUND_ID" -o json
a query sink state -o json
```

An early finalize must fail. After a reporter already finalizes, a repeat must
not pay again. If the sender pays for the manual transaction, account for its
fee separately when comparing balances. Do not shorten clocks, alter proofs,
reset epoch usage or override holds to obtain a passing refund.

## Stop and resume

```bash
dc stop reporter0 reporter1 reporter2 reporter3 reporter4 reporter5 reporter6 relayer
dc stop solana validator
# Restore the same variables and shell functions before resuming.
dc up -d --wait validator solana
dc up -d relayer reporter0 reporter1 reporter2 reporter3 reporter4 reporter5 reporter6
```

Do not rerun `prepare` or `bootstrap` on retained state. Compare chain ID,
Solana genesis hash, balances and both transfer IDs across restart. Keep source
revisions, artifact hashes, committed transaction hashes and bounded public query
results with the test record. Never share the runtime directory or raw genesis.
It contains private keys. After retaining needed evidence, `dc down` removes
only this project's containers and network. Remove only its exact `SINK_RUN`
when the test data is no longer needed. Container image/toolchain caches and
npm's own container temporary files have separate lifetimes.
