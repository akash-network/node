# Four-validator bridge rehearsal

This profile uses the native `akash testnetify` command on an isolated local seed snapshot. It runs four Akash validators, one Solana test validator, the header/proof relayer and three reporter processes. It has separate keys, ledgers and ports from the original single-validator stack.

## Run

After [setup](setup.md), from the Solana repository:

```bash
export AKT_LOCAL_RUN=dev
npm run testnet:up
npm run testnet:demo
npm run testnet:scenarios
npm run testnet:check-header
npm run testnet:verify
npm run testnet:status
```

Build prerequisites are the same as [the single-validator stack](local-stack.md#stop-restart-and-rebuild). `testnet:up` creates the testnet once and resumes it on subsequent runs. These commands use local test funds only.

Run the commands sequentially. The scenario runner deliberately stops validators, pauses minting and creates a governance proposal. It stops the normal relayer while driving proof transactions itself, then restores the services. Each demo/scenario run creates new transfers and spends development balances; native rate limits still apply.

| Service               | Endpoint                 |
| --------------------- | ------------------------ |
| Akash validator 0 RPC | `http://127.0.0.1:27657` |
| Akash validator 1 RPC | `http://127.0.0.1:27667` |
| Akash validator 2 RPC | `http://127.0.0.1:27677` |
| Akash validator 3 RPC | `http://127.0.0.1:27687` |
| Solana RPC            | `http://127.0.0.1:18999` |
| Solana WebSocket      | `ws://127.0.0.1:19000`   |

The default Akash chain ID is `akash-bridge-v2-testnetify-1`. `AKT_LOCAL_RUN=dev` selects `akash-bridge-v2-testnetify-dev-1` and `../../.cache/solana/run/testnetify-stack-v2-dev/`. Keep that variable set for every command; all run names share these ports. The mint has the same development PDA as the single-validator profile but lives on a different Solana cluster. Reporter configurations pin that cluster's genesis hash.

```bash
npm run testnet:down
npm run testnet:up
npm run testnet:verify
```

Stopping preserves both chains. The original `npm run local:*` commands continue to address the original stack on ports 26657 and 8899.

Use [the test guide](reproduce.md#burn-indexing-and-restart) to compare balances, burns, chain identity and reporter decisions across a restart.

## Initialization and trust

The launcher first creates and runs a funded native Akash seed with `x/sink`, then stops it and saves `seed-snapshot/`. It invokes `akash testnetify` on the working copy, funds four development validator operators and sets a short local governance voting period. The source is newly generated local state, not a mainnet snapshot. Sink starts at genesis with an empty escrow; a separate [existing-chain upgrade rehearsal](reproduce.md#5-existing-chain-upgrade) tests adding sink to an old binary's ledger.

After the conversion exits, the launcher copies the stopped database to the other validator homes, preserving each node's own consensus key and signing state. All database copies finish before any node starts. The saved conversion marker prevents `testnetify` from running twice. An interrupted mutation without a completion marker fails explicitly; inspect its log and preserved seed before attempting recovery.

The light client starts with one explicitly trusted checkpoint while one validator is bonded. Three `MsgCreateValidator` transactions add the other validators, one at a time, with equal stake. After each addition, the relayer proves a new signed header through the existing client. It never reinitializes the client to make the transition pass. `validator-transition-*.json` records each staking transaction and verified height.

The native tool currently logs a repeated database-close error after its completion message. The launcher checks completion, then starts the fork normally and verifies that blocks advance. The conversion log remains available for inspection; a failed conversion is never silently retried.

## Checks performed

| Scenario               | Required result                                                                                                                                                                                       |
| ---------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Validator additions    | The same light client authenticates transitions from 1 to 2 to 3 to 4 validators.                                                                                                                     |
| Invalid deposit        | Direct bank deposits and malformed MsgSink destinations reject without creating a transfer.                                                                                                           |
| Normal deposit         | MsgSink returns a record ID; 10 AKT mints and three reporters confirm the outcome on Akash.                                                                                                           |
| Invalid recipient      | 3 AKT is permanently voided, then refunded after the challenge window.                                                                                                                                |
| One validator offline  | The other three continue consensus. The four-entry commit retains one absent entry, and Solana verifies the three signatures.                                                                         |
| Altered app root       | The light client rejects the changed header and subsequently accepts the genuine signed header.                                                                                                       |
| Two validators offline | Half the voting power cannot advance consensus. Restoring validators resumes blocks.                                                                                                                  |
| Pause and retry        | A 2 AKT deposit stays pending while paused, mints after unpausing, and a repeated relay leaves supply unchanged.                                                                                      |
| Unauthorized void      | An ordinary sender cannot authorize an Akash governance void.                                                                                                                                         |
| Governance void        | A real proposal receives validator votes, writes a void order, and its proof permanently voids a 5 AKT deposit on Solana. Reporters refund it after the challenge, including while minting is paused. |
| Health and restart     | Four nodes agree on a committed block, all services run, all reporters agree with terminal outcomes and propose no duplicate actions, and supply equals accounted escrow.                             |

Fresh genesis has zero Sink escrow and zero Solana supply. Run this rehearsal with the matching SDK, node and program. Earlier scalar-ID results do not validate the current interface.

`testnet:verify` is read-only apart from writing its local result file. It checks actual RPC state, balances and reporter decisions. Run it after all deposits settle; it includes independently indexed finalized SPL burns in the escrow/supply check.

## Evidence

Everything generated lives under the ignored `../../.cache/solana/run/testnetify-stack-v2/` directory, or `../../.cache/solana/run/testnetify-stack-v2-<run>/` when `AKT_LOCAL_RUN` is set:

- `testnetify.log`, `testnetify-started.json`, `testnetify-completed.json` and `seed-snapshot/` show the fork lifecycle.
- `validator-transition-1.json` through `validator-transition-3.json` record validator additions and authenticated headers.
- `demo-result.json` records the live mint and invalid-recipient refund.
- `scenarios-result.json` records outage, pause/retry and governance-void results. `scenarios-progress.json` preserves completed checks if a later one fails.
- `health-result.json` records committed block agreement, reporter scans and reconciled supply. `health-before-restart.json` preserves the first rehearsal's pre-restart comparison.
- `akash*.log`, `solana.log`, `relayer.log` and `reporter*.log` contain service output.

The directory also contains development private keys. Share individual public result files, not the whole directory.

## Extended failure tests and limits

`npm run testnet:settlement` stops the local relayer/reporters/monitor while it drives reporter outages, threshold voting, a false-void veto, conflicting reports with governance recovery, the refund challenge/cap, a real SPL burn and payer exhaustion. It writes `settlement-scenarios.json`. Run `npm run testnet:up` afterward to resume services. Run it sequentially with other scenarios. It creates transfers and burns 2 local AKT. The demo, scenario and health commands now include prior burns in their cumulative accounting; avoid concurrent transfers or burns during their balance-delta checks.

The [100-validator benchmark](header-scaling.md) now passes equal and weighted commits through the same patched uploader. Separate runtime tests prove conflicting-header freezing and expired-client rejection. Recovery tests interrupt uploads, race operators and wait for actual lookup closure. They use separate synthetic checkpoints, never an expired retained bridge checkpoint.

All live validators and reporters run on one machine. Production key custody, independent nodes, full-size validator changes, approved upgrade policy, metadata, multisig/time locks and audits remain. The monitor scans complete chronological history without a record-count limit. Missing or pruned history still fails closed. Scan time and memory grow with history.

The Sink service uses the current native Sink interface and native protobuf proofs. Follow [launch and recovery](sink-migration-v2.md) before running these commands against a new namespace. Existing numeric-ID deployments remain separate.

## Maintained testnet receipt adapter

Run `npm run portal:testnet` with `AKT_TEAM_TESTNET=1`, `AKT_TEAM_HOST`, `AKT_TEAM_ORIGIN` and `AKT_PORTAL_DIST` set explicitly. `AKT_PORTAL_DIST` names a separately built portal dist directory. The server binds loopback port 18080. Configure your existing authenticated proxy separately. This adapter does not claim production ownership or provide production deployment automation.

When the proxy publishes Solana on a different port, set `AKT_TEAM_SOLANA_RPC_PORT` to that public RPC port. Status responses advertise it and the next port for WebSocket; local service connections still use `AKT_SOLANA_RPC_PORT`. For the hosted testnet, public 18999/19000 forward to loopback 18799/18800. Keep those forwarding and firewall rules in place.

The API is version 2. Configuration reports the transfer minimum and maximum, account epoch cap, and aggregate epoch cap in uakt. The minimum is positive while deposits are enabled. Zero disables the account cap. Both sending caps may be below the transfer maximum. Limits use Akash's configured native quota period. Owner history returns the newest 100 records, newest first, at one committed height, with a required `truncated` flag. Older records remain accessible by ID. Discovered rows have `txHash: null` and a required composite ID. Their `createdAt` is null only when the creation height is below the node's earliest retained block; other block or status failures return 503. ID receipts need no transaction index. Transaction receipts retain byte/hash/message/response/record binding. Missing and pruned state fail instead of becoming an empty history.

`local/team-contracts.ts` mirrors the portal's public contract. Holds and the five financial statuses remain separate. On-chain status 3 maps to HTTP `void_requested`; Solana `voided` remains a confirmed outcome. Origin allowlists, bounded proxy requests and responses, network genesis pins and concise errors remain enforced. The adapter logs error classes without secret-bearing messages, stacks or request bodies.

Run `SINK_PORTAL_DIR=/path/to/sink-portal npm run test:portal-contract` when
integrating either repository. The check compares the service copy with the
portal's authoritative `contracts.ts` and fails on any difference.

Origin checks do not authenticate native clients. Keep the loopback listener
behind an authenticated proxy or a tailnet with explicit access-control rules.
Requests without an Origin header rely on that access boundary.

The adapter binds loopback at `AKT_TEAM_PORT` (default 18080). API and proxy responses are bounded to 2 MiB. The history page limit is independent of native sending amount limits.
