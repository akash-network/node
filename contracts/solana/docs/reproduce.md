# Testing

Start with [setup.md](setup.md). Run commands from `contracts/solana` unless a block changes directory. A documented command is not a claim that it ran again today.

## 1. Build from a clean checkout

```bash
npm run doctor
npm run setup
```

Setup builds the native node against the chain-sdk checkout, applies the pinned verifier patch, checks its generated IDLs, builds the AKT program and header encoder, and typechecks. It generates no public-chain transactions. No original wallet, deployment key or ledger is needed. See [README.md](../README.md) for the checkouts.

The `chain-sdk` branch owns the sink schemas, generated Go and TypeScript clients,
and shared CLI. Node's ignored `.cache/solana-sdk.json` records the actual SDK
path. Setup builds the local SDK package before installing this repo. See
[SDK integration](sdk-integration.md) for generation and dependency updates.

## 2. Component and proof checks

Run these sequentially. The contract and native-proof suites use isolated Solana RPC 19899 and WebSocket 19900, plus gossip/faucet ports 19800-19850. They leave a Phantom tunnel on 8899 alone. `AKT_TEST_RPC_PORT` overrides the RPC pair.

```bash
npm run typecheck
npm run format:check
npm run test:unit
npm run test:reporter
npm run test:durable
npm test
npm run test:sink
```

| Check           | What a passing run establishes                                                                                                   | Evidence                                                                                                       |
| --------------- | -------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| `test:unit`     | Rust expiry, frozen-client and cap boundaries.                                                                                   | Console test results.                                                                                          |
| `test:reporter` | Evidence validation, signer boundaries, quorum/veto decisions and own-vote reuse.                                                | Console test results.                                                                                          |
| `test:durable`  | The OS payer lock excludes another writer and releases after process death.                                                      | Console test results.                                                                                          |
| `npm test`      | One-time genesis, mint authority, proof rejection, recipient checks, replay, caps, buffers and expiry.                           | Console results; `.cache/contract-test.*/validator.log`.                                                       |
| `test:sink`     | A signed native deposit produces a real multistore proof; Solana verifies it and returns three signed reports accepted by Akash. | Console results; node `.cache/sink-proof.json` and `.cache/sink-reporter.json`; a new contract-test directory. |

`test:sink` also runs the contract suite, so it intentionally overlaps `npm test`. These proof suites start from explicitly trusted test roots. The live network below separately verifies Akash consensus signatures.

Run native deposit, settlement and genesis tests against the same SDK used by setup:

```bash
(
  . scripts/workspace.sh
  cd "$NODE_SINK_DIR"
  GOWORK=off go test -mod=readonly -modfile=.cache/solana.mod \
    ./x/sink/... ./app ./app/types ./cmd/akash/cmd -count=1
)
```

These cover MsgSink destination and field validation, typed response IDs, status queries, signed direct/Amino transactions, rejected bank/authz/CosmWasm ingress, atomic rollback, limits, votes, conflicts, challenge windows, refunds and export/import. Dedicated live IBC ingress scenarios remain work.

## 3. Live chains, faults and settlement

All run names share ports. Inspect `testnet:status` for the current run and stop it with its existing `AKT_LOCAL_RUN` before choosing a new name. Do not delete its ledger. Use a new short name for this rehearsal if `review` already exists.

```bash
export AKT_LOCAL_RUN=review
npm run testnet:up
npm run testnet:demo
npm run testnet:verify
npm run testnet:scenarios
npm run testnet:check-header
npm run testnet:settlement
npm run testnet:up
npm run testnet:reconcile
npm run testnet:verify
```

Run one command at a time and stop on failure. `testnet:settlement` stops normal services while driving faults; the following `testnet:up` resumes them. Do not send concurrent deposits or burns during balance checks. All amounts are local test funds.

| Stage                          | Expected result                                                                                                                                                                     | File under `.cache/testnetify-stack-review/`                            |
| ------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| Startup                        | Ten services; the same light client authenticates 1→2→3→4 validator transitions.                                                                                                    | `validator-transition-1.json` through `validator-transition-3.json`.    |
| Demo                           | Direct bank deposits and malformed destinations reject; MsgSink returns IDs; 10 AKT mints; 3 AKT is voided/refunded. Fresh supply and escrow both equal 10 AKT.                     | `demo-result.json`.                                                     |
| Consensus/governance scenarios | One validator offline succeeds, two offline halt consensus, pause/retry and replay behave correctly, governance void refunds.                                                       | `scenarios-result.json`; partial progress in `scenarios-progress.json`. |
| Header check                   | Altered app root rejects; authentic signed header succeeds.                                                                                                                         | Console output and relayer log.                                         |
| Settlement                     | Reporter outage, insufficient quorum, false-void veto, conflicts/governance recovery, challenge timing, cap, repeated refund and empty-payer retry pass. Includes a 2 AKT SPL burn. | `settlement-scenarios.json`.                                            |
| Reconciliation                 | Finalized supply plus independently observed burns matches backing; pending/voided escrow and report lag are separate.                                                              | `reconciliation.json`, `burn-index.json`, `burn-events/`.               |
| Health                         | All ten services run, four validators agree, all transfers settle, each reporter proposes zero duplicate actions.                                                                   | `health-result.json`.                                                   |

Later scenario runs change cumulative totals. Compare each scenario's assertions and result files rather than expecting the first demo's supply forever. Per-account limits still apply. The [four-validator guide](testnetify.md) explains the lifecycle and ports; the [single-validator alternative](local-stack.md) exercises the same basic mint/refund flow.

### Burn indexing and restart

The settlement scenario produces a real finalized burn and checks the independently observed amount. To check rescanning and a full restart, keep the network idle after every transfer settles:

```bash
npm run testnet:verify
cp ".cache/testnetify-stack-$AKT_LOCAL_RUN/health-result.json" ".cache/testnetify-stack-$AKT_LOCAL_RUN/health-before-restart.json"
npm run testnet:reconcile
npm run testnet:reconcile
npm run testnet:down
npm run testnet:up
npm run testnet:verify
```

Allow new Solana blocks to finalize after restart. A transient stale-slot error can be retried once the node catches up; missing historical evidence must be investigated. After a successful health check, compare the saved results:

```bash
python3 - <<'PY'
import json
import os
from pathlib import Path

root = Path('.cache') / ('testnetify-stack-' + os.environ['AKT_LOCAL_RUN'])
before = json.loads((root / 'health-before-restart.json').read_text())
after = json.loads((root / 'health-result.json').read_text())
fields = [
    'chainId', 'solanaGenesisHash', 'solanaSupplyUakt', 'cumulativeBurnedUakt',
    'lockedUakt', 'mintedUakt', 'refundedUakt', 'reporters',
    'validators', 'runningServices',
]
assert after['checkedAt'] > before['checkedAt'], 'need a fresh successful health check'
assert after['height'] > before['height'], 'chain did not advance'
for field in fields:
    assert before[field] == after[field], f'changed after restart: {field}'
result = {'passed': True, 'preserved': fields, 'before': before, 'after': after}
(root / 'restart-result.json').write_text(json.dumps(result, indent=2) + '\n')
print('Balances, burns, chain identity and reporter decisions survived restart.')
PY
```

This also catches duplicate burn counting across rescans. The earlier separate 1 AKT burn audit was a manual experiment; its saved result is historical evidence, not a standalone test command. The reusable settlement scenario covers burn observation, and the comparison above checks stable totals through rescans/restart.

## 4. Verifier scale, recovery and consensus failures

After setup, run these sequentially on free port 19999. Each creates its own explicit synthetic checkpoint and stops its own validator. They do not touch the live bridge's trust state.

```bash
npm run bench:headers
npm run bench:headers -- weighted
npm run test:receipts
npm run test:recovery
npm run test:misbehaviour
npm run test:expiry
```

| Check              | Expected result                                                                                     | File under the printed `.cache/scale/run-*/` |
| ------------------ | --------------------------------------------------------------------------------------------------- | -------------------------------------------- |
| Equal benchmark    | 67/100 and 100/100 authenticate; 66/100 reaches quorum rejection within runtime limits.             | `results.json`.                              |
| Weighted benchmark | Sufficient voting power authenticates; insufficient power and changed root reject.                  | `results.json`.                              |
| Receipts           | Precompile/signature binding, ownership, duplicate insertion and capacity checks pass.              | `batch-tests.json`.                          |
| Recovery           | Interrupted uploads/assembly/cleanup, competing operators and actual lookup-table rent return pass. | `recovery-tests.json`.                       |
| Misbehaviour       | Two valid conflicting signed headers freeze the client; further updates reject.                     | `misbehaviour-tests.json`.                   |
| Expiry             | Driver refuses expired trust before spending; bypassing it still fails on chain.                    | `expiry-tests.json`.                         |

Benchmark output separates fees, refundable deposits, outstanding lookup rent and permanent rent. See [measurements](header-scaling.md) and [patch rationale](signature-batches.md). Synthetic fixtures use the normal consensus rules; no special Akash validator set is required.

The patch's host verifier tests check malformed account layouts and identities:

```bash
cargo test --locked --manifest-path .deps/ibc-contracts/Cargo.toml \
  -p tendermint-light-client-solana --lib
```

`npm run setup:light-client` reapplies or verifies the tracked patch and compares both generated IDLs. If it reports modified sources, preserve them and inspect the difference; do not overwrite local work. SBF stack-size diagnostics remain an audit item even when exercised paths pass.

## 5. Existing-chain upgrade

Build the pre-sink revision once, using the same SDK and native build helper:

```bash
(
  . scripts/workspace.sh
  git -C "$NODE_SINK_DIR" worktree add --detach .cache/upgrade-base 5193e54672e49ef328f67758ef5c91e24990c09a
  bash scripts/build-node.sh "$NODE_SINK_DIR/.cache/upgrade-base"
)
npm run test:upgrade
```

If that worktree already exists, inspect its revision and skip its creation. `AKASH_UPGRADE_BASE=/absolute/path/to/old/akash` selects an existing old binary. Ports 30657/30656/9390 must be free. The rehearsal submits a real governance upgrade, swaps binaries, and checks that sink installs with a zero balance, survives a restart and moves a preexisting sink balance to the community pool. Results go to `.cache/upgrades/run-*/results.json`. The handler is restricted to rehearsal chains and is not production activation tooling.

## Evidence and limits

Save console output as well as JSON results, the node and chain-sdk revisions, tool versions and command exit status. In Bash, use `set -o pipefail` before piping a command through `tee`; otherwise a passing `tee` can hide a failed test. Use new evidence filenames for later runs. Never report success from a stale result file after a command failed.

Generated directories also contain private development keys. Keep them ignored; share only reviewed result files. A new clone regenerates evidence and does not receive the original machine's `.cache/`.

The fresh-clone build, demo and core suites passed on 2026-10-13. Scale, recovery, consensus-fault, settlement and upgrade results predate that handoff. They were not all rerun then. The retained `milestones-v2` network later lost early Solana transaction history, so its full health check fails despite healthy supply reconciliation. Do not weaken evidence checks or reset trust to hide that failure. Complete fresh-run checks before ledger pruning or the 24-hour development trusting period becomes a factor.

Audits, production authority/checkpoint ceremonies, real full-size validator rotations, sustained load, independent reporters, archival/indexed monitoring, metadata and launch operations remain.
