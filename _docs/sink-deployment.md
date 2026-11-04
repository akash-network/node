# Deploy the Akash and Solana sides of Sink

Use this guide for an operated network. For a disposable CLI-only rehearsal,
use [local E2E](sink-local-e2e.md). The Solana [production runbook](../contracts/solana/docs/production-deployment.md)
owns custody, network parameters and launch approval; its
[program ceremony](../contracts/solana/docs/deploy-program.md) and
[service installation](../contracts/solana/docs/deploy-services.md) provide the
Solana commands. A portal is optional for the bridge.

## Prepare one matching release

Record the full node and SDK commits, SDK archive integrity, IBC commit,
program IDs, ELF/IDL hashes and service archive hash. If deploying the testnet
portal, record its commit and asset hash too. Build on Linux for Linux hosts.

From a clean node checkout, load the repository's normal direnv environment:

```bash
direnv allow
GOWORK=off make bins
GOWORK=off make test-full
go version -m .cache/bin/akash
.cache/bin/akash version --long
sha256sum .cache/bin/akash
export DEPLOY_RUN=$(mktemp -d "$PWD/.cache/sink-deploy-XXXXXX")
```

Both `pkg.akt.dev/go` modules must resolve from committed `go.mod`/`go.sum`
pins. Do not qualify a release with a development workspace or local module
replacement. The ordinary build must not contain `sinkdev`; its challenge
minimum is 72 hours. Retain the actual test output, including skipped tests.

Build the programs and Linux header encoder using [setup](../contracts/solana/docs/setup.md),
the exact locked [SDK archive](../contracts/solana/docs/sdk-integration.md),
and the pinned IBC source. Build/install the immutable service archive using
[service deployment](../contracts/solana/docs/deploy-services.md).
Do not substitute artifacts from an older fork.

## Upgrade Akash before enabling Sink

This release registers **`v2.2.0`**. The rehearsal starts with **v2.1.2** and
mainnet-derived state. A binary's git-describe version may differ from the
registered upgrade name; verify its full commit and the applied plan separately.

1. Rehearse on a separately named mainnet snapshot fork using testnetify and
   fresh test keys. Keep public-chain peers disabled. Preserve the native epoch
   schedules, consensus timing and release challenge floor.
2. At a fixed pre-upgrade height, capture supply, escrow balances, the Sink
   address balance, module versions and native epoch records. Retain the old
   binary and database backup. Never copy validator keys into a second running
   signer.
3. Stage the verified candidate at
   `$DAEMON_HOME/cosmovisor/upgrades/v2.2.0/bin/akash` on each validator.
   Keep the old binary running until the governed upgrade height. Use the
   existing Cosmovisor service and its normal restart procedure.
4. Query current governance parameters. Choose a height after voting can finish,
   with operational margin, and submit the reviewed software-upgrade proposal.
   Include the preexisting Sink-balance disposition in its summary.
5. Wait for proposal passage, the old binary's halt, Cosmovisor's switch and
   successful new blocks. Record the applied height, actual child executable
   hash, full commit and handler log. Starting directly with the new binary
   does not test the upgrade.

Set `RPC`, `CHAIN_ID`, `GOV_PROPOSER`, `GOV_DEPOSIT` and `UPGRADE_HEIGHT`
to the reviewed network values. Read the voting period and deposit first:

```bash
akash query gov params --node "$RPC" --output json
GOV_AUTHORITY=$(akash query module-name-to-address gov)
jq -n --arg authority "$GOV_AUTHORITY" --arg height "$UPGRADE_HEIGHT" \
  --arg deposit "$GOV_DEPOSIT" '{
  messages: [{"@type":"/cosmos.upgrade.v1beta1.MsgSoftwareUpgrade",
    authority:$authority, plan:{name:"v2.2.0",height:$height,info:""}}],
  metadata:"", deposit:$deposit, title:"Upgrade to v2.2.0",
  summary:"Install Sink disabled; move any preexisting Sink balance to the community pool; apply the reviewed escrow and Pyth migrations.",
  expedited:false}' > "$DEPLOY_RUN/upgrade-proposal.json"
akash tx gov submit-proposal "$DEPLOY_RUN/upgrade-proposal.json" --from "$GOV_PROPOSER" \
  --chain-id "$CHAIN_ID" --node "$RPC" --gas auto --gas-adjustment 1.5 \
  --gas-prices 0.025uakt
```

Review the generated proposal before signing. Follow the network's voting
process; the isolated single-validator rehearsal votes with its own test key.
Record the committed proposal ID and verify its passage before the target height.

The handler installs Sink, moves any preexisting Sink-address balance to the
community pool, repairs orphan escrow payments and migrates the Pyth contract.
Post-upgrade comparisons must account for those intended changes and ordinary
block mint/reward transitions. An unchanged total-supply assertion across
heights is insufficient.

On the upgraded node, set `RPC` to its Comet RPC URL and `API` to its REST URL:

```bash
akash query sink params --node "$RPC" --output json
akash query sink reporters 0 --node "$RPC" --output json
akash query sink state --node "$RPC" --output json
curl -fsS "$API/cosmos/upgrade/v1beta1/module_versions" | jq .
```

Confirm deposits/refunds are disabled, reporters are empty and Sink backing
and totals are zero. Compare balances and epoch state at adjacent heights.
The quota epoch is the existing `day` record in `x/epochs`; do not replace it
with UTC-day arithmetic or reset it during deployment.

## Connect Solana and authorize reporters

Complete the [program deployment ceremony](../contracts/solana/docs/deploy-program.md):
verify all three programs and their upgrade authorities, agree a post-upgrade
Akash checkpoint, initialize the light client and configure the bridge.
Use the reviewed native chain ID and Solana genesis identity throughout.
Start the relayer promptly so the checkpoint does not expire.

Install reporters in preview using the [service guide](../contracts/solana/docs/deploy-services.md).
Each production reporter needs its own funded Akash account, observation
infrastructure and external signer. Local test key files are only for rehearsals.

Fetch the complete current params and edit the resulting public JSON:

```bash
akash query sink params --node "$RPC" --output json |
  jq .params > "$DEPLOY_RUN/params.json"
# Set reviewed reporters, decimal threshold, limits, vaults and guardian.
# Keep enabled=false until the Solana side and reporters are ready.
akash tx sink update-params "$DEPLOY_RUN/params.json" --generate-only \
  --from "$GOV_PROPOSER" --chain-id "$CHAIN_ID" --node "$RPC" \
  --gas 200000 --output json > "$DEPLOY_RUN/params-tx.json"
```

Use fixed gas only for this unsigned inner message: auto-gas attempts to
simulate the governance module as a transaction signer. The enclosing proposal
uses normal automatic gas estimation.

Wrap the generated messages in the same proposal shape used above:

```bash
jq --arg deposit "$GOV_DEPOSIT" '{messages:.body.messages, metadata:"",
  deposit:$deposit, title:"Configure Sink", summary:"Apply the reviewed Sink parameters",
  expedited:false}' "$DEPLOY_RUN/params-tx.json" > "$DEPLOY_RUN/params-proposal.json"
```

Review and submit `$DEPLOY_RUN/params-proposal.json` with `akash tx gov submit-proposal`
using the same network/signing flags. Generating these files neither submits
nor passes a proposal. Preserve unrelated params.
After passage, query `params` and `reporters 0` again and compare them with the
approved proposal. For seven members, a decimal threshold of `0.67` requires
five votes: `ceil(7 × 0.67)`. This is an example, not a production policy choice.

Enable Solana minting through its authority, enable reporter broadcasting after
preview acceptance, then enable native deposits through governance with canary
caps. Follow the [activation order](../contracts/solana/docs/production-deployment.md#5-activation).
A new program/mint namespace must not be attached to already resolved native
deposits without a separately reviewed migration.

## Accept the deployed pair

From a funded test account, make a reviewed canary deposit:

```bash
akash tx sink deposit "$SOLANA_RECIPIENT" 1000000uakt \
  --from "$SENDER_KEY" --chain-id "$CHAIN_ID" --node "$RPC" \
  --gas auto --gas-adjustment 1.5 --gas-prices 0.025uakt
# After inclusion, take owner/height/sequence from EventTransferCreated.
akash query sink transfer "$TRANSFER_ID" --node "$RPC" --output json
akash query sink state --node "$RPC" --output json
```

Verify the finalized Solana transaction and recipient balance, native mint
quorum and exact escrow backing. Retain transaction IDs, heights/slots and the
full composite transfer ID. Exercise an invalid-recipient void, early-refund
rejection and a paid refund after its actual deadline. A release refund is
unfinished until the release-duration challenge has elapsed; an accelerated
`sinkdev` run proves only its separately identified test configuration.

Restart retained databases/services and compare the same historical state
before/after. Reconcile mint supply and burns against native backing. Test
monitoring, signer failures and service recovery using the linked runbooks.
Keep these results separate from unit tests, Compose configuration checks and
upgrade success.

For a testnet portal, deploy assets from the same accepted pair, verify the
chain/genesis/program/mint reported by its API, then test a fresh wallet. The
local faucet server is a testnet tool; production needs its separately reviewed
portal/API. An old URL may redirect to the accepted portal when host routing
prevents a direct gateway.

## Stop or recover

Pause new deposits through the configured guardian or governance when the pair
cannot safely settle. Preserve chain data, transaction receipts and relayer
journals. Follow [incident recovery](../contracts/solana/docs/production-deployment.md#incident-runbooks)
for an expired/frozen client or disputed settlement.

After the native upgrade commits, do not start the old binary on the migrated
database. A rollback requires the network's coordinated state-recovery plan.
A service rollback must retain the current durable transaction/spend records;
it cannot undo on-chain transactions. Keep historical failed rehearsals and
unfinished import/refund checks explicit in the deployment handoff.
