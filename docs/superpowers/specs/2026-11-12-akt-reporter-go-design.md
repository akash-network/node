# akt-reporter: Go reporter design

|               |                                                                                                |
| ------------- | ---------------------------------------------------------------------------------------------- |
| Status        | Draft for review                                                                               |
| Date          | 2026-11-12                                                                                     |
| Scope         | The sink reporter (`contracts/solana/reporter`)                                                |
| Replaces      | The TypeScript reporter release: `reporter/v0.1.x` tags, `ghcr.io/akash-network/akt-reporter` |
| First release | `reporter/v0.2.0`                                                                              |

## 1. Summary

The sink reporter moves from TypeScript to a Go binary, `akt-reporter`, built from the node module without CGO. Its decision logic and evidence checks are ported one-to-one. The speed comes from how it reads and writes, not from relaxed checks:

- Akash state is read through the node's gRPC endpoint, every read pinned to one height, many reads in flight.
- Solana accounts are read 96 transfers at a time in one finalized snapshot. Each block is fetched once and indexed.
- Akash snapshots are cached per height and Solana block times per slot.
- Messages are sent in batched, unordered Akash transactions. Each transaction's timeout comes from a fixed nonce slot of its messages, so the chain itself refuses a second execution of any vote, veto or refund.

Configuration follows `cmd/akash`: cobra and viper, `--home`, a TOML file, `AKT_REPORTER_*` environment variables and flags. GoReleaser builds static binaries and a multi-arch distroless image whose default command starts the reporter.

## 2. Background

### 2.1 The TypeScript reporter

The reporter lives in `contracts/solana/reporter/` (`run.ts`, `scan.ts`, `akash.ts`, `solana.ts`, `policy.ts`, `config.ts`, `signer.ts`, `key.ts`), with listing in `local/open-transfers.ts` and ID helpers in `shared/id.ts` and `shared/proof.ts`. `deploy/operator.ts` packages it with a Node runtime, and `.github/workflows/release-operators.yaml` publishes it on `reporter/v*` tags.

Each pass reads the light client and Akash state, then lists every dispute and every refund-pending, void-requested and transfer-pending transfer at one height. It adds one page of a rolling chronological audit and reviews the transfers one at a time. One review costs about ten round trips before any block fetch:

- Akash: `status`, `abci_info`, `block`, and `abci_query` for State, Params, Reporters and Transfer.
- Solana: `getGenesisHash`, `getMultipleAccounts`, `getBlockTime`, and `getBlock` for a first observation.

Each planned message goes out in its own transaction, and the reporter waits up to 30 s for inclusion before moving on. `contracts/solana/docs/reporter.md` already warns that pass time and memory grow with the backlog.

### 2.2 Platform facts the design relies on

- **ABCI lock.** The akash node starts CometBFT with `proxy.NewLocalClientCreator` (`chain-sdk/go/cli/server.go:314`). That is one mutex for all ABCI connections, held by every `abci_query` (`abci/client/local_client.go:102` in the CometBFT fork). Heavy `abci_query` traffic therefore runs one query at a time and delays block execution. BaseApp's gRPC server serves the same queries without that mutex. A request pins its height with the `x-cosmos-block-height` header and the response repeats it (`baseapp/grpcserver.go:50-77` in the cosmos-sdk fork).
- **Unordered transactions** are enabled (`app/types/app.go:284`) with the SDK defaults. In the cosmos-sdk fork:
  - `timeout_timestamp` must not be before the executing block's time, and at most 10 min after it (`x/auth/ante/sigverify.go:431-450`);
  - each costs 2,240 extra gas;
  - the signer sequence must be 0;
  - the replay key is (timeout in nanoseconds, signer). It is checked when a node admits a transaction and again when a block executes it. A reused key fails the ante handler, so the transaction has no effect and pays no fee (`x/auth/keeper/keeper.go:345-354`, `baseapp/baseapp.go:928-955`). Simulation skips the check;
  - a replay key is kept until the start of the first block whose time reaches its timeout (`PreBlock` in `x/auth/module.go:102`, `x/auth/keeper/keeper.go:359-361`).
- **Block times strictly increase** (CometBFT BFT time), and a read pinned to height H reflects every block up to H.
- **Duplicates cost a fee.** `MsgReportOutcome` succeeds without effect for duplicate or late same-outcome votes (`x/sink/keeper/settlement.go:88-139`), and `MsgFinalizeRefund` for a refund already paid (`settlement.go:395-397`), so a duplicate still pays its fee. Apart from invalid input, such as a signer outside the policy or an unknown transfer, a vote fails only on a stale policy or round.
- **TS transactions** are ordered, with `timeout_height` 20 blocks above the snapshot height (`contracts/solana/reporter/akash.ts:316-325`).
- **Solana v1 transactions** (SIMD-0385) have been on mainnet since 2026-09-15. The node pins `gagliardetto/solana-go` v1.14.0 through chain-sdk, and that version cannot decode them in binary form. Upgrading it would change the consensus binary's dependencies. The TS reporter reads blocks and transactions as JSON, which needs no binary decoding.
- **CGO.** `pkg.akt.dev/go/cli`, which holds akash's config handling, builds only with CGO because of wasmd and wasmvm. Everything the reporter needs builds with `CGO_ENABLED=0` against the module's pinned dependencies (`GOWORK=off`): chain-sdk sink types, cosmos-sdk client, tx, keyring and keys packages, the CometBFT RPC client, solana-go keys, grpc, viper and prometheus.

## 3. Goals, non-goals and success criteria

### Goals

1. The same decisions and evidence checks as the TS reporter for the same chain state.
2. A steady-state pass over 100,000 open transfers in at most 60 s, with bounded memory.
3. Akash-style configuration and CLI.
4. GoReleaser-built binaries and images. The image starts the reporter with no arguments.
5. Unordered Akash transactions.
6. Signing in-process (Cosmos keyring) or through a Cosmos-KMS `SignerService`.
7. No repeated votes: each vote, veto and refund executes at most once, as defined in section 6.1, across restarts, processes and nodes.

### Non-goals

- Porting the relayer; it stays in TypeScript.
- Changing `x/sink`, the AKT Solana program, or the node's dependency versions. In particular, solana-go and chain-sdk are not upgraded.
- Event-driven observation through websocket subscriptions.
- Persisting reporter state across restarts.
- The external-command signer; it is removed.

### Success criteria

1. **Parity.** Every golden scenario (section 11.2) produces events structurally identical to the TS reporter's, under the ordering rules in section 9.1. The only exceptions are the intentional differences listed in section 5.6.
2. **Review throughput.** The `steady-100k` benchmark (section 11.4) completes a pass in at most 60 s, with no failures, the expected decisions, and peak heap at most 512 MiB, on a 4-vCPU GitHub runner.
3. **Build.** `CGO_ENABLED=0 GOWORK=off` builds succeed for linux and darwin on amd64 and arm64.
4. **Image.** `docker run <image>` starts scanning when configuration comes from the environment or a mounted home.
5. **Broadcast.** Component tests (section 11.3) show:
   - unordered transactions with sequence 0 and slot timeouts (section 6.1);
   - batching per slot, send priority, simulation pruning and fee caps;
   - the slot table: de-duplication within a window, deferral to the next window, and releases after every failure before submission;
   - a vote backlog draining while reviews proceed without waiting;
   - refunds progressing through the refund queue without waiting for another pass.
6. **Node load.** On the same runner, the real application's gRPC server, seeded with 100,000 transfers, sustains at least 2,000 height-pinned `Transfer` queries per second from 32 concurrent clients (section 11.4).
7. **No repeated execution.** `TestNoVoteExecutesTwice` (section 11.3) finds no vote, veto or refund executed more than once across randomized lower snapshots, restarts, lost responses, two processes sharing a key, and nodes with and without a transaction index. A test on the real application shows the chain refusing a reused replay key without charging a fee.

## 4. Decisions

| #   | Decision                                                                                                           | Rejected alternatives                                  |
| --- | ------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------ |
| D1  | Port the reporter only. The TS reporter source stays, because the relayer, the local stack and tests import it.    | Port the relayer too                                   |
| D2  | New akash-style CLI and configuration; the JSON-lines event and alert schema is kept.                              | Drop-in TS CLI; log-based events                       |
| D3  | Target 100,000 open transfers per pass in at most 60 s.                                                            | 10,000; 1,000,000+ with streaming and no re-reads      |
| D4  | A concurrent pipeline per pass.                                                                                    | Event-driven; straight port with a worker pool         |
| D5  | Akash state reads go through gRPC; CometBFT RPC serves status, blocks, broadcast and transaction lookup.           | `abci_query` only; gRPC optional                       |
| D6  | Unordered transactions only.                                                                                       | Ordered transactions with sequence tracking            |
| D7  | Sign with the Cosmos keyring or a Cosmos-KMS `SignerService`. The exec signer is dropped.                         | Keep the exec signer; cloud KMS SDKs in-process        |
| D8  | Distroless static image, UID 10001, `ENTRYPOINT ["/usr/bin/akt-reporter"]`, `CMD ["start"]`.                       | `ubuntu:noble` base                                    |
| D9  | The code lives in the node module and builds without CGO.                                                          | A separate Go module                                   |
| D10 | Solana blocks and transactions are decoded from JSON.                                                              | Upgrade solana-go to v1.23+                            |
| D11 | The first Go release is `reporter/v0.2.0`.                                                                         | None                                                   |
| D12 | Every transaction's timeout comes from a fixed nonce slot of its messages, so the chain refuses repeats (section 6.1). | A write-ahead journal; a broadcast pause after every start |

## 5. Architecture

### 5.1 Packages

Every package lives in module `pkg.akt.dev/node/v2` and builds with `CGO_ENABLED=0`.

| Package                              | Responsibility                                                                                                                          |
| ------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------- |
| `cmd/akt-reporter`                   | `main`, cobra commands, configuration loading                                                                                           |
| `operator/reporter/config`           | Config struct, defaults, TOML template, validation, network guard                                                                       |
| `operator/reporter/sinkid`           | Compact transfer IDs: canonical parsing, ordering, key suffix                                                                           |
| `operator/reporter/akash`            | gRPC and CometBFT clients, height tracker, snapshot cache, listing, re-reads                                                            |
| `operator/reporter/solana`           | JSON-RPC client, PDAs, account, event and mint decoders, program-log attribution, streaming block decoder, per-slot evidence index      |
| `operator/reporter/observe`          | Port of `SolanaObserver`: chunk snapshots, deployment checks, light-client pin or follow, resolved and absent paths                     |
| `operator/reporter/policy`           | Port of `policy.ts`; pure functions                                                                                                     |
| `operator/reporter/scan`             | Pass orchestration: list, merge, chunk, review, audit cursor, summary                                                                   |
| `operator/reporter/tx`               | Nonce slots, batching, unordered transaction building, simulation, broadcast, confirmation, slot table, follow-up reviews                                       |
| `operator/reporter/signer`           | Keyring signer, Cosmos-KMS `SignerService` client, signature normalization and verification                                            |
| `operator/reporter/events`           | JSON-lines event writer                                                                                                                 |
| `operator/reporter/metrics`          | Prometheus collectors and HTTP endpoint                                                                                                 |
| `operator/reporter/internal/simnode` | Test simulator of Akash (gRPC and CometBFT RPC) and Solana JSON-RPC                                                                     |

Every dependency is already in `go.mod`. The Cosmos-KMS proto is copied into `operator/reporter/signer` at a pinned upstream commit, and its generated Go code is committed. The `cosmos/kms` module itself is not imported.

### 5.2 One pass

1. **Start.**
   - Take a Solana deployment snapshot without status accounts (section 5.4). A light-client pin mismatch ends the pass.
   - Take the tracker's Akash snapshot S(H) (section 5.3) and require `reporter` to be a member of the current policy. Otherwise the pass ends with `not a current reporter`.
2. **List** at height H. These run in parallel:
   - all disputes;
   - the refund-pending, void-requested and transfer-pending buckets;
   - one audit page from the audit cursor.

   Only compact IDs are kept, about 100 bytes each. A listing error ends the pass. An audit error writes `audit-unavailable`, counts one failure and keeps the cursor; the pass continues.
3. **Merge** in the TS order: disputes, refund-pending, void-requested, transfer-pending, then audit entries not already present. Each ID keeps its first position.
4. **Review** in chunks of up to 96 IDs, with `scan.workers` chunks in flight. For each chunk:
   1. Take the tracker's latest snapshot S(H′) and re-read each transfer at H′.
   2. Take one Solana snapshot of the deployment accounts and the chunk's status accounts.
   3. For each transfer:
      - take the audit early exit (section 5.4), or make the resolved or absent observation;
      - plan (section 5.5);
      - write the event;
      - queue its messages for broadcast (section 6).
5. **Summary**, once every chunk is done: `{scanned, audited, index, failures, lightClientProgram}`.

### 5.3 Akash reads

**Transports.**

- **gRPC (`grpc-addr`)** carries every state read:
  - `akash.sink.v1.Query/{State, Params, Reporters, Transfer, Transfers, Disputes}`;
  - `cosmos.auth.v1beta1.Query/Account`;
  - `cosmos.tx.v1beta1.Service/Simulate`.

  Each `akash.sink.v1` read sends `x-cosmos-block-height: H`, and the response header must repeat H, as the TS reporter checks `reply.height`. The server echoes the height it was asked for, and answers a request that arrived without the header at its latest height. The check therefore catches a header lost on the way; that the read used H rests on the server honoring the header. `Account` (read once at startup) and `Simulate` run against the node's latest state. Messages are gogoproto, encoded with the SDK's gRPC codec. The client accepts responses up to 64 MiB.

  A node serves gRPC without TLS. Loopback addresses therefore always use plaintext. Other addresses use TLS unless `grpc-insecure` is set.
- **CometBFT RPC (`node`)** carries `status`, `abci_info`, `block`, `broadcast_tx_sync` and `tx`.
- **Same node.** At startup, CometBFT `/status` `node_info.id` must equal `default_node_info.default_node_id` from gRPC `cosmos.base.tendermint.v1beta1.Service/GetNodeInfo`. The two endpoints must be one node so that height H exists on both.

**Height tracker** (port of `AkashClient.state()`).

- A refresh runs these steps:
  1. Read `status`: the network must equal `chain-id`, the node must not be catching up, and the latest block's age must be in [−10 s, `max-age`].
  2. Read `abci_info`, and take H = min(latest block height, application last block height). H must be greater than 1.
  3. Read `block(H)`: its height and chain ID must match, and its age must be in [−10 s, `max-age`].
  4. Read State, Params and Reporters at H, with the TS checks:
     - exactly one current policy, with version > 0;
     - params, totals and both usages present;
     - epoch ≤ 2^63 − 1;
     - usage epochs equal to the state epoch;
     - no usage before epoch counting starts.
- At most one refresh runs per second, and concurrent callers share an in-flight refresh. A failed refresh is also cached for one second, so a broken node is not hammered.
- A snapshot is immutable. It holds the height, block time, policy, params, refund usage, epoch and refund head.

**Listing** (port of `open-transfers.ts` and `AkashClient.transfers` and `disputes`).

- Pages of `scan.page-size` are read at the fixed height H, with these checks:
  - the response height is H;
  - there are at most `limit` entries;
  - pagination is present;
  - every entry decodes (canonical ID, canonical recipient, positive amount ≤ 2^64 − 1) and has a settlement;
  - status and owner filters hold, and disputes are held;
  - no ID appears twice, and the cursor advances.
- The four sources run concurrently; each pages sequentially.
- The audit reads one history page of min(`scan.audit-per-scan`, `scan.page-size`) entries from its cursor. It applies the TS chronological checks and the next/tail cursor rules.

**Re-reads.**

- Each ID in a chunk is read with `Transfer` at H′.
- gRPC `NotFound` maps to `Sink record not found`. Any other error means the node is unavailable.
- The returned ID must equal the requested ID, and a settlement must be present.
- The plan uses S(H′) with transfers read at H′, so every decision uses one height.

### 5.4 Solana reads

**Client.** JSON-RPC over HTTP with keep-alive, finalized commitment and `request-timeout` per request. Any transport or RPC error becomes `Solana node is temporarily unavailable`, as in TS. The exception is `getBlock`: its errors mean missing evidence (see the evidence index below). Methods:

- `getGenesisHash`;
- `getMultipleAccounts` (`base64` encoding);
- `getBlockTime`;
- `getBlock` (`encoding: json`, `transactionDetails: full`, `rewards: false`, `maxSupportedTransactionVersion: 1`);
- `getTransaction` (`encoding: json`, `maxSupportedTransactionVersion: 1`).

**Chunk snapshot** (port of `finalizedAccounts`).

1. `getGenesisHash` must equal `solana.genesis-hash`; otherwise `wrong Solana cluster`.
2. One `getMultipleAccounts` covers the program, `PDA("config")`, `PDA("bridge")`, the mint and the chunk's status PDAs. The RPC allows at most 100 keys per call, which is why a chunk holds 96 transfers. The context slot must be positive.
3. `getBlockTime(slot)` is cached per slot. The age is checked when the snapshot is taken, before any evidence fetch, and must be in [−10 s, `max-age`]; otherwise `stale Solana finalized state`. A missing (null) block time counts as stale, as in TS.
4. Deployment checks:
   - the program is executable;
   - the launch and bridge accounts are owned by the program and not executable;
   - launch version is 2, and launch mint equals `solana.mint`;
   - bridge chain ID equals `chain-id`;
   - the light-client check on the bridge account's `light_client_program` (section 5.6);
   - the mint is owned by the SPL Token program, is 82 bytes, initialized, with 6 decimals, no freeze authority, and mint authority `PDA("mint_authority")`.

The pass-start snapshot (section 5.2) runs the same checks without status accounts.

**Resolved path** (port of `observeFinalized` and `observe`).

- The status account must be owned by the program and not executable, and must decode. It must match all three ID components, with proof height > deposit height and 0 < resolved slot ≤ context slot. A mint must also match the Akash amount and recipient.
- Evidence: reuse this reporter's own stored vote when its outcome and slot match and its signature is 64 bytes, not all zero. Otherwise look the status up in the per-slot evidence index.

**Audit early exit.** An audit entry whose status is terminal is checked against the chunk snapshot first. When Solana resolved it and `settledConsistently` holds, the review stops and writes the short audit event (Appendix A).

**Per-slot evidence index** (port of `resolutionSignature` and `resolves`).

- **Fetch.** One `getBlock` per slot, shared by concurrent lookups and limited by `scan.block-concurrency`.
- **Decode.** The block is decoded as a stream, one transaction at a time; it is never held whole in memory. A transaction becomes a candidate when it succeeded, has log messages, and its logs yield at least one `TransferResolved` event from an AKT program frame. For each such event the index keeps the event, the transaction's first signature, and whether the transaction writes the status PDA derived from the event's ID. A matching event carries the status's own ID, so that PDA is exactly the status account a lookup asks about.
- **Retention.** The indexes of the 256 most recent slots are kept, within 64 MiB in total, evicting the oldest slot first. An evicted slot is fetched again on its next lookup.
- **Lookup.** For a status at slot S, select entries whose event matches the status and whose transaction writes the status account. Matching covers the ID, proof height, resolved slot and outcome (amount and recipient, or void reason).
  - Two or more matching transactions give `multiple resolution transactions in block`.
  - None gives `resolution evidence unavailable in its block`.
  - A missing, skipped or pruned block, or an RPC error, gives `resolution evidence unavailable: block missing from owned node history`.
- **Writability.** Static keys use the message header's signed and unsigned read-only counts. They are followed by `meta.loadedAddresses.writable`, then `meta.loadedAddresses.readonly`. These are the rules web3.js applies to legacy, v0 and v1 messages. A transaction version other than legacy, 0 or 1 supplies no evidence.

**Absent path** (port of `absent` and `showsReported`).

- slotLimit = context slot + lead. The lead is `solana.max-slot-lead`, or (max-age + 60 s, rounded up to whole seconds) × 5 slots per second when that key is 0.
- A reported slot in (context slot, slotLimit] gives `Solana node is behind reported evidence; absence cannot authorize a veto`.
- Each reported observation at or below the context slot is looked up with `getTransaction` (finalized). If the transaction exists at that slot with that first signature, and resolves the status with the reported outcome, the result is `reported Solana transaction contradicts missing status`.

**Program logs** (port of `programEvents`). The invoke and exit patterns and the invoke-depth checks are the same as in TS. `Program data:` lines are attributed to the executing frame, including CPI frames. Agave writes `Log truncated` once and keeps logging later messages that still fit, so parsing stops at the first `Log truncated`: only top-level AKT frames that closed with `success` before it count, and the lines after it are ignored. Unbalanced logs supply no evidence.

**Decoders.**

- Anchor discriminators come from the AKT IDL: accounts `LaunchConfig`, `BridgeConfig` and `TransferStatus`; event `TransferResolved`.
- Borsh layouts follow `programs/akt/src/state.rs`, `events.rs` and `wire.rs`:
  - `TransferId` is `{owner: Vec<u8> of 1 to 255 bytes, height: u64, sequence: u64}`;
  - `TransferOutcome` is `Minted {amount: u64, recipient: Pubkey}` or `Voided {reason: InvalidRecipient | Governance}`;
  - unknown enum tags are rejected.
- Accounts may carry trailing bytes, because their space is allocated up front and Anchor's decoder accepts that. Events must be consumed exactly.
- A `TransferResolved` line that fails to decode voids that transaction's evidence. This matches TS, where the decode error inside `resolves` yields false.

### 5.5 Decision logic

`policy` ports `policy.ts` function by function:

- `challengeSeconds`;
- `reporterRatio` and `reporterThreshold`, on `math.LegacyDec`: the threshold is ceil(ratio × members), and the quorum must exceed two thirds;
- `reportedObservations`, `minimumSlot`, `compareOutcome`, `settledConsistently` and `planOutcome`.

Amounts use `math.Int`, and coins must be `uakt`. The package builds three messages:

- `MsgReportOutcome`;
- `MsgVetoVoid`, with reason `No Solana status at finalized slot <slot>`;
- `MsgFinalizeRefund`.

It does no I/O.

### 5.6 Concurrency, caches and guarantees

| Limit                                         | Default | Key                       |
| --------------------------------------------- | ------- | ------------------------- |
| Chunks in flight                              | 16      | `scan.workers`            |
| Concurrent Akash gRPC reads                   | 32      | `scan.akash-concurrency`  |
| Concurrent Solana requests, excluding blocks  | 8       | `scan.solana-concurrency` |
| Concurrent block fetches                      | 2       | `scan.block-concurrency`  |
| Listing page size                             | 500     | `scan.page-size`          |
| Per-request timeout                           | 15 s    | `request-timeout`         |

Caches are keyed by height or slot, so nothing is ever invalidated:

- the latest Akash snapshot;
- block time per slot (4,096 slots);
- the evidence index per slot (256 slots, at most 64 MiB).

Each HTTP endpoint gets one keep-alive connection pool sized to its concurrency.

**Guarantees kept from TS.**

1. Each decision uses one Akash height and one finalized Solana slot.
2. A `light-client-changed` alert is written before any event decided under the new client. On every observation, the followed program and the highest observed snapshot slot are updated together under one lock, and the alert is written under the same lock. An observation from an older slot is ignored, as in TS (`solana.ts:271-275`).
   - **Limitation.** Because the follow only moves to higher slots, a re-pin and a re-pin back within one chunk's flight (A→B→A) can leave a chunk decided under B without its own alert: a later chunk's snapshot already showed A again and was followed first, so the older snapshot naming B is ignored rather than alerted.
3. A light-client pin mismatch stops the pass.
   - No new chunk starts, and running chunks are cancelled; events they already wrote stand.
   - The `light-client-mismatch` alert and an error are written once every running chunk has stopped, so no review event follows them. No summary is written.
   - From the moment a chunk finds the mismatch until the next pass begins, follow-up reviews (section 6.6) are skipped. One already running stops before its next review or `{id, error}` event, counting nothing more, and never resumes, even after the next pass begins. A follow-up that finds the same change writes no alert of its own, so the pass writes exactly one mismatch alert, followed by its error.
   - A deferred message whose follow-up review is skipped gets `{id, error: "sink message not sent: deferred (<reason>) until <time>"}`, counts as a failure and is released, so `scan` does not wait for it window after window.
   - Messages already planned are still sent; TS had sent them before it reached the mismatch.
4. Pass-start identity and freshness failures, and listing errors, end the pass with an error line and no summary. During reviews, snapshot and observation errors fail only the affected transfers (`{id, error}`), and the pass continues.
5. Stale nodes, missing history, RPC errors and signer failures never produce a veto or a refund.
6. No local state is needed after a restart. Recovery comes from votes and terminal states on chain.

**Guarantee added in Go.** No vote, veto or refund executes twice, as defined in section 6.1.

**Intentional differences from TS.**

1. Per-transfer events are not written in strict priority order. Section 9.1 lists the ordering that is kept.
2. Freshness and identity checks run per snapshot instead of per transfer: at most once a second for Akash, once per chunk for Solana. Audit entries use their chunk's snapshot for both the early exit and the observation.
3. Akash state is read through gRPC instead of `abci_query`.
4. Messages are batched into unordered transactions and confirmed asynchronously. In `start`, outcome events may follow the summary. A message whose slot is taken in its window waits for the next window (section 6.5).
5. AKT events other than `TransferResolved` are not decoded. TS rejected a transaction carrying a malformed one.
6. Mint `COption` tags other than 0 and 1 are rejected. TS treated any non-zero tag as present.
7. TS assertions that had no message get descriptive Go messages. CheckTx rejection, expiry, an unconfirmed transaction and a message `scan` could not send get their own error texts (section 6.5).
8. The audit page is min(`scan.audit-per-scan`, `scan.page-size`). TS used min(`auditPerScan`, 500), which is identical at the default page size.
9. After each confirmed refund finalization, the new refund head is reviewed at once (section 6.6). TS reached a later head in the same pass only when it was listed after the refund just paid.

### 5.7 Shutdown

On the first SIGINT or SIGTERM:

1. No new chunk starts. In-flight requests finish or time out. A review's Akash read still waiting for a concurrency permit is not in flight, and is cancelled.
2. No new transaction is signed. Queued and deferred messages are released without an event and planned again on the next start; TS also skipped submissions after a signal.
3. In-flight transactions get up to 30 s to confirm.
4. The summary of the partial pass is written and events are flushed. As in TS, `scanned` is the number of open transfers listed, even when reviews stopped early.

`start` then exits 0, and `scan` exits 1 if the pass had failures. A second signal exits immediately.

## 6. Broadcasting

### 6.1 Nonce slots

The chain refuses a second transaction with the same signer and `timeout_timestamp` (section 2.2). The reporter derives every timeout from the messages a transaction carries, so all transactions that could carry the same message in the same window share one replay key, and the chain executes at most one of them. Slots need no persisted state and never read the host clock.

**Constants (nonce scheme v1).** These are protocol values, not configuration.

| Constant                      | Value                        |
| ----------------------------- | ---------------------------- |
| Window length W               | 2 min of chain time          |
| Vote slots                    | 128, numbered 0 to 127       |
| Veto and refund slots         | 1,024, numbered 128 to 1,151 |
| Spacing between slot timeouts | 1 ms                         |

- **Message key:** the transfer ID and message kind, plus the outcome, round and policy version for a vote. A veto's key has no round, on purpose. `MsgVetoVoid` carries none and the keeper checks none (`x/sink/keeper/settlement.go:204-240`), so a veto planned in one round can execute after a governance resolution starts the next. A round-less key puts it in the same slot as any veto planned in that next round.
- **Execution:** inclusion with code 0. A transaction that fails delivery pays its fee, but its messages change nothing.
- **Window of a planned message:** w = ⌊t / W⌋, where t is the block time, in Unix nanoseconds, of the snapshot the message was planned from.
- **Slot of a message:** h is the first 8 bytes, big-endian, of SHA-256 over:
  - the ASCII label `akt-reporter/nonce/v1`;
  - one kind byte: 1 for `MsgReportOutcome`, 2 for `MsgVetoVoid`, 3 for `MsgFinalizeRefund`;
  - the transfer key: one owner-length byte, the owner bytes, then the height and sequence as 8-byte big-endian integers;
  - for a vote, the outcome as a 4-byte big-endian integer, then the round and policy version as 8-byte big-endian integers.

  A vote's slot is h mod 128. A veto's or refund's slot is 128 + (h mod 1,024).
- **Timeout:** T(w, s) = (w + 1) · W − (s + 1) ms. The 1,152 timeouts of a window are distinct and fall in its last 1.152 s. T(w, s) is at most W after the planning snapshot's block time.
- **One slot per transaction:** a transaction carries messages of a single window and slot, and its `timeout_timestamp` is that slot's T(w, s).
- **Send cutoff:** a slot's transaction is signed only while T(w, s) minus the latest block time seen is at least 20 s plus the signer timeout (`signer.remote.timeout`, or 0 for the keyring). The cutoff keeps transactions from expiring unsent; safety does not depend on it.
- **Changing the scheme.** A later version that changes the window, the slot counts, the spacing or the hash input must ship an upgrade note: stop every old process and wait four minutes before starting the new one. That is one window for the old timeouts to pass, plus the age of the oldest snapshot the new version may accept (`max-age` and clock skew, section 6.3), so its first plans see every old execution. Test vectors pin the constants (section 11.1).

**Why no message executes twice.** Take two transactions X and Y that carry the same message key k, planned in windows w_X ≤ w_Y.

1. **Same window.** X and Y have the same signer and timeout, hence the same replay key. The first to pass the ante handler stores the key, and the other fails it, with no effect and no fee. The key stays until a block's time reaches the timeout, and every later block refuses both transactions as expired. The block whose time equals the timeout is the one exception (residual risk, below).
2. **Different windows.** If X executes, its block's time is at most T(w_X, s), which is before w_Y starts, and so before the block time of Y's planning snapshot. Block times strictly increase, so that snapshot's height is above X's block, and its state includes X's effect. By the policy lemma below, no such state in the same settlement round plans k. Y can therefore carry k only after a governance resolution has started a new round, and only a veto's key cannot tell those rounds apart.
3. **Lower snapshots** need no height floor. In the same window, rule 1 applies. A plan from an earlier window carries a timeout the chain has already passed, so it can never execute. The broadcaster defers it once it has seen a later block; a process that has seen only a lagging node may send it, but no future block can include it.

So restarts, lost responses, separate `scan` runs, lagging or load-balanced nodes, a missing transaction index and processes sharing a key cannot make a message execute twice:

- a vote executes at most once per (transfer, outcome, round, policy version);
- a veto executes at most once per transfer and settlement round;
- a refund executes at most once per transfer.

A governance resolution starts a new round (`settlement.go:339-347`). A veto after it is a new decision, which TS also makes. The one exception is the SDK boundary under "Residual risk".

**Policy lemma.** Once k has executed, no later state in the same settlement round plans k; for a vote, the policy version must also be the same.

An execution can leave no trace: `MsgReportOutcome` returns without effect under a conflict hold, or when the transfer is terminal, refund-pending or held (`settlement.go:110-112`, `131-133`). The lemma holds anyway, because within one round and policy version the state only moves forward:

- statuses only advance;
- a hold is cleared only by a governance resolution, which starts a new round;
- votes only accumulate.

So, per message kind:

- **Vote.** `PlanOutcome` plans one only with `HOLD_NONE` and an active status, or on a contradiction, and only when not `alreadyReported`. It also plans one for `refresh` (`policy.ts:203-220`). Each way is closed after the vote executes:
  - a recorded vote sets `alreadyReported`;
  - a vote without effect met a state that stays inactive and uncontradicted;
  - a contradicting vote sets `HOLD_CONFLICT`;
  - `refresh` needs a stale policy version, and the first vote under the current version replaces it (`settlement.go:114-120`).
- **Veto.** It needs `STATUS_REFUND_PENDING`, `HOLD_NONE` and an open challenge window (`policy.ts:172-185`). An executed veto sets `HOLD_VETO` (`settlement.go:232-234`).
- **Refund.** It needs `STATUS_REFUND_PENDING`. An executed refund sets `STATUS_REFUNDED`, which is terminal (`settlement.go:475`).

Tests pin the lemma for every message kind, including the executions without effect (section 11.1).

**Residual risk.** The SDK removes a replay key at the start of the block whose time equals its timeout, and that block still accepts the timeout (section 2.2).

- **What can happen:** a second execution in a block whose time equals the timeout to the nanosecond, if that block also includes another transaction with the same replay key.
- **Why it needs an unusual proposer:** honest proposers draw blocks from mempools that recheck has cleared of such copies, CometBFT's default. Akash's proposal handlers do not filter block contents (`app/app.go:287-292`). So it takes a proposer that skips recheck or builds its own blocks.
- **Status:** upstream cosmos-sdk rejects this case since cosmos/cosmos-sdk#26672. Until the Akash fork adopts that change, this is the one exception to the guarantee (section 14).

**TS transactions** are ordered and expire 20 blocks after they are built, outside this scheme. Section 12.1 gives the migration step.

### 6.2 Batching and sending

- `MsgReportOutcome` messages of one window and slot are batched, up to `tx.max-msgs` per transaction.
- `MsgVetoVoid` and `MsgFinalizeRefund` get one transaction each, in their own slot range. They can fail on the challenge window, refund order or caps, and one failed message fails its whole transaction.
- A vote batch is sent when it is full, when the review that filled it ends, or 10 s before its send cutoff. Vetoes and refunds are sent at once.
- A slot carries at most one live transaction per window. A transaction refused on its first send never held the replay key, so its slot can be used again in that window.
- Before signing, the latest snapshot must still list `reporter` (otherwise `reporter was removed`), the signer's address must equal `reporter`, and the send cutoff must not have passed.
- At most `tx.max-in-flight` transactions are unconfirmed at once. When one resolves, the broadcaster sends `MsgVetoVoid` first, then `MsgFinalizeRefund`, then vote batches in plan order. Without a transaction index, a sent transaction leaves the in-flight count but keeps its slot until chain time passes its timeout.
- Reviews never wait for broadcasting, so a slow or unavailable signer delays submissions, not passes. The slot table needs no separate bound: it admits each message key at most once per window (section 6.5), so it never exceeds the open set. `scan` sends everything it planned and waits for the outcomes.
- With the defaults, at most 200 votes are unconfirmed at once (`tx.max-in-flight` × `tx.max-msgs`). A window carries at most 128 vote transactions, which is also about what 8 in flight over the roughly 16 blocks before the cutoff can confirm. A 5,000-vote backlog drains in three to four windows on a fresh node.

### 6.3 Unordered transactions

- **Body:**
  - the messages and the memo `AKT sink reporter`;
  - `unordered = true`;
  - `timeout_timestamp` = T(w, s) of the transaction's slot (section 6.1);
  - no `timeout_height`.
- **Auth info:** one signer, `SIGN_MODE_DIRECT`, sequence 0, the fee and the gas limit. The account number is read once at startup. If the account does not exist, because the address never received funds, broadcast mode refuses to start with `reporter account not found`.
- **Timeout bounds.** T(w, s) is at most 2 min after the planning snapshot's block time, well inside the chain's 10 min limit. In broadcast mode, validation requires `max-age` + the signer timeout ≤ 80 s, so a message planned early in a window from the oldest accepted snapshot can still be sent before its cutoff. The budget:

- 120 s, the window;
- minus 1.152 s, the earliest slot timeout's distance from the window's end;
- minus 10 s, the clock skew the freshness check allows;
- minus 20 s, the cutoff base;
- leaves 88.8 s. The 8.8 s above 80 s covers review and signing.

Older snapshots only defer messages; they never affect safety.
- **No fallback.** If the chain stops accepting unordered transactions, every broadcast is rejected with `unordered transactions are not enabled`, reported per message. There is no ordered fallback (D6).

### 6.4 Simulation, gas and fees

- Every batch is simulated through gRPC `Simulate` before signing. Simulation skips the replay-key check (section 2.2), so it cannot tell whether a slot is taken.
- With `gas = auto`, the gas limit is ceil(gas used × `gas-adjustment`). A numeric `gas` is per message, so the limit is `gas` × the number of messages.
- Simulation runs against the latest committed state, so the limit must also cover other reporters' votes landing first in the same block, including the vote that completes the quorum; the `gas-adjustment` default of 2.0 covers that with a 10% margin for policies of up to 5 members whose votes all land in one block.
- The simulated transaction has the signed one's gas-relevant shape: a fee, a gas limit and a full-length placeholder signature. The fee and limit are the final ones when they are known before simulating (`fees`, or a numeric `gas`). Otherwise the fee is one unit of each `gas-prices` denomination when `fees` is empty, and the declared limit is 0. Simulation meters no limit; the declared one only meets the ante handler's block max gas check, which runs in simulation too and which 0 always passes, and adds a few bytes of size gas, which `gas-adjustment` covers.
- If simulation fails at `message index: N`, message N is dropped with `{id, error}` and the batch is simulated again, for up to three rounds. Messages still left after that are deferred (section 6.5), because a slot carries at most one live transaction per window.
- A simulation failure without a message index, such as an invalid timeout, fails the whole batch before submission. Each message gets `{id, error}`, and its entry is released.
- With `gas-prices`, simulation carries only a one-unit fee, so an account that cannot pay the real fee is refused at CheckTx, reported per message, and its entry released. With `fees`, simulation fails on it as above.
- The fee is `fees` when that is set, otherwise ceil(gas limit × `gas-prices`).
- Caps: the gas limit must be at most the gas cap and the fee at most `tx.max-fee`. The gas cap is `tx.max-gas`, or the consensus block max gas when that is positive and lower, since no block includes a transaction declaring more; broadcast mode reads it at startup through gRPC `cosmos.consensus.v1.Query/Params` and logs the effective cap. A batch over a cap is halved, and the second half is deferred. A single message over a cap gets `{id, error}` and is not sent. The signer's sign-bytes limit, when it declares one, is a cap like `tx.max-gas`: batches are sized to fit it, a batch over it is halved and its second half deferred, and a single message over it gets `{id, error}`.

### 6.5 Confirmation and the slot table

- **Slot table.** Only the broadcaster's owner goroutine reads or writes it, and only that goroutine writes outcome events. It does no I/O: workers simulate, sign, broadcast and look up transactions, then report back.
  - Entries are keyed by window and slot. Each holds the queued messages, then the transaction built from them.
  - A planned message whose key is already queued or sent in its slot is skipped.
  - A planned message is **deferred** when its slot already has another transaction in that window (sent, or refused because its replay key is taken), when its batch is full, or when its send cutoff has passed. A plan from a window earlier than the latest block seen counts as past its cutoff. A deferral is not an outcome and writes no event.
  - A queued message is released on any failure before its first broadcast, and each such failure writes `{id, error}`: simulation rejection, signing failure, membership or signer-identity failure, or a cap rejection. On shutdown it is released without an event (section 5.7). A later review can plan it again.
  - A window's entries leave the table once a block's time passes the window's end and their final lookups have run.
- **Broadcast** uses `broadcast_tx_sync`. After a transport error, the same signed bytes are re-sent until the timeout passes.
- **Identical copies.** `tx already exists in cache` arrives as a JSON-RPC error, not a CheckTx code (`rpc/core/mempool.go:35-42` in the CometBFT fork). On any send, first or not, it means identical bytes are already pending or were included. Another process, or a restarted one, can build identical bytes, because signing is deterministic. The transaction is marked sent and confirmed by its hash.
- **Re-sends.** For a transaction already sent once, `failed to add unordered nonce` also means an earlier copy may be pending or included. The transaction stays sent.
- **Slot taken.** `failed to add unordered nonce` on a first send means another transaction holds the slot's replay key: an earlier attempt by this process or another one. The slot is closed for the window, and its messages are deferred.
- **Rejection.** Any other CheckTx rejection of a first send is final. Each message gets `{id, error: "sink transaction rejected: <HASH> code <n>: <log>"}`, and the slot stays usable in that window. A rejection of a re-send is not final; the transaction stays sent.
- **Confirmation.** While any transaction is unconfirmed, the broadcaster refreshes the height tracker once a second, including between passes. On every new height, and at the next refresh after a failed lookup, each unconfirmed transaction is looked up with `tx(hash)`, or with the batch under Expiry once chain time has passed its timeout:
  - code 0 gives `{id, transactionHash}` for each message, with the hash in uppercase hex as cosmjs printed it;
  - a replay-key failure (codespace `sdk`, code 18, a log containing `failed to add unordered nonce`) means another transaction took the slot first, and its messages are deferred;
  - any other code gives `{id, error: "sink transaction failed: <HASH> code <n>"}`, the TS text.
- **Expiry** is decided by chain time and the node's index. A node indexes a block only after committing it, so a missing transaction proves nothing until its index has caught up. Once a committed block's time passes the timeout, each lookup is one JSON-RPC batch, which one node answers when the endpoint passes it on whole (a node, or a plain HTTP proxy): `block_search` for the height at which the broadcaster first saw chain time past the timeout, then `tx_search` for the hash. CometBFT indexes a block's transactions before the next block's events, so with that height in the block index, every block that could include the transaction is indexed. A transaction still missing then can never be included, and each message gets `{id, error: "sink transaction expired: <HASH>"}`. Until then, a missing transaction stays unresolved and is looked up again at the next refresh. Two cases remain: a gateway that splits a batch between nodes can still answer from a lagging one, and an indexer that failed to write a block, which CometBFT logs as `failed to index block txs` and does not retry, can make an included transaction look missing.
- **Deferral.** Deferred transfers are reviewed again once a block from a later window is seen, through the follow-up reviews of section 6.6. If a deferred message executed in the meantime, through another process's transaction or an earlier attempt, that review plans nothing.
- **Waiting limit.** `scan` waits until no transaction is unresolved, nothing is deferred and follow-up reviews are idle, or until 3 min of wall-clock time pass without progress: a submission, a resolved outcome or a new window.
  - A transaction still unresolved then, for example because the chain halted, the node's index stopped, or the endpoint refuses JSON-RPC batches, gives `{id, error: "sink transaction unconfirmed: <HASH>"}`. It is not called expired.
  - A message still deferred gives `{id, error: "sink message not sent: deferred (<reason>) until <time>"}`, with its deferral reason (section 9.3) and its slot's timeout in RFC 3339.
  - A message still queued, or in a transaction still being built, gives `{id, error: "sink message not sent: queued until <time>"}`, where `<time>` is its slot's timeout in RFC 3339. If that build sends its transaction afterwards, the transaction keeps its slot, and no further event is written for its messages.
  - Both count as failures. `start` keeps both: a sent transaction stays until inclusion or chain-time expiry, re-sending only its existing bytes, and a deferred message is reviewed again in the next window.
- **No transaction index.** If `/status` reports `node_info.other.tx_index` as anything other than `on`, the reporter logs a warning once, because outcomes cannot be observed.
  - CheckTx rejections, including a taken slot, are still reported or deferred as above.
  - Accepted transactions get no outcome event, do not count as failures, and leave the table at chain-time expiry. `scan` does not wait for them.
  - Reviews in the next window reflect their effect. Safety does not depend on the index.
- **Failure counting.** `scan` counts every outcome before its summary, so its exit code includes them. `start` counts an outcome in whichever pass is running when it resolves.
- **Deferred vetoes** wait up to one window. That is negligible against the 72 h minimum challenge period. On networks built with a shorter period (the `sinkdev` tag allows 1 s), a deferred veto can miss its challenge window.
- **A lagging node** makes the reporter plan from old snapshots, and the send cutoff is measured against what that node has seen. The chain may then refuse the resulting transactions as expired, which shows up as expiry events and `cutoff` deferrals. The remedy is a healthy node or a lower `max-age`.
- **Restart.** The table is lost, and nothing needs to be persisted. A new process that plans a message the old one had in flight uses the same slot in the same window, so the chain executes at most one of the two (section 6.1).

### 6.6 Follow-up reviews

Between passes, the broadcaster asks the scanner to review specific transfers in two cases.

**Refund-head progression.** `x/sink` pays refunds strictly in queue order. `MsgFinalizeRefund` succeeds only for the current refund head, which advances only when that refund executes (`x/sink/keeper/settlement.go:430-437`). The transfers of a chunk share one snapshot, so on its own a pass finalizes at most the head it saw. The broadcaster therefore keeps the queue moving:

1. When a `MsgFinalizeRefund` is included with code 0, the broadcaster takes a fresh snapshot at once.
2. It reviews the new refund head as a one-transfer chunk: re-read at H′, Solana snapshot, plan.
3. If that review plans another finalization, it is sent the same way, and the steps repeat.

Progression stops when the head plans no finalization (not yet eligible, held, or the epoch capacity is used), or on shutdown. Without a transaction index no lookup confirms a refund, so refund-head progression does not run and only passes advance the head. Refunds are paid at about one per block; a refund whose slot is taken in the current window waits for the next one.

**Deferred transfers.** Once a block from a later window is seen, the deferred transfers are reviewed in chunks of up to 96, each with a fresh snapshot (section 6.5).

Each follow-up review needs a snapshot at least as high as the block that triggered it: the inclusion, or the first block seen in the new window. A lower snapshot from a load-balanced node is retried at the tracker's refresh rate until the waiting limit or shutdown, so progression cannot stop on a stale refund head.

Follow-up reviews run one at a time, never on the owner goroutine, and write the normal review events. `scan` waits for them before its summary. The slot table keeps a pass and a follow-up review from sending the same message twice in one window.

## 7. Signing

Broadcast needs exactly one signer: the keyring (`from`) or the remote signer (`signer.remote.address`). Preview never opens the keyring or connects to the signer, even when either is configured.

### 7.1 Keyring, in-process

- Settings: `from` (key name or address), `keyring-backend` (default `test`) and `keyring-dir` (default: the home directory). `keys` reads these settings from the same configuration as `start` (flags, environment, file), so both use one keyring. The cosmos-sdk default of `os` does not apply.
- The key must be secp256k1 and must derive `reporter`; otherwise `signer does not match reporter`.
- Backends:
  - `test`: files in the home, mode 0600, owned by the service user (UID 10001 in the image). This is the container default.
  - `file`: encrypted at rest. The passphrase is read from stdin once at startup, so this backend suits interactive runs or systemd with `StandardInput=file:`. Containers use `test` or the remote signer.
  - `os`: the Linux secret service. It needs a D-Bus session, which containers and system services usually lack, and it is unavailable in darwin builds, which have no CGO.
- `akt-reporter keys` exposes the cosmos-sdk key commands, including `import-hex`. Without the hex argument, `import-hex` reads the key from stdin, which keeps it out of shell history.

### 7.2 Cosmos-KMS SignerService, remote

Cosmos-KMS (`github.com/cosmos/kms`) is the documented successor to tmkms. tmkms speaks privval, which cannot sign transactions; Cosmos-KMS also offers a gRPC `SignerService` that can.

- **Protocol:** `cosmos.kms.signerservice.SignerService`, calls `GetKey` and `Sign`. The proto is copied at a pinned commit, recorded in the file header, and the generated code is committed.
- **Startup:** `GetKey(signer.remote.key-id)` must return scheme `ECDSA_SECP256K1` or `ECDSA_SECP256K1ETH`, and a 33-byte compressed key on the curve that derives `reporter`.
- **Per transaction:**
  1. The payload is the sign bytes for `ECDSA_SECP256K1`, or their SHA-256 for `ECDSA_SECP256K1ETH`.
  2. The response may be a 64-byte r‖s or a 65-byte r‖s‖v; v is dropped.
  3. The signature is normalized to low-S and verified over SHA-256(sign bytes) with the key from startup.
  4. On any failure nothing is broadcast, and each message gets `{id, error}`.
  5. For `ECDSA_SECP256K1`, Cosmos-KMS signs through AWS KMS raw messages, which are limited to 4096 bytes, so the remote signer declares that limit and refuses longer sign bytes; `ECDSA_SECP256K1ETH` sends a 32-byte digest and has no limit.
- **Transport:** TLS only; there is no plaintext option.
  - `signer.remote.ca-file` sets the trusted CA; system roots are used when it is empty.
  - `signer.remote.server-name` overrides the verified server name.
  - An optional client certificate (`cert-file`, `key-file`) is for an mTLS proxy in front of the KMS.
  - `signer.remote.timeout` applies per call.
- **Access.** The `SignerService` authenticates no caller. Operators must restrict network access so that only the reporter can reach it.
- **Release gate.** A release enables the remote signer only after a CI job against a real `kms` binary passes for each supported scheme. The job sets `REPORTER_KMS_REQUIRED=1`, so missing gate configuration fails the test instead of skipping it, and it requires the `PASS` line of the test and of each scheme's subtest in the verbose output, so a renamed test cannot pass with "no tests to run".

## 8. CLI and configuration

### 8.1 Mechanics

This follows `cmd/akash` and chain-sdk's `InterceptConfigsPreRunHandler`. It is re-implemented because that package needs CGO.

- **Home and file.** `--home`, also `AKT_REPORTER_HOME`, defaults to `$HOME/.akt-reporter`. `<home>/config/reporter.toml` is read when present.
- **Flags and environment.** Every key is also a flag and an environment variable.
  - Flags use the key name, for example `--solana.rpc`, as cosmos-sdk does with `--grpc.address`.
  - Environment variables add the prefix `AKT_REPORTER_` and map `.` and `-` to `_`, for example `AKT_REPORTER_SOLANA_RPC`.
- **Precedence:** flag, then environment, then file, then default. This uses viper plus a copy of chain-sdk's `bindFlags`.
- **Unknown keys** in the file are errors. Unknown `AKT_REPORTER_*` variables are logged as warnings and ignored, because platforms add their own. For example, Kubernetes injects `AKT_REPORTER_SERVICE_HOST` and similar variables when a Service is named `akt-reporter`.
- **Logging flags** match akash: `--log_level`, `--log_format plain|json`, `--log_color` and `--log_timestamp`. Logs go to stderr; stdout carries only events.
- **Writes.** Only `init` and `keys` write files.

### 8.2 Commands

| Command   | Behavior                                                                                                                                                                       | TS equivalent                 |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------- |
| `init`    | Writes `<home>/config/reporter.toml` rendered from the effective configuration (defaults plus any flags and environment). Refuses to overwrite without `--overwrite`. With `--overwrite`, renames a synced temporary file over the target, so a failed write keeps the old file. Keeps the existing file's mode, owner and group where it may, and a new file is 0600; without the privilege to keep the owner, the file belongs to the user who runs `init` and keeps its group where that user may set it. On Linux the replacement also takes the existing file's POSIX access ACL, or none when it had none, and `init` fails without replacing the file when that ACL cannot be copied. No other ACL is kept: not an NFSv4 ACL on Linux, and none on other platforms, where an existing file's ACL entries, deny entries included, are dropped and the replacement may inherit entries from its directory. Through a symlink, replaces the link's target, and refuses a target that is not a regular file. Where no temporary file can be created or renamed into place (a single-file bind mount), writes in place, and there a failed write can leave a partial file. Through a dangling symlink, creates the link's target in place. | None                          |
| `start`   | Runs a pass, waits `scan.interval`, and repeats. This is the image's default command.                                                                                          | `run --watch`                 |
| `scan`    | Runs one pass, then waits for its transactions and follow-up reviews (section 6.5). Exits 1 if anything failed.                                                                | `run`                         |
| `check`   | Validates offline, including the network guard and signer selection, with no RPC, keyring or signer access. Prints `Configuration valid. No RPC request or transaction sent.` | `check`                       |
| `keys`    | The cosmos-sdk keyring commands: `add`, `import`, `import-hex`, `export`, `list`, `show`, `delete` and the rest.                                                               | `keys create`, `keys address` |
| `version` | Prints the service name, version, commit, platform and Go version.                                                                                                             | `--version`                   |

`start` and `scan` first run the `check` validation. They then connect to both Akash endpoints and run the node-ID check, plus the transaction-index check in broadcast mode. In broadcast mode they load the signer, check its address, read the account number and read the consensus block max gas (section 6.4). Finally they bind the metrics listener, if configured; a bind failure ends the command before any pass.

### 8.3 Keys

| Key                                                 | Default                         | Rules                                                          | TS field                         |
| --------------------------------------------------- | ------------------------------- | -------------------------------------------------------------- | -------------------------------- |
| `network`                                           | `mainnet`                       | `mainnet` or `testnet` (section 8.4)                           | `--network`                      |
| `broadcast`                                         | `false`                         | `false` means preview                                          | `--broadcast`                    |
| `reporter`                                          | required                        | Canonical lowercase `akash1…` address of 20 bytes              | `reporter`                       |
| `chain-id`                                          | required                        |                                                                | `akashChainId`                   |
| `node`                                              | `tcp://localhost:26657`         | `tcp`, `http` or `https`; no credentials in the URL            | `akashRpc`                       |
| `grpc-addr`                                         | `localhost:9090`                | `host:port`                                                    | none                             |
| `grpc-insecure`                                     | `false`                         | Plaintext gRPC for a non-loopback address; loopback addresses always use plaintext | none           |
| `max-age`                                           | `60s`                           | Above 0, at most `10m`; when broadcasting, `max-age` + signer timeout at most 80 s (section 6.3) | `maxAgeSeconds` |
| `request-timeout`                                   | `15s`                           | `1s` to `60s`                                                  | none                             |
| `from`                                              | empty                           | Keyring key name or address                                    | `keyFile`                        |
| `keyring-backend`                                   | `test`                          | `os`, `file` or `test`; also used by `keys`                    | none                             |
| `keyring-dir`                                       | the home directory              |                                                                | none                             |
| `gas`                                               | `auto`                          | `auto`, or gas per message; the transaction limit is that times the message count, at most `tx.max-gas` | `gasLimit` |
| `gas-adjustment`                                    | `2.0`                           | `1.0` to `10.0`                                                | none                             |
| `gas-prices`                                        | `0.025uakt`                     | `uakt` only                                                    | none                             |
| `fees`                                              | empty                           | `uakt` only, at most `tx.max-fee`; overrides `gas-prices`      | `feeAmountUakt`                  |
| `solana.rpc`                                        | `http://localhost:8899`         | `http` or `https`; no credentials in the URL                   | `solanaRpc`                      |
| `solana.genesis-hash`                               | required                        | Canonical base58 of 32 bytes                                   | `solanaGenesisHash`              |
| `solana.program-id`                                 | required                        | Canonical base58                                               | `programId`                      |
| `solana.mint`                                       | required                        | Must equal `PDA("mint")` of the program                        | `mint`                           |
| `solana.light-client-program`                       | empty, meaning follow           | Canonical base58                                               | `lightClientProgram`             |
| `solana.max-slot-lead`                              | `0`, meaning derive             | At most 100,000                                                | `maxSlotLead`                    |
| `scan.interval`                                     | `15s`                           | `1s` to `60s`                                                  | `pollSeconds`                    |
| `scan.audit-per-scan`                               | `100`                           | 1 to 10,000                                                    | `auditPerScan`                   |
| `scan.page-size`                                    | `500`                           | 1 to 5,000                                                     | none                             |
| `scan.workers`                                      | `16`                            | 1 to 256                                                       | none                             |
| `scan.akash-concurrency`                            | `32`                            | 1 to 256                                                       | none                             |
| `scan.solana-concurrency`                           | `8`                             | 1 to 64                                                        | none                             |
| `scan.block-concurrency`                            | `2`                             | 1 to 16                                                        | none                             |
| `tx.max-gas`                                        | `20000000`                      | 1 to 20,000,000; a lower positive block max gas caps it further (section 6.4) | Bound on `gasLimit` |
| `tx.max-fee`                                        | `1000000uakt`                   | `uakt`, above 0, at most `1000000uakt`                         | Bound on `feeAmountUakt`         |
| `tx.max-msgs`                                       | `25`                            | 1 to 100                                                       | none                             |
| `tx.max-in-flight`                                  | `8`                             | 1 to 64                                                        | none                             |
| `signer.remote.address`                             | empty                           | `host:port`                                                    | `externalSigner`                 |
| `signer.remote.key-id`                              | empty                           | Required with `address`                                        | none                             |
| `signer.remote.ca-file`                             | empty, meaning system roots     | Path                                                           | none                             |
| `signer.remote.server-name`                         | empty                           |                                                                | none                             |
| `signer.remote.cert-file`, `signer.remote.key-file` | empty                           | Both or neither                                                | none                             |
| `signer.remote.timeout`                             | `10s`                           | `1s` to `120s`                                                 | `externalSigner.timeoutSeconds`  |
| `metrics.listen`                                    | empty, meaning disabled         | `host:port`                                                    | none                             |

### 8.4 Validation

`check`, `start` and `scan` apply every rule in section 8.3, plus the following:

- `solana.mint` must equal `PDA("mint")` derived from `solana.program-id`.
- In broadcast mode, exactly one of `from` and `signer.remote.address` must be set.
- The network guard:
  - **`mainnet`** requires `chain-id` `akashnet-2` and genesis hash `5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d`, and refuses the development program ID `FfFAPUKqjf68ggj1HvpyzMHFDRnT2cFAdZYYQb7mK9i5`.
  - **`testnet`** refuses both mainnet identities.

`init` writes the file even when it breaks a rule, because a fresh file has its required keys empty, and prints every rule the file does not yet satisfy.

## 9. Output

### 9.1 Events on stdout

- **Schema.** Events use the TS JSON-lines schema in Appendix A, unchanged:
  - the same field names and alert strings;
  - 64-bit integers and amounts as decimal strings;
  - bytes as base64;
  - enums as numbers;
  - message fields in the TS camelCase names;
  - transaction hashes in uppercase hex.
- **Writing.** One writer goroutine writes whole lines.
- **Ordering that is kept:**
  - For each transfer, its review event or error comes before its transaction events.
  - A `light-client-changed` alert comes before every event decided under the new client.
  - The summary is the pass's last event. The exceptions are transaction outcome events and follow-up reviews in `start` (sections 6.5 and 6.6).
  - Events of different transfers have no defined order.

### 9.2 Logs on stderr

Akash-style zerolog. Logs cover:

- a startup summary of the effective configuration, without key material and with RPC endpoints shown as `scheme://host`;
- retries;
- per-pass cache and RPC statistics;
- signer and node identity.

### 9.3 Metrics

When `metrics.listen` is set, `/metrics` serves:

| Metric                                         | Type      | Labels             |
| ---------------------------------------------- | --------- | ------------------ |
| `akt_reporter_pass_duration_seconds`           | histogram |                    |
| `akt_reporter_pass_transfers`                  | gauge     |                    |
| `akt_reporter_reviews_total`                   | counter   | `result`           |
| `akt_reporter_alerts_total`                    | counter   | `alert`            |
| `akt_reporter_rpc_duration_seconds`            | histogram | `chain`, `method`  |
| `akt_reporter_rpc_errors_total`                | counter   | `chain`, `method`  |
| `akt_reporter_cache_requests_total`            | counter   | `cache`, `result`  |
| `akt_reporter_tx_total`                        | counter   | `result`           |
| `akt_reporter_tx_messages_total`               | counter   | `type`             |
| `akt_reporter_tx_fees_uakt_total`              | counter   |                    |
| `akt_reporter_tx_in_flight`                    | gauge     |                    |
| `akt_reporter_deferred_messages_total`         | counter   | `reason`           |

`result` is `ok` or `error` for reviews, `hit` or `miss` for caches, and `included`, `failed`, `rejected`, `slot_taken`, `expired` or `unconfirmed` for transactions. `chain` is `akash` or `solana`. `reason` is `slot_busy`, `slot_full`, `cutoff` (including plans from an earlier window), `simulation` or `cap`.

## 10. Build, image and release

### 10.1 GoReleaser

`.goreleaser-reporter.yaml`, kept separate from the node configurations:

- **Project:** `akt-reporter`, dist `.cache/goreleaser/reporter`.
- **Build:** one build of `./cmd/akt-reporter`.
  - Environment: `CGO_ENABLED=0` and `GOWORK=off`.
  - Targets: linux and darwin, amd64 and arm64.
  - Flags: `-trimpath -mod=readonly`.
  - ldflags: `-s -w`, plus the cosmos-sdk `version.Name`, `version.AppName`, `version.Version` and `version.Commit`.
- **Archives:** `akt-reporter_{{ .Version }}_{{ .Os }}_{{ .Arch }}.tar.gz`, containing the binary, `LICENSE` and `reporter.example.toml`. Checksums go in `akt-reporter_{{ .Version }}_checksums.txt`.
- **Images (`dockers_v2`):**
  - image `{{ envOrDefault "REPORTER_IMAGE" "ghcr.io/akash-network/akt-reporter" }}`;
  - tags `{{ .Version }}`, plus `latest` for releases without a prerelease suffix;
  - platforms `linux/amd64` and `linux/arm64`;
  - OCI labels and annotations for title, description, source, revision, version and creation time.
- **GitHub release:** disabled (`release.disable`, `changelog.disable`). GoReleaser OSS does not understand tag prefixes, so the workflow publishes the `reporter/vX` release itself.

### 10.2 Dockerfile

`_build/akt-reporter.Dockerfile`:

```dockerfile
FROM gcr.io/distroless/static-debian12:nonroot
ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/akt-reporter /usr/bin/akt-reporter
ENV AKT_REPORTER_HOME=/home/akt-reporter
USER 10001:10001
WORKDIR /home/akt-reporter
ENTRYPOINT ["/usr/bin/akt-reporter"]
CMD ["start"]
```

- `docker run <image>` starts the reporter. Any arguments replace the default command.
- The image's home is not writable, so commands that write need the home mounted, owned by UID 10001. For example: `docker run --rm -i -v <home>:/home/akt-reporter <image> keys import-hex reporter < reporter.key`.
- The Dockerfile has no `RUN` step, so the arm64 image builds without emulation.

### 10.3 Make

`make/reporter.mk`, included from `Makefile`, provides:

- `akt-reporter`: a local CGO-free build;
- `test-reporter`;
- `reporter-snapshot`: a GoReleaser snapshot with archives and local per-architecture images;
- `reporter-goldens`: regenerates the parity goldens (section 11.2).

None of them depend on wasmvm or the contracts build.

### 10.4 Release workflow

The new `.github/workflows/release-reporter.yaml` runs on `reporter/v*` tags:

1. Check the tag identity as today: the tag format, the tagged commit being on `main`, and no published release for the tag.
2. Run `go test` for the reporter packages with `GOWORK=off`.
3. Build a GoReleaser snapshot. Smoke-test the amd64 image with `--network none --read-only`: run `version`, and `check` against the example configuration supplied through the environment.
4. Log in to GHCR. `GORELEASER_CURRENT_TAG=v$VERSION goreleaser release --clean --skip=validate -f .goreleaser-reporter.yaml` builds the archives and pushes the multi-arch image.
5. With `gh`:
   1. create a draft release;
   2. upload the archives, checksums, and the compose, `.env` and configuration examples;
   3. check that the image can be pulled anonymously;
   4. publish with `--latest=false`, and with `--prerelease` for tags with a prerelease suffix.

The same change narrows `release-operators.yaml` to `relayer/v*`, so one tag never starts both workflows.

### 10.5 Pull request CI

`tests.yaml` gains a reporter job that runs:

- `go vet`;
- `go test -race`, with CGO enabled because the race detector needs it;
- `CGO_ENABLED=0 GOWORK=off go build ./cmd/akt-reporter`, which proves the CGO-free build;
- `goreleaser check -f .goreleaser-reporter.yaml`.

## 11. Testing

### 11.1 Unit tests

The Go unit tests port the TS suites.

- `policy`: every case in `policy.test.ts`, plus the lemma of section 6.1. For each message kind, no state after its execution in the same round plans it again. That includes executions without effect: a conflict hold, and a terminal, refund-pending or held transfer. A governance resolution then starts a round in which a veto may be planned again.
- `tx` slots: pinned vectors (fixed message keys map to fixed slots and timeouts), the 1,152 timeouts of a window distinct and within its last 1.152 s, and the send cutoff.
- `solana`:
  - program-log attribution: CPI frames, truncation, unbalanced logs, failed frames;
  - account, event and mint decoders, using vectors from the Rust tests and `programs/akt/tests/fixtures`;
  - PDAs, checked against TS-generated vectors;
  - writability for legacy, v0 and v1 messages;
  - the streaming block decoder.
- `sinkid`: canonical parsing, ordering and the key suffix.
- `observe`: snapshots completing out of slot order under the light-client follow rule, including A→B→A changes and newer unchanged snapshots.
- `akash`: tracker checks, listing checks and cursor rules, from the cases in `scan.test.ts` and `akash.test.ts`.
- `config`: validation rules, the network guard, precedence and unknown keys.
- `signer`:
  - the keyring identity check;
  - the KMS client against a fake `SignerService`: both schemes, high-S, wrong key, bad signature, oversized response, TLS failure.

### 11.2 Parity goldens

- **Simulator.** `simnode` serves a scenario's chain state over CometBFT RPC (including `abci_query`, which the TS reporter uses), gRPC and Solana JSON-RPC.
- **TS export.** The development-only script `contracts/solana/reporter/goldens.ts` runs the TS `Scanner` in preview for each scenario, against `simnode` built from `operator/reporter/internal/simnode/cmd/simnode`. It writes the events to `operator/reporter/testdata/goldens/<scenario>.jsonl`.
- **Go comparison.** A Go test runs the Go reporter against the same scenarios and compares events as parsed JSON, under the ordering rules of section 9.1. Error texts are compared where TS asserts with an explicit message.
- **Scenarios** cover the cases listed under "Tests and schema" in `contracts/solana/docs/reporter.md`:
  - replayed evidence, CPI and truncated logs, and block spam;
  - wrong owners, amounts and recipients;
  - stale and lagging nodes, and fabricated future slots;
  - missing reported signatures and missing history;
  - own-vote reuse, duplicate votes and policy rotation;
  - contradictions, vetoes and refund eligibility;
  - light-client follow and pin;
  - paging consistency, and dispute and refund priority;
  - audit failures and the round-robin audit.
- **Regeneration.** `make reporter-goldens` regenerates the goldens and needs the `contracts/solana` development setup. CI only reads them.

### 11.3 Component tests

These are Go tests against an in-process `simnode`. They cover:

- **Broadcast mode:**
  - the unordered flag, sequence 0 and slot timeouts; a slot already taken by another process or an earlier attempt defers its messages to the next window;
  - batch limits per slot, sending when full, at the end of a review and before the cutoff, and send priority;
  - simulation pruning, batch failures without a message index, and splitting at the caps, with leftovers and second halves deferred;
  - CheckTx rejection, re-sends after a lost response, an identical copy already in the node's cache on a first send, failed delivery, a replay-key failure inside a block, and expiry;
  - the slot table: de-duplication within a window, releases after every failure before submission, deferral and re-review in the next window, and plans from an earlier window never sent;
  - the membership re-check before signing;
  - the no-index fallback;
  - a chain that halts after accepting a transaction: `scan` reports `unconfirmed`, or `not sent` for deferred messages, and exits 1 at the waiting limit, while `start` keeps the entries;
  - refund-head progression across several eligible refunds.
- **No repeated execution:** `TestNoVoteExecutesTwice` runs randomized interleavings of:
  - lower snapshots;
  - restarts (a new broadcaster against the same `simnode`);
  - lost responses;
  - two processes sharing a key;
  - nodes with and without a transaction index.

  `simnode` enforces the chain's replay-key rule and counts executions. Planning goes through a real scanner, never synthetic plans. No vote, veto or refund may execute more than once, as defined in section 6.1. A separate test sends a veto before a governance resolution and lets it execute after.
- **Drain:** 5,000 planned votes against a `simnode` that produces a block every 6 s of chain time. The test shortens the window and the block interval by the same factor through a test-only hook, and starts 90% into a window.
  - Unconfirmed transactions never exceed `tx.max-in-flight`.
  - Every vote is confirmed and none executes twice. At the defaults, the drain should take three to four windows (section 6.2); the test fails beyond eight.
  - The pass finishes without waiting for the drain.
  - A signer that is slow (100 ms per signature), then unavailable for 10 s, recovers without a restart, and the affected messages are planned again.
- **Replay key on the real application:** an `x/sink` test, using the test harness, delivers two unordered transactions from one reporter with the same timeout and the same vote, once in one block and once in consecutive blocks. The second fails with `failed to add unordered nonce`, the reporter pays one fee, and the vote is recorded once. The test runs on every pull request, so an SDK upgrade that changes the replay-key rule fails CI. A companion test, `TestUnorderedReplayKeyBoundary`, asserts that a copy delivered in a block whose time equals the timeout is refused. It stays skipped until the Akash fork adopts cosmos/cosmos-sdk#26672 (section 14).
- **Startup:** refusal on a node-ID mismatch, a gRPC height mismatch, a KMS key that does not derive `reporter`, or a missing reporter account.
- **Shutdown:** behavior and exit codes for `scan` and `start`.

### 11.4 Benchmark `steady-100k`

The benchmark runs in preview mode against an in-process `simnode`.

- **Workload:** 100,000 open transfers, a 3-member policy, and an audit page of 100.
  - 70,000 transfer-pending with no votes and no Solana status. They are absent with no reported observations, so they need no `getTransaction` calls.
  - 20,000 transfer-pending, 4,000 refund-pending before their challenge ends, and 1,000 held disputes. All are resolved on Solana with this reporter's matching vote stored, so evidence is reused and no blocks are read.
  - 5,000 void-requested, voided on Solana without this reporter's vote, spread over 100 blocks of 1,500 transactions each.
  - An audit page of 100 terminal transfers, settled consistently.
- **Injected latency:** 2 ms per Akash request, 5 ms per Solana request, plus 200 ms per `getBlock`.
- **Pass criteria:**
  - the pass takes at most 60 s, and peak heap, sampled from `runtime/metrics`, stays at most 512 MiB;
  - the summary reports no failures, and the events match the expected decisions: 5,000 planned `MsgReportOutcome`, 1,000 `Transfer requires governance review` alerts, 100 audit early exits, and no other message or alert;
  - CPU and heap profiles are kept as artifacts.
- **Where it runs:** as `BenchmarkPass` and as a test gated by `REPORTER_PERF=1`, nightly and on `workflow_dispatch`. It does not run on every pull request.

**Node-load benchmark.**

- **Setup:** 100,000 transfers deposited into an in-process akash application through the x/sink test harness (`x/sink/sink_test.go`), served by the application's own gRPC server (`servergrpc.NewGRPCServer`, as the akash node does in `chain-sdk/go/cli/server.go:433`).
- **Measurement:** height-pinned `Transfer` queries over the application's real gRPC server, from 32 concurrent clients.
- **Target:** at least 2,000 queries per second on the same runner, which leaves the 60 s pass room for listing and Solana work.
- **Load on the operator's node:** up to about 2,000 queries per second while a pass runs.
- **Gate:** this benchmark gates the first release. If it misses the target, the plan revisits the re-read strategy before release (section 14).

### 11.5 End-to-end

With `REPORTER_GO_BIN` set, `local/stack.ts` runs the Go binary for its three reporters. The variable has no `AKT_REPORTER_` prefix, because child processes inherit it. For each reporter it:

1. creates a home;
2. imports the development key into the `test` backend with `keys import-hex`;
3. writes a `reporter.toml` with `node`, `grpc-addr` (a loopback address, so plaintext), the Solana settings and `broadcast = true`;
4. runs `akt-reporter start`.

The stack already starts Akash with `--grpc.address` (`local/stack.ts:328`). The demo's two 300 s waits, for `STATUS_MINTED` and for `STATUS_REFUNDED`, must still pass. A deferral delays a message by at most about one window.

## 12. Migration and documentation

### 12.1 Operator steps

These steps go in the new `contracts/solana/docs/reporter-migration.md`.

1. Enable gRPC on the Akash node (`app.toml`: `[grpc] enable = true`) and set `grpc-addr`. The node serves gRPC without TLS. A loopback address works as is; for a node on another host, set `grpc-insecure = true` on a private network, or put TLS in front.
2. Run `akt-reporter init --home <home>`, then carry settings over using the TS field column in section 8.3.
3. Move the signing key.
   - For a key file: `akt-reporter keys import-hex reporter --keyring-backend test --home <home> < reporter.key`, then set `from = "reporter"`.
   - For an external signer: run Cosmos-KMS with a secp256k1 key for the reporter account and set `signer.remote.*`.
4. Run `akt-reporter check`, then `start` in preview. Preview never signs, so it can run next to the TS reporter.
5. Stop the TS reporter. On the node akt-reporter uses, wait until 21 blocks have been committed after the stop, so that its last ordered transactions have been included or have expired (section 6.1). Then wait two more minutes, so that every snapshot akt-reporter can still accept (at most `max-age` old, plus 10 s of clock skew) shows them. Only then set `broadcast = true`. Never run the TS reporter and akt-reporter with the same key.
6. Keep the existing alert routing. The event schema is unchanged; only the order across transfers differs.
7. Adjust fees. TS paid `feeAmountUakt` for each single-message transaction; with batching, use `gas = auto` and `gas-prices`.

To roll back, stop akt-reporter, wait until the node it used shows a block whose time is at least 2 minutes after it exited, then wait the TS reporter's `maxAgeSeconds` plus 10 s before the TS reporter broadcasts. Never run both on one key.

TS command-line flags map as follows:

| TS flag                         | Go replacement                              |
| ------------------------------- | ------------------------------------------- |
| `--watch`                       | the `start` command                         |
| `--broadcast`                   | `broadcast = true`                          |
| `--network testnet`             | `network = "testnet"`                       |
| `--key-file PATH`               | `keys import-hex` and `from`                |
| `--signer-command PATH`         | `signer.remote.*` (Cosmos-KMS)              |

### 12.2 Documentation and deployment files

- Rewrite `reporter.md`, `operator-releases.md`, `operator-configuration.md` and the reporter parts of `deploy-services.md`.
- Replace `_run/akt-reporter/` with:
  - a `docker-compose.yaml` without `command` and without today's `platform: linux/amd64`, since the image is multi-arch. It keeps today's hardening and adds an optional `env_file`. The home is mounted read-only at `/home/akt-reporter` and must be owned by the container user (`OPERATOR_UID`, default 10001). `stop_grace_period: 60s` covers the 30 s confirmation wait plus one request timeout;
  - `.env.example` and `reporter.example.toml`.

  `broadcast.yaml` is removed; broadcast becomes `AKT_REPORTER_BROADCAST=true` or `broadcast = true`.
- Update `deploy/systemd/akt-reporter.service` to `ExecStart=/usr/local/bin/akt-reporter start --home /var/lib/akt-reporter`, keeping the same sandboxing.
- Two akt-reporter processes with one key are safe. For every message they plan in the same window they share one replay key, so the chain executes at most one transaction and charges nothing for the refused one (section 6.1). Byte-identical copies are recognized by their hash (section 6.5). The documentation allows a warm standby but recommends one active signing process, because a second one wastes signatures and node load.

### 12.3 Sequencing

1. **With the Go reporter:** the new workflow, the `release-operators.yaml` trigger change, `_run/akt-reporter/`, the systemd unit and the documentation.
2. **After `reporter/v0.2.0` is published:** remove the reporter role from:
   - `deploy/operator.ts`, `deploy/service-config.ts` and `deploy/service.sh`;
   - `deploy/build-operator.sh` and `deploy/test-operator*.sh`;
   - the reporter stage of `_build/akt-operator.Dockerfile`.

   The TS reporter modules stay, for the relayer, the local stack and tests.

## 13. Security

- **RPC URLs** must not contain credentials, as in TS; authenticate through a local proxy.
- **Preview** never touches keys or signers.
- **Keys at rest.** `test` keyring files are plaintext with mode 0600, like the TS key file. The `file` backend encrypts at rest.
- **KMS.** TLS is mandatory, every signature is verified locally against the key from startup, and network isolation is required.
- **Before signing:** the membership check, the signer identity check, and the gas and fee caps.
- **Image:** distroless, no shell, non-root UID 10001, a read-only root filesystem in compose, no capabilities.
- **Metrics:** the listener is off by default; bind it to a private interface.
- **Logs and events** never contain key material.

## 14. Risks and follow-ups

| Risk                                                                                                    | Mitigation                                                                                                                                  |
| ------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| Cosmos-KMS is young: the repository dates from 2026-06, and its README and proto disagree on secp256k1 output | Pinned proto; support for both schemes; local verification; contract tests against a fake server; a CI job against a real `kms` binary gates every release that enables the remote signer |
| Operators whose custody cannot be exposed as a Cosmos-KMS `SignerService` lose their external-signer integration (D7) | The service is three calls (`GetKey`, `GetKeys`, `Sign`); the migration guide documents a minimal adapter, and the keyring remains available |
| Real per-query cost on a node: each height-pinned query builds a multistore view at that height | The node-load benchmark (section 11.4) gates the first release. If it misses, reuse listed states while the snapshot height is unchanged |
| GoReleaser OSS cannot handle `reporter/` tag prefixes                                                   | `GORELEASER_CURRENT_TAG` with `--skip=validate`, dry-run first on a prerelease tag. Fallback: snapshot build, `docker buildx imagetools create`, `gh` upload |
| The JSON shape of Solana v1 transactions                                                                | A fixture captured from an Agave 4.2.2 or later test validator; unknown versions supply no evidence                                         |
| Node setup: gRPC must be enabled, both endpoints must be one node, confirmations need a transaction index | Startup checks with explicit errors, the no-index fallback, documentation                                                                 |
| The benchmark's latencies are synthetic                                                                 | Production metrics for pass duration and RPC latency                                                                                        |
| Batched fees differ from the TS per-message fee                                                         | Caps unchanged, `gas = auto`, documentation                                                                                                 |
| darwin builds lack the `os` keyring backend                                                             | darwin is a development platform; `file` and `test` work                                                                                   |
| The no-repeat guarantee rests on the SDK's replay key (signer, timeout)                                  | A test on the real application pins it on every pull request (section 11.3), so an SDK upgrade that changes it fails CI                    |
| The SDK drops a replay key at the start of the block whose time equals its timeout, and that block still accepts the timeout | Only a proposer that skips mempool recheck or builds its own blocks can exploit it, and only at a block time equal to the timeout to the nanosecond (section 6.1). Upstream fixed it in cosmos/cosmos-sdk#26672 (follow-up 3) |
| A node lagging by up to `max-age` makes the reporter sign transactions the chain refuses as expired | Expiry events and `cutoff` deferrals show it; the remedy is a healthy node or a lower `max-age` (section 6.5) |
| Slots trade batching for uniqueness: a burst spreads over up to 128 vote transactions per window, and a 5,000-vote backlog takes about three windows | A message waits at most about one window longer; the drain test bounds the backlog; `tx.max-msgs` and `tx.max-in-flight` stay tunable |

Follow-ups, each a separate decision:

1. **The `solana.max-slot-lead` default** is ported unchanged: (max-age + 60 s) × 5 slots per second. That assumes 2.5 slots per second with a 2× margin, and faster Solana slots (SIMD-0525 and later) shrink the margin.
2. **TS reporter packaging** is removed after `reporter/v0.2.0` (section 12.3).
3. **The replay-key boundary.** Adopt cosmos/cosmos-sdk#26672 in the Akash cosmos-sdk fork, then un-skip `TestUnorderedReplayKeyBoundary`. This is a consensus change and needs a chain upgrade.

## 15. Out of scope

- Porting the relayer.
- The event-driven design.
- gRPC streaming and other 1,000,000+ optimizations.
- Changes to `x/sink`, the AKT program, or node dependencies.
- Persisted state.
- Windows builds.
- The exec signer.

## Appendix A. Events

The same lines as the TS reporter. Values are illustrative.

```json
{"id":"akash1…/1200/3","mode":"preview","audit":false,"observed":{"kind":"absent","contextSlot":"350000000","slotLimit":"350000600"},"messages":[],"alerts":[]}
{"id":"akash1…/1200/4","mode":"broadcast","audit":false,"observed":{"kind":"resolved","contextSlot":"350000000","observation":{"outcome":1,"slot":"349999000","transactionSignature":"<base64>"}},"messages":[{"typeUrl":"/akash.sink.v1.MsgReportOutcome","value":{"reporter":"akash1…","id":{"owner":"akash1…","height":"1200","sequence":"4"},"policyVersion":"2","round":"1","observation":{"outcome":1,"slot":"349999000","transactionSignature":"<base64>"}}}],"alerts":[]}
{"id":"akash1…/1100/1","audit":true,"observed":"resolved","messages":[],"alerts":[]}
{"id":"akash1…/1200/4","transactionHash":"<HEX>"}
{"id":"akash1…/1200/5","error":"resolution evidence unavailable in its block"}
{"id":"akash1…/1200/6","error":"sink message not sent: deferred (slot_busy) until 2026-11-12T10:01:59.871Z"}
{"alert":"light-client-changed","previous":"<base58>","current":"<base58>","slot":"350000000"}
{"alert":"light-client-mismatch","pinned":"<base58>","current":"<base58>"}
{"alert":"audit-unavailable","error":"<message>"}
{"error":"not a current reporter"}
{"scanned":100000,"audited":100,"index":"canonical","failures":0,"lightClientProgram":"<base58>"}
```

- `mode` is `preview` or `broadcast`.
- `outcome` is 1 for minted and 2 for voided.
- Error texts follow TS, except the broadcaster's own: `sink transaction rejected`, `expired` and `unconfirmed`, and `sink message not sent` (section 6.5).
- Plan alerts, verbatim:
  - `Akash reports a Solana slot beyond any plausible finalized slot`
  - `Akash reports an outcome without a finalized Solana status`
  - `Akash settlement contradicts finalized Solana outcome`
  - `Transfer requires governance review`
- Message values:
  - `MsgReportOutcome`: `{reporter, id, policyVersion, round, observation}`
  - `MsgVetoVoid`: `{signer, id, reason}`
  - `MsgFinalizeRefund`: `{executor, id}`
