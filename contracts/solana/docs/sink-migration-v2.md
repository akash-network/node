# Launch and recover canonical Sink services

This account layout is for a new launch with an unused Sink module and fresh Solana program, mint and service state. It changes instruction, account and event encoding and every permanent status PDA, including 20-byte owners. Do not upgrade an existing program in place or point a fresh program at previously resolved native deposits: the old outcome accounts would no longer prevent replay. Existing test deployments remain on their original binaries and namespaces. Preserve their keys, ledgers and receipt history.

## Prepare the namespace

1. Build the SDK and install the same local archive into the bridge and portal. Preserve package versions and unrelated lock entries. Compare installed package bytes with the archive before testing.
2. Build the program and generated IDL with `npm run build`. Configure observers, relayers and the portal with the program, its canonical mint, the Akash chain ID and both genesis identities.
3. Choose a fresh run name with `AKT_LOCAL_RUN`. The defaults remain `live-stack-v2` and `testnetify-stack-v2` under the node repository's `.cache/solana/run`. These directory names do not identify the protobuf schema.
4. Export complete Sink genesis using the paired node version. Retain `records`, `accounts`, `policies`, `policy_sequence`, send usage and refund usage, together with the complete native `x/epochs` state. Each record contains its ID, immutable data, settlement and financial status. Preserve permanent void authorizations and every referenced vote or veto policy.
5. Set `initial_height` above the exported height and `genesis_time` at or after that height's committed block time. Keep the raw export and prepare a separate import file. The importer rebuilds derived indexes and rejects inconsistent escrow, totals, identities or policy references. Do not relabel an earlier schema or truncate its evidence.
6. Read `State`, `Params` and `Reporters` (version 0) at the same committed height before enabling deposits. Verify reporter membership, decimal threshold, Coin caps, native epoch, escrow backing and the Solana client's trust state.

## Identity and proofs

A transfer ID is `owner/height/sequence`. The owner is a canonical Akash account of 1–255 bytes, matching the native SDK. Height is a positive int64. Sequence is a positive uint64 allocated within a block, beginning at one. TypeScript uses bigint and JSON uses decimal strings.

The mint key is `0200 || U8(owner_length) || owner || BE64(height) || BE64(sequence)`, `19 + owner_length` bytes. Its value is the SDK's native protobuf `RecordData`, containing a canonical base58 recipient and a positive uint64 `uakt` Coin amount.

The settlement key is `0301 || status || U8(owner_length) || owner || BE64(height) || BE64(sequence)`, `20 + owner_length` bytes. Its value is native protobuf `SettlementData`. Status belongs to the key and to the public `TransferState.status` field. The settlement embeds its reporter policy and permanently retains `void_authorization.requested_height` and `requested_round` after governance orders a void.

A governance void proves the complete settlement value. The relayer binds the returned key, value and IAVL existence proof to one queried snapshot. It rejects a wrong height, round, authorization, key, value or store. The program authenticates both ICS23 membership levels under `["sink", key]`. A value hash in a local job never replaces the value sent for membership verification.

The permanent outcome PDA uses `transfer`, SHA-256 of the raw owner bytes, LE64 height and LE64 sequence. Hashing keeps the owner seed within Solana’s 32-byte limit. The Anchor IDL cannot represent a hash-derived seed; clients supply the PDA using `statusAddress`. Proof buffers use `proof`, submitter32 and all 32 bytes of SHA-256 of the exact key. Each settlement status has a different buffer nonce. Header H proves state at H-1. The proof must follow the deposit height and cover the authorization's request height.

## Financial states and authorization

The five financial populations are `TRANSFER_PENDING`, `MINTED`, `VOIDED`, `REFUND_PENDING` and `REFUNDED`. Each has its own Coin amount and uint64 count in totals. Escrow contains the first four populations. Refunded amounts have already left escrow.

`VOIDED` is a governance order awaiting a Solana outcome. The receipt API calls it `void_requested`. It promises no refund. `SolanaReceipt.kind: "voided"` describes an actual confirmed Solana outcome. A permanent minted PDA blocks voiding, and a permanent voided PDA blocks minting.

Holds are independent of financial status. Conflicts can include terminal records. Reporter messages carry the current policy version and settlement round, with flat `observation: { outcome, slot, transaction_signature }`. The first quorum freezes its certificate. One opposite witness can create a conflict. Same-outcome reports cannot enlarge a frozen certificate, and further reports after conflict have no effect. The reporter avoids paying for those no-op submissions.

An unheld unpaid reporter attempt refreshes under a new policy. Held and terminal evidence remains. Governance-confirmed refund authority survives reporter rotation. The permanent void order survives status changes, restart, rotation and refund. It permits a Solana sealing attempt even after a refund, while an unbacked mint is refused.

`MsgRequestVoid`, `MsgRestartTransfer`, `MsgConfirmMint` and `MsgConfirmVoid` carry `expected_round`. Read the round before preparing a proposal. Confirm messages carry typed Solana evidence. Only accepted restart or confirmation increments the round. Retrying an existing order preserves its original receipt.

## Limits and deadlines

The transfer maximum, aggregate send cap and refund cap are positive `uakt` Coins. The minimum may be zero while deposits are disabled. Zero disables the account send cap. Account and aggregate caps may be below the transfer maximum. There are no account count quotas.

Account, aggregate send and refund usage follow the current number of the existing `x/epochs` schedule named `day`. Its duration and start come from epochs genesis. Preserve that schedule on import. Epoch zero permits no deposit or refund payment. Refunds do not restore send capacity, and changing a cap preserves consumption.

`challenge_end` is bigint Unix seconds, positive for refund-pending and zero otherwise. The independent challenge Duration must fully elapse even when quota epochs change. Refund payment also requires actual finalized void evidence, current reporter or retained governance authorization, no hold, enabled refunds, queue priority and available epoch capacity.

Solana keeps its independent per-transfer and daily mint caps. Mints submit immediately. Pause, proof failure and cap failure roll back status creation and issuance.

## Buffer funding and recovery

The Solana upload limit is 16,384 bytes. Native reporter policy has no corresponding count limit, so this is not a proven bound on every valid native settlement. Audit and launch must address that mismatch; increasing the reporter set can create an unrelayable governance void. Test the complete proof for the proposed policy and retain the larger-policy case in the audit scope.

For a proof that fits, the account needs an 80-byte header plus its declared proof length. Creation allocates at most 10,240 bytes. `growProofBuffer()` funds and allocates the rest, at most one growth with the current capacity. Uploads append at most 500 bytes per transaction. Resolution requests a 256 KiB heap and the existing compute limit.

The payer needs temporary liquidity for the buffer as well as permanent spend. Query the cluster's rent exemption for `80 + proofLength` before funding. `proof-buffer-ready` records the actual allocation and refundable lamports. `relay-payment` records transaction fees, payer balance changes and buffer funding or reclamation. The ledger retains cumulative `bufferFundedLamports` and `bufferReclaimedLamports` separately from `spentLamports`.

The default net-spend budget stays 5,000,000 lamports per transfer. Every transaction fee counts, including failed transactions and cleanup. The resolving transaction also counts its permanent account rent. Refundable buffer liquidity does not count as permanent spend. Header costs remain separate. Budget exhaustion stops paid work while permitting cleanup.

The proof journal lives beside the transaction intent as `relay-jobs-v2/<genesis>/<program>/<transfer-key-hash>.json.proof`. It binds chain ID, Solana genesis, program, payer, transfer ID, light client, exact key, full nonce, value hash, proof height, proof bytes and upload progress. The signed transaction intent is persisted before broadcast. Restart reconciles the same signature before sending another transaction and charges it once.

The relayer reads the actual allocation and uploaded prefix after restart. It grows or appends only what is missing. When status, value, round, proof height or client changes, it closes the old journaled buffer before creating a replacement. It preserves unresolved signed transactions and never deletes an intent to force another submission. A completed outcome triggers buffer reclamation without another resolution. Keep journals, budget files and discovery checkpoints together.

## Discovery and receipt history

Each pass reads every dispute, then refund-pending, void-requested and transfer-pending records at one pinned height. There is no backlog count limit. Durable unfinished jobs also return to the queue after restart. `Transfers` pagination keys are opaque server-issued tokens; clients neither construct them from IDs nor compare their bytes for ordering. Discovery saves the token that starts its last page and the last processed ID. At EOF it replays that page, skips already-seen IDs, and discovers later deposits. The `relay-discovery-v3` file records this checkpoint. Older v2 discovery files are preserved and cause a one-time rescan; spending ledgers and unfinished proof jobs are retained.

Unknown or pruned heights and malformed pages remain failures. A failed history or audit query retains its checkpoint while open transfers continue. A failed open-transfer scan stops the pass. The receipt API preserves the newest 100 owner records, its `truncated` flag and null creation dates only for genuinely pruned block history.

## Verification commands

Run `npm run typecheck`, `npm run test:services` and `npm run test:portal-contract` after installing the paired SDK and IDL. Service tests drive the actual relayer tick and signed transaction journal with controlled RPC boundaries. They are not validator membership evidence.

Run `npm test` for the program's isolated validator tests and `npm run test:sink` for a chain-produced proof after the node fixture gate passes. Go tests use the committed pins by default; local SDK development must explicitly select the existing `go.work` mechanism. Preserve their fresh test directories. Live headers, full refund recovery, cross-host services and populated mainnet-fork recovery remain separate gates.
