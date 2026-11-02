# Running the complete local bridge

`npm run local:up` starts a native Akash validator with `x/sink`, a Solana test validator, the authenticated header/proof relayer, three signing reporters, and the continuous reconciliation monitor. Everything uses loopback RPCs and newly generated development keys under the ignored `.cache/live-stack/` directory. These keys and addresses are for local testing only.

For native `testnetify`, four Akash validators, live validator additions, outage tests and a governance-void rehearsal, use the separate [testnetify profile](testnetify.md). Both profiles can run together on different ports.

## Start and exercise it

After the [fresh-clone setup](setup.md), from the Solana repository:

```bash
npm run local:up
npm run local:demo
npm run local:status
```

The demo rejects a direct bank deposit and a malformed destination, deposits 10 AKT with MsgSink to a fresh Solana wallet, waits for a proof-backed mint and three native reporter confirmations, then deposits 3 AKT to an on-curve token account. Solana permanently voids the latter transfer. Reporters confirm the void, wait through the 20-second local challenge and refund the original sender on Akash. The demo checks exact balances, the permanent void, the untouched launch reserve and aggregate supply. It leaves both chains and all services running.

Each demo creates new transfers. The demo requires no concurrent deposits or burns during its own balance-delta checks. Its final reconciliation includes prior SPL burns. Per-account deposit limits still apply; repeatedly running the demo can reach them.

`npm run local:check-header` temporarily stops the header watcher, submits an altered header, checks that the client rejects it without storing a consensus state, then submits the authentic signed header. It resumes the watcher afterward. Run this diagnostic separately from the demo so they do not compete for the development payer.

| Service            | Endpoint                 |
| ------------------ | ------------------------ |
| Akash CometBFT RPC | `http://127.0.0.1:26657` |
| Akash gRPC         | `127.0.0.1:9090`         |
| Solana RPC         | `http://127.0.0.1:8899`  |
| Solana WebSocket   | `ws://127.0.0.1:8900`    |

The local AKT mint is `EnEomHaBpuMZZLN4TPtVAsqVofzMSkrPWYhEQ1cFjKv1`. It is a development address, not an announced mainnet token.

Inspect the services and test result:

```bash
tail -f .cache/live-stack/relayer.log
tail -f .cache/live-stack/reporter0.log
cat .cache/live-stack/demo-result.json
```

Logs also exist for `akash`, `solana`, `reporter1` and `reporter2`. `checkpoint.json` records the initial trusted height, block hash and app root. `initialized.json` records public Akash account addresses. Do not publish the directory: it also contains development signing keys.

## Stop, restart and rebuild

```bash
npm run local:down
npm run local:up
```

Stopping preserves both ledgers and keys. Restarting preserves the mint, transfer statuses, votes, escrow and any operator pause; it does not repeat genesis. The launcher checks its recorded process IDs against their workspace paths before stopping them and refuses unmanaged listeners on its RPC ports. Run lifecycle commands sequentially. The relayer and each reporter account must have only one signing process. If bootstrap stops between configuring a new bridge and enabling it, an operator must explicitly enable it after checking the configuration.

Stop this stack before `npm run test:sink`, which also uses port 8899. `npm test` and `npm run localnet` use their own ports.

On a fresh checkout, install the pinned toolchain and build first:

```bash
npm run setup
```

Setup also builds `../node/.cache/bin/akash` against the matching local SDK branch. The host header encoder links the pinned upstream Borsh codec from `.deps/ibc-contracts`; setup builds these in order. Local preloading uses fixed program IDs and needs no original deployment key.

Program files are preloaded when the Solana ledger is first created. Rebuilding `.so` files alone does not upgrade a running or retained ledger. A code-change rehearsal needs an explicit local program upgrade or a fresh paired test environment. There is no automatic reset command that could erase a running chain.

On a shared host, `local/supervise.ts` runs the four-validator profile under systemd. Every 15 seconds it restarts stopped processes. Every `AKT_MONITOR_INTERVAL_SECONDS` (default 60) it also checks:

- disk use on the runtime filesystem: warning at 80%, critical at 90%;
- Akash height and Solana slot: critical when either has not increased for `AKT_MONITOR_STALL_SECONDS` (default 300);
- the light client the bridge account pins: age against its trusting period, warning at 25%, critical at 50% or when frozen; a warning for one pass when the pinned program changes, and while the payer cannot update it;
- relayer payer balance: warning below `AKT_MONITOR_PAYER_WARN_SOL` (default 5), critical below `AKT_MONITOR_PAYER_CRITICAL_SOL` (default 1).

Level changes, and problems that persist for 15 minutes, are logged as `monitor-alert` or `monitor-clear` JSON lines with a journald priority, so `journalctl -u akt-bridge -p warning` lists them. Each pass writes `monitor.json` to the runtime directory. The checks only read and report; they never delete data or reset a chain.

## What runs on chain

Akash genesis enables deposits with an empty sink escrow, and Solana initialization creates the mint with zero supply, so every local AKT on Solana comes from a proven deposit. The local caps are 1,000,000 AKT per transfer, 10,000,000 AKT minted and 1,000,000 AKT refunded per day. Local Akash inflation is disabled to make accounting easier to inspect.

The initial light-client checkpoint is explicitly trusted. Later app roots come from actual CometBFT signed headers. The Rust helper uses the upstream Tendermint types and Borsh encoding; the relayer pre-verifies Ed25519 signatures, uploads header chunks, calls `assemble_and_update_client`, checks the stored app root, then reclaims temporary accounts. It does not deploy ICS26 or use the upstream proof API.

The relayer fetches an ICS23 proof at application height H and submits it against verified header H+1. It chooses committed application state and waits for a canonical CometBFT commit. Proofs are uploaded in chunks before the mint or void instruction. Permanent Solana status prevents a retry from issuing tokens twice.

Each pass re-reads the bridge account for the light client it pins and follows a governance re-pin, logging `light-client-changed`. The local IDs in `local/common.ts` are used only to deploy and bootstrap a fresh network. On a node with x/sink `OpenTransfers`, the relayer reads Solana statuses only for open transfers; otherwise it reads them in batches of 100, starting from the lowest record ID without one, so resolved records cost nothing. Before paying for an upload, the relayer checks the bridge pause, the per-transfer maximum, the remaining daily mint cap and whether the recipient would mint or void. It logs `relay-wait` and waits without paying while one of these retryable limits applies. A failed transfer backs off exponentially from 6 seconds up to `AKT_RELAY_RETRY_MAX_SECONDS`, default 900, and logs `relay-retry`. Header update failures back off separately, up to two minutes, and never stop relays that the latest verified height can already prove. Commits with more than `AKT_RELAY_MAX_HEADER_SIGNATURES` votes, default 100, submit only the highest-power signers the verifier needs and mark the rest `Absent`; see [large validator sets](operators.md#large-validator-sets).

Every transaction is simulated before it is sent. The relayer records the fees and resolution rent it pays per transfer in `relay-budget-<genesis>-<program>.json`. Past `AKT_RELAY_MAX_LAMPORTS_PER_TRANSFER`, default 5,000,000, it stops paying for that transfer and logs `relay-budget-exhausted`. After investigating, reset it with `npx tsx local/relay.ts --reset-budget ID` under the stack's `AKT_LOCAL_PROFILE` and `AKT_LOCAL_RUN`. The same relayer runs against any deployment from a config file; see [running a relayer](operators.md).

The relayer never relays a record that Akash shows as voided or refunded while Solana has no status for it. It re-reads the record at the newest Akash height just before its first paid transaction and skips it with `relay-final-check` if it became voided or refunded or its void order changed. In the honest flow an Akash void or refund follows a Solana void, so this state means false void reports or a governance confirmation. Minting it would leave the refund as unbacked supply. The relayer pays nothing for such a record and logs `relay-unbacked-settlement` when it first sees the state and every 15 minutes while it lasts; the reconciler reports it as a mismatch. `mint_transfer` remains permissionless, so this protects only the honest relayer. Anyone else can still submit the proof and mint.

Each client update stores a consensus state that holds about 0.0015 SOL of rent, so updates are rare. The relayer updates when a record without a Solana status needs a proof height above the latest verified height, or when the client's age on the Solana clock reaches a share of its trusting period. That share is `AKT_RELAY_REFRESH_FRACTION`, default one fifth, so an idle refresh comes before the monitor's 25% warning. One update to the newest canonical header serves every pending record in that pass, and later relays prove at the latest verified height. Every `AKT_RELAY_PRUNE_INTERVAL_SECONDS`, default 600, the relayer closes up to 20 expired consensus states other than the latest and recovers their rent through the [patched verifier](verifier-patches.md); it logs `consensus-pruned`. The relayer's Akash node must therefore keep state back to that height. Skipping trust across a long interval also requires the old validator set to hold more than a third of the new voting power; lower the fraction if the set changes quickly. This development client has a 24-hour trusting period. A long shutdown can expire it; the launcher does not replace an expired checkpoint. Production expiry recovery remains a separate governance and security decision.

Three reporters use separate funded Akash keys and submit real signed transactions. Their generated configs set no `lightClientProgram`, so they follow the bridge account too. The local set has four members and requires three votes. All three processes share this machine's Solana test validator, so this tests the protocol flow, not operator independence. After a restart, each reporter can reuse its own authenticated vote's evidence only after freshly checking finalized status, outcome and resolution slot. A first observation still requires the successful transaction and matching program event from owned node history.

## Verified locally and still outstanding

The live run verified signed header updates, rejection of an altered app root, successful mint and confirmation, automatic void followed by a refund, and preservation of the 1M reserve. The demo result records amounts and deposit transaction hashes. Accounting includes all earlier local runs, so cumulative supply can exceed the single demo's 10 AKT issuance.

The live driver and [100-validator benchmark](header-scaling.md) share compact signature batches, lookup-table uploads, durable transaction journals and delayed rent reclamation. Equal and weighted fixtures now pass. Real four-validator transitions exercise the same uploader. The benchmark uses an isolated payer for exact accounting; live payer deltas may include other activity.

The monitor stores `reconciliation.json`, `burn-index.json` and one atomic evidence file per burn under `burn-events/`. It reads finalized supply and transfer statuses, independently indexes successful SPL burns, and reports reporter lag separately. Missing retained history makes accounting unavailable. It never infers burns from a supply gap. Status reads are paged 100 accounts at a time. The first page, which includes the mint, pins the finalized slot; later pages must be at least that recent, and statuses resolved after the pinned slot are left out, so supply and statuses describe one slot. The monitor reports a mismatch when Akash shows a void without any Solana status, or when a Solana outcome has stayed pending on Akash longer than `AKT_RECONCILE_REPORT_LAG_SECONDS`, default 600, estimated at 400 ms per slot. Each pass reads every record, so production still needs an index and archival history.

For a fresh separate run use `AKT_LOCAL_RUN=<short-name>` on every lifecycle command. It changes the chain ID and storage directory, not ports. Stop the previous profile first. The validator retains one million shreds from startup for the accounting tests. Reconfiguration cannot recover already-pruned history.

Before public deployment, finish the production checkpoint/expiry ceremony, full-set transitions and long-running throughput tests, protected signer custody, independent reporter nodes and alerts. Select approved upgrade parameters, multisig/time locks and metadata, resolve reachable upstream build diagnostics, and audit all new code.

The single-validator profile also exposes Akash REST on `127.0.0.1:1318`
(the four-validator profile uses `1317`). Portal RPC, REST and developer
endpoints follow the selected profile, so parallel local stacks cannot route
wallet requests to the other Akash chain.
