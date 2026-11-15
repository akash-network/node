# Running a relayer

The relayer carries Akash state to Solana. It uploads signed Akash headers to
the ICS07 light client and submits ICS23 proofs to `mint_transfer` and
`void_transfer`. It holds no trusted power: the light client verifies every
header and the AKT program verifies every proof. Anyone can run one against any
deployment with their own nodes and payer key. Several can run at once. Each
transfer resolves once on Solana. A relayer that loses the race pays only its
proof-upload fees; its resolution fails simulation and is not sent.

The relayer is the Go command `akt-relayer`, built from `cmd/akt-relayer`.
`run --config` serves an existing deployment; `local` uses the
[local stack](local-stack.md).

## Requirements

**Binary.** Use the [operator download or image](operator-releases.md), or build
`./cmd/akt-relayer` from the node repository with its pinned Go toolchain.
The binary includes header encoding and process locking; no Node, npm, Rust,
Python, Akash validator or Solana validator is needed on a relayer host.

**Akash node.** A CometBFT RPC endpoint you run or trust:

- `abci_query` with `prove=true` on the `sink` store, at every height from the
  light client's latest verified height to now. Each relay proves at that
  height, which can be up to `refreshFraction` of the trusting period old
  (about 5 hours on the test network). Keep at least one full trusting period of state;
  `pruning = "nothing"` is simplest.
- `commit` and `validators` at the client's latest height plus one and at the
  newest block.
- Not catching up, with its latest block younger than `maxAgeSeconds`.
- The matching x/sink interface and its canonical queries are required. See [canonical Sink](sink-migration-v2.md).

**Solana RPC.** HTTP JSON-RPC only; the relayer polls and opens no WebSocket. It
uses `getAccountInfo`, `getMultipleAccounts`, `getProgramAccounts` on the ICS07
program (pruning), `simulateTransaction` through preflight, `sendTransaction`,
`getSignatureStatuses`, `getTransaction`, `getLatestBlockhash` and address
lookup tables. A dishonest RPC cannot make the programs accept a false header
or proof, but it can waste fees. Use your own node or a provider you trust.

**Payer.** A Solana CLI keypair file, mode 0600, owned by the relayer user.
Give each relayer its own key. Fund it as described under
[costs](#costs-and-funding).

**State directory.** One per deployment and payer. It holds header-upload
journals, `header-uploads/payer.lock` and the spend ledger. A restart resumes an
interrupted upload from it. Never point two running relayers at one directory.

## Configure

Copy [`relayer/config.example.json`](../relayer/config.example.json). Use absolute
paths for `payerKeyFile`, `stateDir`, and the deprecated `headerEncoder` field if
supplied. Unknown fields are rejected.

| Field                    | Required | Meaning                                                                     |
| ------------------------ | -------- | --------------------------------------------------------------------------- |
| `akashRpc`               | yes      | CometBFT RPC URL                                                            |
| `akashChainId`           | yes      | Akash chain; the node and the bridge account must both match                |
| `solanaRpc`              | yes      | Solana JSON-RPC URL; no credentials in the URL                              |
| `solanaGenesisHash`      | yes      | Checked at startup; get it from a trusted source, not the RPC you configure |
| `programId`              | yes      | AKT program                                                                 |
| `payerKeyFile`           | yes      | Owner-only keypair file                                                     |
| `stateDir`               | yes      | Journals and spend ledger                                                   |
| `lightClientProgram`     | no       | Pin. Unset follows the bridge account                                       |
| `maxLamportsPerTransfer` | no       | [Spend limit](#spend-limit), default 5,000,000                              |
| `maxHeaderSignatures`    | no       | [Large commits](#large-validator-sets) above this are trimmed, default 100  |
| `refreshFraction`        | no       | Update an idle client after this share of its trusting period, default 1/5  |
| `retryMaxSeconds`        | no       | Longest backoff for a failed transfer, default 900                          |
| `pruneIntervalSeconds`   | no       | Consensus-state pruning interval, default 600                               |
| `maxAgeSeconds`          | no       | Oldest acceptable Akash block, default 60                                   |
| `pollSeconds`            | no       | Pause between passes, default 3                                             |
| `headerEncoder`          | no       | Deprecated legacy path; validated if present, never executed                |

Skipping trust across a long interval needs the old validator set to hold more
than a third of the new voting power. Lower `refreshFraction` if the set changes
quickly.

## Run

For production installation, supervision and rollback use
[service deployment](deploy-services.md). The generic commands below also
support explicitly configured development networks.

```bash
akt-relayer run --config /etc/akt-relayer/config.json --watch
```

Drop `--watch` for one pass. Run it under a supervisor that restarts it. Each line of output is JSON;
alerts go to stderr.

## Light-client discovery

Governance can [re-pin](deploy-program.md#later-changes) the
bridge to another light client at any time, so the relayer never uses fixed
verifier addresses. Every pass it reads:

| Account                | Seeds                                  | Holds                                          |
| ---------------------- | -------------------------------------- | ---------------------------------------------- |
| Bridge                 | `["bridge"]` under the AKT program     | `light_client_program`, `chain_id`             |
| ICS07 client state     | `["client"]` under the light client    | Chain, trust parameters, latest height, frozen |
| ICS07 consensus states | `["consensus_state", height LE u64]`   | Timestamp and app root per verified height     |
| ICS07 app state        | `["app_state"]` under the light client | `am_state.access_manager`                      |
| Access manager state   | `["access_manager"]` under that        | `RELAYER_ROLE` members                         |

- A changed program logs `light-client-changed` with both IDs and the relayer
  follows it. A header upload in flight for the old client moves to
  `header-uploads/abandoned-*.json`; its temporary accounts keep their rent
  under the old program.
- A bridge that accepts a chain other than `akashChainId` logs
  `bridge-chain-mismatch` and the relayer pays nothing.
- With `lightClientProgram` set, a different program logs
  `light-client-mismatch` and the relayer pays nothing until the bridge or the
  pin changes. It keeps polling and repeats the alert every 15 minutes.

The monitor, health check and reporters follow the same account.

## Update rights

Mints and voids are permissionless. Header updates and misbehaviour need
update rights, logged as `relayer-access` at startup and on change:

| `access` | How                                                                       | Updates headers | Prunes |
| -------- | ------------------------------------------------------------------------- | --------------- | ------ |
| `member` | The access-manager admin granted `RELAYER_ROLE` to the payer              | yes             | yes    |
| `open`   | `RELAYER_ROLE` lists the ICS07 program's own ID, the open relaying marker | yes             | no     |
| `none`   | Neither                                                                   | no              | no     |

With `none`, the relayer relays only transfers the latest verified height
already proves, and logs `relay-header-blocked` when an update is needed. It
does not resume a stored upload. [Later changes](deploy-program.md#later-changes)
lists the `grant-relayer` and `open-relaying` commands; [IBC verifier](verifier.md)
covers the program change.

## Large validator sets

Assembly runs out of compute or heap above about 100 signatures, and nothing
above 128 fits one update. When a commit has more than `maxHeaderSignatures`
votes, the relayer keeps the highest-power signers until they hold more than
2/3 of the new set's power and, when the update skips heights, more than the
client's trust level of the trusted set's power, each with 1% to spare where
possible. The other votes become `Absent`; the verifier needs no more. It logs
`header-signatures-trimmed` with the counts.

If no subset of at most 128 signatures passes, or assembly still exhausts
compute or heap, the header is unsupported: the relayer logs
`header-unsupported` and tries again only after `retryMaxSeconds`, since the
same validator set fails the same way.

## Costs and funding

A relay pays for proof creation, optional growth, uploads of up to two 450-byte chunks per transaction, resolution and close. Query the actual cluster fee and rent schedules when funding the payer.

Proof buffers contain an 80-byte header and at most 16,384 proof bytes. Creation allocates at most 10,240 bytes, and one growth funds the remaining allocation when needed. This rent is temporary liquidity. Resolution also locks permanent outcome-account rent and, for a new recipient token account, ATA rent.

Read `proof-buffer-ready` for the actual allocation and refundable balance. `relay-payment` records fees, payer balance deltas and buffer funding or reclamation. The ledger's `bufferFundedLamports` and `bufferReclaimedLamports` accumulate until an explicit budget reset. After a reset, inspect the proof journal's observed lamports and the on-chain buffer balance for liquidity still held. Allow enough payer liquidity for the full proof buffer in addition to the net-spend budget. A 0.05 SOL balance can be insufficient for a large proof.

Header updates store consensus accounts and use separate temporary accounts. Updates occur when a transfer needs a newer proof height or when the client reaches `refreshFraction` of its trusting period. Monitor the payer balance and reclaim expired consensus states under the existing permissions.

## Spend limit

The relayer records the lamports it pays per transfer: every fee, plus the
rent its resolving transaction locks. Refundable proof-buffer rent is tracked separately. An interrupted upload resumes its existing allocation and byte prefix. A changed canonical proof closes the exact journaled buffer before replacement.
Header updates serve all transfers and are not charged.

When a transfer's total exceeds `maxLamportsPerTransfer`, the relayer stops
paying for it and logs `relay-budget-exhausted` with the total and attempt
count, then every 15 minutes. It still closes a leftover proof buffer, which
only returns rent. The default net-spend limit remains 5,000,000 lamports. Buffer liquidity does not authorize increasing that limit.

The ledger is `<stateDir>/relay-budget-v2-<genesis>-<programId>.json`, so restarts
keep it. Investigate the transfer's `relay-retry` errors first, then reset:

```bash
akt-relayer run --config /etc/akt-relayer/config.json --reset-budget 'akash1.../9001/42'
akt-relayer run --config /etc/akt-relayer/config.json --reset-budget all
```

The running relayer reads the ledger every pass. Resolved transfers remain recorded in it
automatically.

## Checks before paying

For each transfer without a Solana status, the relayer:

1. Waits without paying while the bridge is paused or the per-transfer maximum
   or daily cap applies (`relay-wait`).
2. Never mints a record Akash shows as refund-pending or refunded without a
   Solana status. It still relays a governance void order in either status.
   Without that order, it reports `relay-unbacked-settlement`.
3. Right before the first paid transaction, re-reads the transfer at the newest
   Akash height. It skips if a planned mint became unbacked or the void order
   changed (`relay-final-check`).
4. Simulates every transaction before sending it, so a failing one costs
   nothing.

`mint_transfer` stays permissionless, so these checks protect only honest
relayers. The reconciler reports unbacked records.

## Pending transfers

The relayer queries every dispute, then refund-pending, then void-requested (`VOIDED`), then transfer-pending transfer. It also discovers immutable history in chronological order. Every page uses one committed height. A durable full-ID checkpoint survives empty `next_key` pages and restarts. No numeric fallback exists. An open-set listing error stops the pass. A history discovery failure raises `relay-discovery-failed` and retains the checkpoint; open transfers still run.

A terminal conflict is still discoverable. An Akash refund-pending or paid refund without a finalized Solana status produces an alert and no mint. An immutable governance order is a distinct proof action, not a changed deposit value.

After cap admission, fresh pending deposits use a stable order derived from the
payer's public key so different relayers are less likely to start the same upload.
Disputes, refunds, void orders and retained uploads keep their queue positions.
Every relayer still considers every eligible transfer; no roster or additional
configuration is needed. Races remain possible, and using the same payer gives
the same order.

At most two historical proofs prepare concurrently. Payments remain sequential,
with current-state checks before spending and resolution. A slow proof read can
delay its batch partner.

`relay-stage-duration` logs `stage`, optional transfer `id`, `elapsedMs` and
`completed`. Use these to locate slow queries, uploads, header updates or ticks.
`completed` means the operation returned without throwing, not that the Akash
settlement is complete. Paid stages include transaction preparation and
confirmation; the total tick includes its child stages, so do not add them
together.

## Alerts

Route these to a person.

| Event                                                 | Meaning                                            | Action                                                       |
| ----------------------------------------------------- | -------------------------------------------------- | ------------------------------------------------------------ |
| `light-client-changed`                                | Governance re-pinned the bridge                    | Confirm the proposal; check `relayer-access`                 |
| `light-client-mismatch`                               | Bridge differs from the pin; paying stopped        | Verify the re-pin, then update or remove the pin             |
| `bridge-chain-mismatch`                               | Bridge accepts another Akash chain; paying stopped | Check `akashChainId` and the deployment                      |
| `relayer-access` with `none`, `relay-header-blocked`  | Payer cannot update the client                     | Request the role, or rely on another updater                 |
| `relay-budget-exhausted`                              | Spend limit reached for one transfer               | Read its `relay-retry` errors, fix, reset                    |
| `relay-final-check`                                   | Akash changed between planning and paying          | Review the transfer; usually a governance action             |
| `relay-unbacked-settlement`                           | Refund state without a Solana status or void order | Investigate the reporters or governance                      |
| `relay-discovery-failed`                              | History scan failed; open relays continue          | Restore access to the required history                       |
| `expiry-certificate-superseded`                       | Observed transaction supersedes an absence capsule | Preserve evidence; investigate provider history and finality |
| repeated `relay-retry`, `header-retry`                | Transfers or updates keep failing                  | Check RPCs, payer balance and client status                  |
| `header-upload-abandoned`                             | Re-pin during an upload                            | None; the new client gets a fresh upload                     |
| `header-unsupported`                                  | No commit subset fits one update                   | The client will expire; plan a verifier change               |
| `consensus-prune-failed`, `proof-buffer-close-failed` | Rent recovery failed                               | Retried later; check the RPC if it persists                  |

Watch the light client's age as well. The relayer stops at expiry with
`light client expired`; recovery is a governance re-pin.

## V2 durable recovery

Follow [buffer funding and recovery](sink-migration-v2.md#buffer-funding-and-recovery). Start v2 in a fresh namespace. Preserve scalar-ID state files. Budget files bind the new deployment and reject old versions. Signed transaction jobs and charged-signature dedupe survive restart; retries retain their deadline. Never delete a pending job to bypass an ambiguous transaction.

An expired blockhash and a missing signature result alone cannot establish whether a transaction landed earlier. Recovery first requests finalized transaction metadata so that fees and payer balance changes remain accounted for. A permanent transfer-status account establishes the business outcome but supplies neither of those amounts. A budget reset cannot bypass unresolved transaction history.

For eligible ordinary transactions, recovery can also certify non-inclusion using the configured Solana RPC: it reads a bounded window of 165 complete finalized blocks around the signed blockhash's origin and expiration, verifies parent hashes and heights, checks every returned signature, and repeats the genesis, outcome and blockhash-invalidity checks. This relies on the configured provider returning complete, accurate signature lists; it is not cryptographic proof of non-inclusion. Contradictory local observations, inconsistent block views, missing history or an unsupported transaction preserve the exact signed intent and stop recovery. Restore retained or archival history for the same genesis and retry when certification cannot complete.

A successful certificate is durably preserved with the original intent and its state associations under `expired-intents-v1` in the state root before the pending file is removed. Keep this evidence. Retirement stops the current pass without inventing a transaction result, charging fees or completing a transfer. The next pass reads fresh state before preparing more work. Cached certificates require fresh checks against their provider after restart, including current local contradictions. A legitimate change to the associated session or proof document is validated afresh; it does not rewrite the certificate's original association evidence. If the configured provider identity changes, recovery performs a full new certification and preserves it in an immutable endpoint generation beside the original capsule. Conflicting finalized block views or corrupt evidence stop retirement. Proof-buffer cleanup and ledger completion cannot bypass this recovery barrier; do not manually delete pending files or certificates to force progress.

If settlement later observes a transaction for an interrupted retirement, the relayer first writes an immutable `.superseded.json` marker with the actual receipt evidence, then emits `expiry-certificate-superseded`. This marker prevents every provider generation for those exact pending bytes from being reused. The original capsules remain intact. Investigate the provider's history completeness and finality; the event does not by itself prove a provider fault. Receipt confirmation requirements remain those of the existing stage: some header signature/chunk stages can settle at confirmed commitment, while the expired-history recovery path requests finalized metadata. Actual fees and balance changes still pass through the observer before pending unlink. A marker write failure retains pending and does not invoke the observer. After an observer failure or crash the observer may run again; the spend ledger deduplicates charges by signature. Missing or changed association documents do not block this positive-receipt reconciliation.

Positive receipt reconciliation still requires any existing capsule and its namespace to be readable and valid. A damaged or noncanonical capsule, an inaccessible namespace, or a symlinked state root can withhold the observer even when the RPC returns a valid receipt. After a supersession marker is written, a retry must reproduce the whole receipt document; changed logs or optional metadata can cause an immutable-receipt collision despite unchanged fees and balances. Preserve the original receipt, marker and capsule, and reconcile the conflict against verified transaction history. Do not remove evidence to force the observer through.

New provider generations are compared with the original capsule, not every other endpoint generation. Each generation still needs its own complete provider certification; this is not multi-provider consensus. A conflict with the original prevents publication and retirement. A later admitted attempt can repeat the complete scan because a rejected generation is not retained as a reusable negative result. Repeated conflicts therefore require investigation rather than repeated provider switching.

Local evidence scanning has no cumulative journal-size or session-count cutoff. The header journal is streamed to its end with a 2 MiB limit per line; session documents are read in batches to the end of the directory with a 2 MiB limit per document. The existing spend ledger is read and parsed as a whole document, without an expiry-only size cap, so its memory cost still grows with the ledger. Pending documents remain limited to 16 KiB and individual capsules to 80 MiB. All these local evidence files must be regular files, and the configured state root and directories below it must be real directories, not symlinks. Recovery does not canonicalize a symlinked state root. The payer lock coordinates product writers; file and directory identity, size and modification checks reject changes detected during scanning, and cancellation stops reads. These checks do not protect against an arbitrary writer bypassing the lock.

A malformed or torn journal line is a recovery barrier because it may contain a terminal transaction observation. The diagnostic identifies the line and requires repair from verified history. Cancellation at a read boundary can also produce this diagnostic, so confirm whether the attempt exhausted its deadline before treating the file as damaged. Preserve the journal, pending intent, sessions, ledger and capsules; recover genuinely missing or damaged evidence from a verified retained copy or transaction history before retrying. A later append does not repair a torn line. There is no automatic repair or supported history rotation in this change; do not truncate, discard a fragment, delete certificates or assume absence to force recovery.

Certification remains an all-or-nothing attempt, with a 120-second overall deadline, 256-request limit, eight-second per-request deadline, 260 ms minimum request spacing, 2 MiB per response and 64 MiB total response budget. Local scans consume time within recovery's overall deadline; ledger parsing still has its existing whole-document memory cost. Complete block signature lists can exhaust the response budget, and sufficiently slow providers or large local histories can exhaust the time budget. Partial scans are not checkpointed. Failed attempts back off from one minute to one hour; a longer provider `Retry-After` is capped at two hours. These limits and the trusted-provider completeness assumption mean recovery cannot guarantee automatic progress in every deployment. Capsules retain full fetched evidence and continue consuming disk until an operator's separately reviewed retention process applies.

Check existing journal size before cutover and investigate recovery duration as history grows. Treat 200 MiB of journal or 5,000 header uploads as an early review trigger, not a safe capacity limit or a deletion threshold. A local synthetic measurement of 5,000 100-signature uploads took about ten seconds per scan on one Mac, without provider latency or the race detector. An attempt scans local evidence twice, sharing the same deadline with network requests; slower storage, different line shapes and provider latency reduce the available margin. Unbounded journal growth can eventually make every attempt time out. This release preserves the finite deadline rather than promising recovery for unlimited retained history.

The block locator also has a finite bracket of 4,096 slots. A sufficiently old intent or a large skipped-slot span can remain unlocatable even when the RPC retains the blocks. Changing providers or retrying cannot guarantee that this limit is overcome. Preserve the intent and evidence for a separately reviewed recovery procedure; a locator refusal is not evidence that the transaction never landed.

For an unfinished proof, the relayer checks the exact saved key at its old Akash state height. Only the store's explicit `version does not exist` response at that key and height permits moving past a lost checkpoint. Signed Solana intent reconciliation runs first. The relayer obtains a newer exact native proof and rechecks current state before closing the owned old buffer and replacing the job. Fees and returned rent use transaction metadata as usual. If no newer usable checkpoint exists, the job remains. Unknown RPC failures, ambiguous Solana history, and invalid replacement proofs preserve the evidence for retry. Final on-chain proof verification and clock checks remain required.
