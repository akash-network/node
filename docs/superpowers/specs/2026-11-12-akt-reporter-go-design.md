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
- Messages are sent in batched, unordered Akash transactions.

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
- **Unordered transactions** are enabled (`app/types/app.go:284`) with the SDK defaults:
  - `timeout_timestamp` must be after the executing block's time and at most 10 min later;
  - each costs 2,240 extra gas;
  - the signer sequence must be 0;
  - the replay key is (timeout in nanoseconds, signer).
- **Duplicate votes are harmless.** `MsgReportOutcome` succeeds without effect for duplicate or late same-outcome votes (`x/sink/keeper/settlement.go:88-139`). Apart from invalid input, such as a signer outside the policy or an unknown transfer, a vote fails only on a stale policy or round.
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
   - unordered transactions with sequence 0 and unique timeouts;
   - batching, send priority, simulation pruning and fee caps;
   - in-flight de-duplication, including releases after every failure before submission, and no re-send planned from a snapshot older than an inclusion;
   - a vote backlog draining while reviews proceed without waiting;
   - refunds progressing through the refund queue without waiting for another pass.
6. **Node load.** On the same runner, the real application's gRPC server, seeded with 100,000 transfers, sustains at least 2,000 height-pinned `Transfer` queries per second from 32 concurrent clients (section 11.4).

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
| `operator/reporter/tx`               | Batching, unordered transaction building, simulation, broadcast, confirmation, in-flight table                                          |
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

- slotLimit = context slot + lead. The lead is `solana.max-slot-lead`, or (max-age + 60 s) × 5 slots per second when that key is 0.
- A reported slot in (context slot, slotLimit] gives `Solana node is behind reported evidence; absence cannot authorize a veto`.
- Each reported observation at or below the context slot is looked up with `getTransaction` (finalized). If the transaction exists at that slot with that first signature, and resolves the status with the reported outcome, the result is `reported Solana transaction contradicts missing status`.

**Program logs** (port of `programEvents`). The invoke and exit patterns and the invoke-depth checks are the same as in TS. `Program data:` lines are attributed to the executing frame, including CPI frames. When `Log truncated` is the last line, only top-level AKT frames that closed with `success` count. Unbalanced logs supply no evidence.

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
3. A light-client pin mismatch stops the pass.
   - No new chunk starts, and running chunks are cancelled; events they already wrote stand.
   - The `light-client-mismatch` alert and an error are written once every running chunk has stopped, so no review event follows them. No summary is written.
   - Messages already planned are still sent; TS had sent them before it reached the mismatch.
4. Pass-start identity and freshness failures, and listing errors, end the pass with an error line and no summary. During reviews, snapshot and observation errors fail only the affected transfers (`{id, error}`), and the pass continues.
5. Stale nodes, missing history, RPC errors and signer failures never produce a veto or a refund.
6. No local state is needed after a restart. Recovery comes from votes and terminal states on chain.

**Intentional differences from TS.**

1. Per-transfer events are not written in strict priority order. Section 9.1 lists the ordering that is kept.
2. Freshness and identity checks run per snapshot instead of per transfer: at most once a second for Akash, once per chunk for Solana. Audit entries use their chunk's snapshot for both the early exit and the observation.
3. Akash state is read through gRPC instead of `abci_query`.
4. Messages are batched into unordered transactions and confirmed asynchronously. In `start`, outcome events may follow the summary.
5. AKT events other than `TransferResolved` are not decoded. TS rejected a transaction carrying a malformed one.
6. Mint `COption` tags other than 0 and 1 are rejected. TS treated any non-zero tag as present.
7. TS assertions that had no message get descriptive Go messages. CheckTx rejection and expiry get their own error texts (section 6.4).
8. The audit page is min(`scan.audit-per-scan`, `scan.page-size`). TS used min(`auditPerScan`, 500), which is identical at the default page size.
9. After each confirmed refund finalization, the new refund head is reviewed at once (section 6.5). TS reached a later head in the same pass only when it was listed after the refund just paid.

### 5.7 Shutdown

On the first SIGINT or SIGTERM:

1. No new chunk starts. In-flight requests finish or time out.
2. No new transaction is signed. Queued messages are released without an event and planned again on the next start; TS also skipped submissions after a signal.
3. In-flight transactions get up to 30 s to confirm.
4. The summary of the partial pass is written and events are flushed. As in TS, `scanned` is the number of open transfers listed, even when reviews stopped early.

`start` then exits 0, and `scan` exits 1 if the pass had failures. A second signal exits immediately.

## 6. Broadcasting

### 6.1 Batching

- `MsgReportOutcome` messages are batched, up to `tx.max-msgs` per transaction.
- `MsgVetoVoid` and `MsgFinalizeRefund` get one transaction each. They can fail on the challenge window, refund order or caps, and one failed message fails its whole transaction.
- A batch is sent when it is full, after `tx.batch-window`, or when the pass ends.
- Before signing, the latest snapshot must still list `reporter` (otherwise `reporter was removed`), and the signer's address must equal `reporter`.
- At most `tx.max-in-flight` transactions are unconfirmed at once. When a slot frees, the broadcaster sends `MsgVetoVoid` first, then `MsgFinalizeRefund`, then vote batches in plan order.
- Reviews never wait for broadcasting, so a slow or unavailable signer delays submissions, not passes. The queue needs no separate bound: the in-flight table admits at most one entry per transfer and message kind (section 6.4), so the queue never exceeds the open set. A queued message leaves the queue only when it is sent, when it fails before submission (section 6.4), or on shutdown. `scan` submits all of them and waits for their outcomes.
- With the defaults, at most 200 votes are unconfirmed at once (`tx.max-in-flight` × `tx.max-msgs`). A larger vote backlog drains over several blocks, across passes.

### 6.2 Unordered transactions

- **Body:**
  - the messages and the memo `AKT sink reporter`;
  - `unordered = true`;
  - `timeout_timestamp` = T + `tx.timeout` + δ;
  - no `timeout_height`.

  T is the block time of the latest snapshot when the transaction is built, right before signing. δ is a nanosecond offset that makes each timeout the process issues strictly greater than the previous one, so one process never repeats a replay key. A standby process signing with the same key starts its offset at a random value below one second, so a collision between the two is unlikely but possible. A collision is a CheckTx rejection (`failed to add unordered nonce`), handled like any other rejection (section 6.4); a later pass plans the message again with a new timeout.
- **Auth info:** one signer, `SIGN_MODE_DIRECT`, sequence 0, the fee and the gas limit. The account number is read once at startup. If the account does not exist, because the address never received funds, broadcast mode refuses to start with `reporter account not found`.
- **Timeout bounds.** In broadcast mode, validation requires `tx.timeout` ≥ `max-age` + the signer timeout (`signer.remote.timeout`, or 0 for the keyring) + 30 s. A transaction built from an accepted but stale snapshot is then still valid after a slow signature and broadcast. It also requires `tx.timeout` ≤ 9 min, which stays inside the chain's 10 min limit.
- **No fallback.** If the chain stops accepting unordered transactions, every broadcast is rejected with `unordered transactions are not enabled`, reported per message. There is no ordered fallback (D6).

### 6.3 Simulation, gas and fees

- Every batch is simulated through gRPC `Simulate` before signing.
- With `gas = auto`, the gas limit is ceil(gas used × `gas-adjustment`). A numeric `gas` is per message, so the limit is `gas` × the number of messages.
- If simulation fails at `message index: N`, message N is dropped with `{id, error}` and the batch is simulated again, for up to three rounds. Any messages still left are then sent one per transaction.
- A simulation failure without a message index, such as insufficient funds or an invalid timeout, fails the whole batch before submission. Each message gets `{id, error}`, and its entry is released.
- The fee is `fees` when that is set, otherwise ceil(gas limit × `gas-prices`).
- Caps: the gas limit must be at most `tx.max-gas` and the fee at most `tx.max-fee`. A batch over a cap is split in half. A single message over a cap gets `{id, error}` and is not sent.

### 6.4 Confirmation and the in-flight table

- **Broadcast** uses `broadcast_tx_sync`. After a transport error, the same signed bytes are re-sent until the timeout; the replay key makes that safe.
- **Re-sends.** For a transaction already sent once, a `tx already exists in cache` reply or a `failed to add unordered nonce` rejection means an earlier copy may be pending or included. The transaction stays submitted and is confirmed or expires as below.
- **Rejection.** Any other CheckTx rejection is final after one `tx(hash)` lookup finds nothing; without a transaction index it is final at once. So is a nonce rejection on the first send, which means a replay-key collision with another process (section 6.2). Each message gets `{id, error: "sink transaction rejected: <HASH> code <n>: <log>"}`.
- **Confirmation.** While any transaction is unconfirmed, the broadcaster refreshes the height tracker once a second, including between passes. On every new height, each unconfirmed transaction is looked up with `tx(hash)`:
  - code 0 gives `{id, transactionHash}` for each message, with the hash in uppercase hex as cosmjs printed it;
  - any other code gives `{id, error: "sink transaction failed: <HASH> code <n>"}`, the TS text.
- **Expiry** is decided by chain time only. Once a committed block's time passes the timeout, a final lookup runs. A transaction still missing can never be included, and each message gets `{id, error: "sink transaction expired: <HASH>"}`.
- **Waiting limit.** `scan` waits until its transactions resolve, or until `tx.timeout` plus 60 s of wall-clock time has passed since its last submission.
  - A transaction still unresolved then, for example because the chain halted, gives `{id, error: "sink transaction unconfirmed: <HASH>"}` and counts as a failure. It is not called expired.
  - `start` keeps such a transaction submitted until inclusion or chain-time expiry, re-sending only its existing bytes.
- **No transaction index.** If `/status` reports `node_info.other.tx_index` as anything other than `on`, the reporter logs a warning once, because outcomes cannot be observed.
  - CheckTx rejections are still reported.
  - Accepted transactions get no outcome event, do not count as failures, and leave the table at chain-time expiry. `scan` does not wait for them.
  - The next pass reflects their effect.
- **In-flight table.** Only the broadcaster adds and removes entries and writes outcome events.
  - The key is (transfer ID, message type), plus (outcome, round, policy version) for votes. A planned message whose key is present is skipped, so the table holds at most one entry per transfer and message kind.
  - An entry is *queued* from planning until its transaction's first broadcast. It is then *submitted*, because from then on inclusion is possible.
  - A queued entry is released on any failure before the first broadcast, and each such failure writes `{id, error}`: simulation rejection, signing failure, membership or signer-identity failure, or a cap rejection. On shutdown it is released without an event (section 5.7). A later pass can plan the message again.
  - A submitted entry stays until inclusion (any code), a final CheckTx rejection, or chain-time expiry.
  - Inclusion with code 0 leaves a marker carrying the inclusion height. A plan for the same key from a snapshot below that height is skipped, because such a snapshot cannot see the transaction's effect. The marker is dropped once no running chunk planned below that height.
- **Failure counting.** `scan` counts every outcome before its summary, so its exit code includes them. `start` counts an outcome in whichever pass is running when it resolves.
- **Restart.** The table is lost. The worst case is a duplicate vote (no effect, fee paid) or a refund the chain rejects; nothing executes twice.

### 6.5 Refund-head progression

`x/sink` pays refunds strictly in queue order. `MsgFinalizeRefund` succeeds only for the current refund head, which advances only when that refund executes (`x/sink/keeper/settlement.go:430-437`). The transfers of a chunk share one snapshot, so on its own a pass finalizes at most the head it saw.

The broadcaster therefore keeps the queue moving:

1. When a `MsgFinalizeRefund` is included with code 0, the broadcaster takes a fresh snapshot at once.
2. It reviews the new refund head as a one-transfer chunk: re-read at H′, Solana snapshot, plan.
3. If that review plans another finalization, it is sent the same way, and the steps repeat.

Progression stops when the head plans no finalization (not yet eligible, held, or the epoch capacity is used), or on shutdown. `scan` waits for progression to stop before its summary. The in-flight table keeps a pass review and a progression review from sending the same finalization twice. Refunds are paid at about one per block.

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
- **Transport:** TLS only; there is no plaintext option.
  - `signer.remote.ca-file` sets the trusted CA; system roots are used when it is empty.
  - `signer.remote.server-name` overrides the verified server name.
  - An optional client certificate (`cert-file`, `key-file`) is for an mTLS proxy in front of the KMS.
  - `signer.remote.timeout` applies per call.
- **Access.** The `SignerService` authenticates no caller. Operators must restrict network access so that only the reporter can reach it.
- **Release gate.** A release enables the remote signer only after a CI job against a real `kms` binary passes for each supported scheme.

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
| `init`    | Writes `<home>/config/reporter.toml` rendered from the effective configuration (defaults plus any flags and environment). Refuses to overwrite without `--overwrite`.           | None                          |
| `start`   | Runs a pass, waits `scan.interval`, and repeats. This is the image's default command.                                                                                          | `run --watch`                 |
| `scan`    | Runs one pass. Exits 1 if anything failed.                                                                                                                                     | `run`                         |
| `check`   | Validates offline, including the network guard and signer selection, with no RPC, keyring or signer access. Prints `Configuration valid. No RPC request or transaction sent.` | `check`                       |
| `keys`    | The cosmos-sdk keyring commands: `add`, `import`, `import-hex`, `export`, `list`, `show`, `delete` and the rest.                                                               | `keys create`, `keys address` |
| `version` | Prints the service name, version, commit, platform and Go version.                                                                                                             | `--version`                   |

`start` and `scan` first run the `check` validation. They then connect to both Akash endpoints and run the node-ID check, plus the transaction-index check in broadcast mode. In broadcast mode they load the signer, check its address and read the account number. Finally they start the metrics listener, if configured.

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
| `max-age`                                           | `60s`                           | Above 0, at most `10m`                                         | `maxAgeSeconds`                  |
| `request-timeout`                                   | `15s`                           | `1s` to `60s`                                                  | none                             |
| `from`                                              | empty                           | Keyring key name or address                                    | `keyFile`                        |
| `keyring-backend`                                   | `test`                          | `os`, `file` or `test`; also used by `keys`                    | none                             |
| `keyring-dir`                                       | the home directory              |                                                                | none                             |
| `gas`                                               | `auto`                          | `auto`, or gas per message; the transaction limit is that times the message count, at most `tx.max-gas` | `gasLimit` |
| `gas-adjustment`                                    | `1.3`                           | `1.0` to `10.0`                                                | none                             |
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
| `tx.max-gas`                                        | `20000000`                      | 1 to 20,000,000                                                | Bound on `gasLimit`              |
| `tx.max-fee`                                        | `1000000uakt`                   | `uakt`, at most `1000000uakt`                                  | Bound on `feeAmountUakt`         |
| `tx.max-msgs`                                       | `25`                            | 1 to 100                                                       | none                             |
| `tx.batch-window`                                   | `500ms`                         | `0` to `5s`                                                    | none                             |
| `tx.timeout`                                        | `2m`                            | `30s` to `9m`; when broadcasting, at least `max-age` + signer timeout + 30 s (section 6.2) | none   |
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
  - The summary is the pass's last event. The exceptions are transaction outcome events and refund-progression reviews in `start` (sections 6.4 and 6.5).
  - Events of different transfers have no defined order.

### 9.2 Logs on stderr

Akash-style zerolog. Logs cover:

- a startup summary of the effective configuration, without key material;
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

`result` is `ok` or `error` for reviews, `hit` or `miss` for caches, and `included`, `failed`, `rejected` or `expired` for transactions. `chain` is `akash` or `solana`.

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

- `policy`: every case in `policy.test.ts`.
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
  - the unordered flag, sequence 0 and unique timeouts, and recovery from a replay-key collision with a second signing process;
  - batch limits, the batch window and send priority;
  - simulation pruning, batch failures without a message index, and splitting at the caps;
  - CheckTx rejection, re-sends after a lost response, failed delivery and expiry;
  - in-flight de-duplication across passes: queued entries released after every failure before submission, submitted entries kept after an ambiguous broadcast, and no re-send planned from a snapshot below an inclusion;
  - the membership re-check before signing;
  - the no-index fallback;
  - a chain that halts after accepting a transaction: `scan` reports `unconfirmed` and exits 1 at the waiting limit, while `start` keeps the entry;
  - refund-head progression across several eligible refunds.
- **Drain:** 5,000 planned votes against a `simnode` that produces a block every second.
  - Unconfirmed transactions never exceed `tx.max-in-flight`.
  - Every vote is confirmed within 50 blocks (twice ⌈5,000 / 200⌉), with no message sent twice.
  - The pass finishes without waiting for the drain.
  - A signer that is slow (100 ms per signature), then unavailable for 10 s, recovers without a restart, and the affected messages are planned again.
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

- **Setup:** 100,000 transfers seeded into an in-process akash application built with the repository's `testutil/network` helpers.
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

The stack already starts Akash with `--grpc.address` (`local/stack.ts:328`).

## 12. Migration and documentation

### 12.1 Operator steps

These steps go in the new `contracts/solana/docs/reporter-migration.md`.

1. Enable gRPC on the Akash node (`app.toml`: `[grpc] enable = true`) and set `grpc-addr`. The node serves gRPC without TLS. A loopback address works as is; for a node on another host, set `grpc-insecure = true` on a private network, or put TLS in front.
2. Run `akt-reporter init --home <home>`, then carry settings over using the TS field column in section 8.3.
3. Move the signing key.
   - For a key file: `akt-reporter keys import-hex reporter --keyring-backend test --home <home> < reporter.key`, then set `from = "reporter"`.
   - For an external signer: run Cosmos-KMS with a secp256k1 key for the reporter account and set `signer.remote.*`.
4. Run `akt-reporter check`, then `start` in preview, then set `broadcast = true`.
5. Keep the existing alert routing. The event schema is unchanged; only the order across transfers differs.
6. Adjust fees. TS paid `feeAmountUakt` for each single-message transaction; with batching, use `gas = auto` and `gas-prices`.

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
- Two processes with one key no longer contend for a sequence. It is safe, but every fee is paid twice, so the documentation keeps recommending one signing process per key.

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

Follow-ups, each a separate decision:

1. **The `solana.max-slot-lead` default** is ported unchanged: (max-age + 60 s) × 5 slots per second. That assumes 2.5 slots per second with a 2× margin, and faster Solana slots (SIMD-0525 and later) shrink the margin.
2. **TS reporter packaging** is removed after `reporter/v0.2.0` (section 12.3).

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
{"alert":"light-client-changed","previous":"<base58>","current":"<base58>","slot":"350000000"}
{"alert":"light-client-mismatch","pinned":"<base58>","current":"<base58>"}
{"alert":"audit-unavailable","error":"<message>"}
{"error":"not a current reporter"}
{"scanned":100000,"audited":100,"index":"canonical","failures":0,"lightClientProgram":"<base58>"}
```

- `mode` is `preview` or `broadcast`.
- `outcome` is 1 for minted and 2 for voided.
- Plan alerts, verbatim:
  - `Akash reports a Solana slot beyond any plausible finalized slot`
  - `Akash reports an outcome without a finalized Solana status`
  - `Akash settlement contradicts finalized Solana outcome`
  - `Transfer requires governance review`
- Message values:
  - `MsgReportOutcome`: `{reporter, id, policyVersion, round, observation}`
  - `MsgVetoVoid`: `{signer, id, reason}`
  - `MsgFinalizeRefund`: `{executor, id}`
