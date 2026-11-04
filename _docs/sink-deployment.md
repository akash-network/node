# Deploy Sink

Upgrade Akash, deploy the Solana programs, then connect the relayer and reporters.
For a disposable CLI test, use [local E2E](sink-local-e2e.md).

## Contents

- [Build the release](#build-the-release)
- [Upgrade Akash](#upgrade-akash)
- [Connect Solana](#connect-solana)
- [Configure and enable Sink](#configure-and-enable-sink)
- [Verify the deployment](#verify-the-deployment)
- [Stop or recover](#stop-or-recover)
- [Solana production runbook](../contracts/solana/docs/production-deployment.md)

## Build the release

From a clean Linux node checkout, load the normal direnv environment:

```bash
direnv allow
GOWORK=off make bins
GOWORK=off make test-full
go version -m .cache/bin/akash
.cache/bin/akash version --long
sha256sum .cache/bin/akash
export DEPLOY_RUN=$(mktemp -d "$PWD/.cache/sink-deploy-XXXXXX")
```

Verify committed SDK pins, no local replacements and no `sinkdev` tag. The
release challenge minimum is 72 hours. Retain the test output, including skips.

Build the [Solana programs](../contracts/solana/docs/setup.md) with the locked
[SDK archive](../contracts/solana/docs/sdk-integration.md) and pinned IBC source.
Build the [service archive](../contracts/solana/docs/deploy-services.md).
Record source commits, program IDs, binary and IDL hashes, and archive hashes.

## Upgrade Akash

1. Rehearse v2.1.2 to registered `v2.2.0` on an isolated testnetify snapshot fork.
   Use fresh keys, disable public peers and preserve epochs, consensus timing
   and the release challenge.
2. Back up the database and old binary. At one height, capture supply, escrow,
   Sink balance, module versions and epochs. Never run two signers with the same key.
3. Stage the candidate at `$DAEMON_HOME/cosmovisor/upgrades/v2.2.0/bin/akash`
   on each validator. Keep the old binary running until the upgrade height.
4. Set `RPC`, `CHAIN_ID` and `GOV_PROPOSER`. Query governance below, then set
   `GOV_DEPOSIT` and an `UPGRADE_HEIGHT` that leaves enough time for voting.

```bash
akash query gov params --node "$RPC" --output json
```

Set the deposit and target height from that query, then generate the proposal:

```bash
GOV_AUTHORITY=$(akash query module-name-to-address gov)
jq -n --arg authority "$GOV_AUTHORITY" --arg height "$UPGRADE_HEIGHT" \
  --arg deposit "$GOV_DEPOSIT" '{
  messages: [{"@type":"/cosmos.upgrade.v1beta1.MsgSoftwareUpgrade",
    authority:$authority, plan:{name:"v2.2.0",height:$height,info:""}}],
  metadata:"", deposit:$deposit, title:"Upgrade to v2.2.0",
  summary:"Install Sink disabled; move any preexisting Sink balance to the community pool; apply the reviewed escrow and Pyth migrations.",
  expedited:false}' > "$DEPLOY_RUN/upgrade-proposal.json"
```

Review the proposal, then submit it:

```bash
akash tx gov submit-proposal "$DEPLOY_RUN/upgrade-proposal.json" --from "$GOV_PROPOSER" \
  --chain-id "$CHAIN_ID" --node "$RPC" --gas auto --gas-adjustment 1.5 \
  --gas-prices 0.025uakt
```

Follow the network's voting process. Require proposal passage, Cosmovisor's
switch at the scheduled height and new blocks. Record the applied plan,
running executable hash and full commit; git-describe is not the upgrade name.

Set `API` to the upgraded node's REST URL and check Sink:

```bash
akash query sink params --node "$RPC" --output json
akash query sink reporters 0 --node "$RPC" --output json
akash query sink state --node "$RPC" --output json
curl -fsS "$API/cosmos/upgrade/v1beta1/module_versions" | jq .
```

Expect disabled deposits and refunds, empty reporters, zero Sink totals and
backing. Compare adjacent heights: the handler sweeps preexisting Sink funds
to the community pool, repairs orphan escrow payments and migrates Pyth.
Account for those changes and normal block rewards. Preserve the native `day`
epoch schedule used for quotas.

## Connect Solana

Follow these guides in order, using the same Akash chain ID and Solana genesis:

1. [Deploy and initialize the programs](../contracts/solana/docs/deploy-program.md).
   Verify all three programs and authorities, then trust a post-upgrade Akash checkpoint.
2. [Install the relayer and reporters](../contracts/solana/docs/deploy-services.md).
   Start the relayer promptly to keep the checkpoint fresh. Run reporters in preview.

Give each production reporter a funded Akash account, external signer and
independent observation infrastructure. Do not attach a new mint to already
resolved native deposits without a reviewed migration.

## Configure and enable Sink

Fetch the complete params:

```bash
akash query sink params --node "$RPC" --output json |
  jq .params > "$DEPLOY_RUN/params.json"
```

Edit `params.json` with the approved reporters, decimal threshold, limits,
vaults and guardian. Preserve unrelated fields. Keep both `enabled` and
`refunds_enabled` false until activation, then generate the unsigned message:

```bash
akash tx sink update-params "$DEPLOY_RUN/params.json" --generate-only \
  --from "$GOV_PROPOSER" --chain-id "$CHAIN_ID" --node "$RPC" \
  --gas 200000 --output json > "$DEPLOY_RUN/params-tx.json"
```

Keep fixed gas for this unsigned message; auto-gas tries to simulate the
module signer. Wrap the message in a governance proposal:

```bash
jq --arg deposit "$GOV_DEPOSIT" '{messages:.body.messages, metadata:"",
  deposit:$deposit, title:"Configure Sink", summary:"Apply the reviewed Sink parameters",
  expedited:false}' "$DEPLOY_RUN/params-tx.json" > "$DEPLOY_RUN/params-proposal.json"
```

Review and submit `$DEPLOY_RUN/params-proposal.json` with the same network and signing flags
as the upgrade proposal. After passage, query `params` and `reporters 0` and
compare with the approved values. For example, seven reporters at `0.67`
require five votes. Choose the actual policy through governance.

Follow the [activation sequence](../contracts/solana/docs/production-deployment.md#5-activation):
enable Solana, enable reporter broadcasting after preview checks, then enable
native Sink through governance with small canary limits.

## Verify the deployment

Set `SOLANA_RECIPIENT` and `SENDER_KEY` for a funded canary account:

```bash
akash tx sink deposit "$SOLANA_RECIPIENT" 1000000uakt \
  --from "$SENDER_KEY" --chain-id "$CHAIN_ID" --node "$RPC" \
  --gas auto --gas-adjustment 1.5 --gas-prices 0.025uakt
# After inclusion, take owner/height/sequence from EventTransferCreated.
akash query sink transfer "$TRANSFER_ID" --node "$RPC" --output json
akash query sink state --node "$RPC" --output json
```

Check the finalized Solana transaction, recipient balance, native report quorum
and exact backing. Retain IDs, heights, slots and before/after balances.

Also verify:

- An invalid recipient produces a void; early refund fails; payment occurs only
  after the recorded deadline. A `sinkdev` refund does not prove the release wait.
- Retained database and service restarts preserve records, balances and journals.
- Mint supply and burns reconcile with native backing.
- Monitoring and signer recovery work using the service runbooks.

If deploying a testnet portal, verify its chain, genesis, program and mint, then
test with a fresh wallet. The local faucet is testnet-only. Record unfinished
checks separately, including unpaid refunds and untested full-state imports.

## Stop or recover

Pause new deposits through the guardian or governance. Preserve data and journals;
follow the [incident runbooks](../contracts/solana/docs/production-deployment.md#incident-runbooks).
Do not start the old binary on a migrated database. Database recovery requires
a coordinated network plan; rolling back a service cannot undo chain transactions.
