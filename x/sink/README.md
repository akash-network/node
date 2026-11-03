# Sink

Sink escrows native AKT for minting on Solana. A deposit creates an immutable proof record and an explicit settlement. Mint confirmation keeps the backing in escrow. A refund pays the original Akash owner after authorization, a challenge interval, and queue and amount checks.

## Authority and bank accounting

`handler.NewServer` unwraps a message, calls the keeper command once, and returns its response. Each command calls SDK `ValidateBasic` before state checks and owns one cache. Its bank changes, records, counters, indexes, allocation, and events commit together.

The ante decorator records the exact sole top-level `MsgSink` request. `Deposit` matches the sender, destination type, destination, denomination, and amount against that private marker. It also requires an ordinary account with a public key. Nested authz and contract calls receive no matching request marker.

Sink remains in the bank's `BlockedAddrs`. Public bank send and multisend handlers reject that recipient, including bank messages dispatched through authz or contracts. Wasm attached-fund transfers also check blocked recipients. Sink registers no additional bank send restriction. Its deposit, refund, and pre-installation sweep use the bank's privileged module helpers after their own validation. Calling those helpers directly is trusted module code, not a public transaction route.

After a bank deposit, the keeper verifies exact escrow backing. After a refund, it verifies both escrow backing and the original owner's balance increase. A bank error or redirected deposit or refund cannot leave successful Sink state. Installation retains its cached sweep, empty-balance check, and cleanup of withdrawal addresses that pointed at Sink before it became blocked.

Totals contain one amount and count for each financial status:

- `TRANSFER_PENDING` has no mint or refund authorization.
- `MINTED` retains the backing in escrow.
- `VOIDED` retains a governance order awaiting Solana confirmation.
- `REFUND_PENDING` has authorization and a challenge deadline, but has not paid.
- `REFUNDED` records a paid refund and sits outside escrow.

Holds do not change these populations. The module's complete bank balance must equal pending, minted, voided, and refund-pending amounts in `uakt`, with no other denominations. A new governance void order moves a pending attempt to `VOIDED`; it never authorizes payment. Totals contain only five Coin/uint64 count pairs. The flat totals event additionally includes normalized refund consumption for the current native epoch.

## Reports and governance

Reporter policies are immutable history. A separate current-policy item selects the active version; reads load only referenced policies. The quorum is the ceiling of `reporters_threshold * member_count`, computed with exact decimals. A membership or effective-threshold change increments the version. Reordering members or changing the ratio without changing the effective threshold does not.

Reports require current membership, current policy version, and exact settlement round. Votes retain their own policy version. An active attempt accepts one vote per reporter for its first outcome. At quorum the certificate freezes. At most one opposite witness is retained; conflict freezes further reports. Same-outcome reports cannot grow a completed certificate. Evidence validation checks signature and slot format; it does not establish Solana truth or finality.

A current report refreshes an unheld unpaid reporter attempt after rotation: replace the embedded policy, clear stale attempt votes and the deadline, and return to transfer-pending or voided according to its permanent order. Old-policy reports fail before clearing. Held, minted, refunded, and governance-authorized refund-pending settlements retain their authorization and evidence. New-policy reports can add an opposite witness without erasing the retained certificate.

`RequestVoid` requires a matching `expected_round` for a new order. An authenticated retry preserves the original receipt without changing the round. The receipt stores request height and round and survives every subsequent transition. `RestartTransfer`, `ConfirmMint`, and `ConfirmVoid` require `expected_round`, then advance the round. Restart applies only before payment or mint confirmation. Confirm-mint never reverses a paid refund. Confirm-void requires an immutable void order for unpaid transfers or conflict-held minted transfers. Those transfers must fit the refund cap. Confirm-void on an already-refunded transfer resolves the dispute without another payment or a new order. Confirmations replace votes and holds with governance evidence.

A reporter can veto a refund before its deadline. Governance can veto before payment. Conflict takes precedence over veto. Changing policy preserves the veto signer, reason, and its membership version. A zero veto version means governance; import and writes authenticate the configured authority.

## Limits and refund order

Send and refund usage read `CurrentEpoch` from the existing `x/epochs` record named `day`. Sink owns no timer or epoch hooks. The schedule's duration and start belong to epochs genesis and can differ from a UTC day. After counting starts, the native timer advances only strictly after the current endpoint, once per block. Downtime can therefore produce successive allowance resets across recovery blocks; unused allowances do not accumulate.

Deposits charge lifetime account usage, account epoch usage, and global send epoch usage. Refunds never restore sending capacity. Queries project stale usage to the native number at the queried height without writes; export preserves raw stamps. Missing or malformed native state fails. Valid unstarted schedules expose epoch zero, while deposits and refund payments reject it. Future stamps and positive epoch-zero consumption fail. Epochs initializes before Sink and already runs first in BeginBlock. Sink still resets its independent per-block sequence in BeginBlock.

The challenge Duration remains independent. Its integer Unix-second deadline is the checked ceiling of block time plus Duration, including fractional carry. Authorization receives the full interval even just before a quota boundary, and existing deadlines survive parameter changes. Immediate Solana mints and their separate amount caps remain unchanged.

The account epoch cap may be zero to disable that limit; account usage is still tracked. The other caps and the minimum are positive `uakt` Coins. Only individual transfer amounts must fit `uint64`. There are no account count quotas. Changing a cap preserves consumption, including when an account cap is re-enabled. Lowering the refund cap scans unpaid transfers and rejects a cap that strands one. Existing challenge deadlines remain unchanged when the challenge parameter changes.

Refund queue keys contain authorization scope, challenge deadline, and ID. Scope zero is governance; a nonzero scope is a reporter policy version. Payment selects the oldest entry across governance and the current policy. Old reporter scopes cannot block current refunds. Queue membership alone does not imply maturity, remaining epoch capacity, or enabled refunds.

The destination vault allowlist exempts configured Solana recipients from the curve check. It does not bypass amount, authorization, or accounting rules.

## Identity, storage, and queries

A transfer ID is `owner/height/sequence`, with a canonical nonzero 20-byte owner and positive height and sequence. A transient allocator issues 1 first and increments globally within each block. `BeginBlocker` resets it to zero. Failed deposits roll back allocation. Simulation uses the proposed next block height because committed state has already cleared its transient allocator; BaseApp discards the simulated writes.

The SDK owns native codec fixtures. Transfer keys are `02 00 || 14 || owner20 || BE64(height) || BE64(sequence)` (39 bytes); values are native protobuf `RecordData`, including the recipient and SDK Coin. Canonical settlement keys are `03 01 || status || 14 || owner20 || BE64(height) || BE64(sequence)` (40 bytes); values are native protobuf `SettlementData`. Status exists only in the settlement key. Its permanent tag-9 void authorization is the sealing receipt. Both maps use `codec.CollValue`. Proof nonces are the full SHA-256 of the proven key; paths are `[sink, key]`. There is no separate void map or status index.

`Transfer` probes all five canonical status keys and requires exactly one settlement. Missing, duplicate, or malformed canonical state fails. `Transfers` ranges the canonical status buckets; owner-only pages merge five ordered iterators by height and sequence. Without filters it orders by height, sequence, then owner. `Disputes` includes held terminal transfers. `Account` and `State` expose normalized epoch usage; State also returns the eligible refund queue head.

Pagination uses native `PageRequest` and `PageResponse`, with default limit 100 and maximum 500. Offsets, count-total requests, and limits above 500 are rejected. Cursors are the selected index's complete key without its collection prefix. Malformed cursors and keys outside the selected owner, status, or index are rejected.

Clients must use the same `x-cosmos-block-height` on every page of a snapshot. A pruned height fails through BaseApp; the keeper does not fall back to current state. Durable discovery retains the last processed chronological identity even when `next_key` is empty. On reconnect, seek that identity at the new height, skip the already-processed record, and continue. A new owner cannot sort behind an old chronological identity. Filtered listings do not serve as global discovery cursors.

## Recovery and compatibility

Consensus version remains 2. Genesis `schema_version` is 4. `GenesisRecord` contains ID, immutable data, complete settlement and its separate status; account usage appears once per owner. `current_policy_version` is explicit. Imports require current and all referenced policies, with uniqueness and consistency but no contiguity requirement. Schema 3 and older shapes fail; there is no in-place conversion or silent evidence truncation. The v2.2.0 upgrade installs Sink only when the input module version map does not contain Sink. It preserves its shared cache around installation, module migration, escrow repair, real Wasm migration, and final checks.

The upgrade moves any preexisting balance at the deterministic Sink address to the community pool. Its governance proposal must disclose that disposition. Installation preserves the existing account number and clears withdrawal addresses pointing to Sink so rewards return to their delegators.

Genesis runs after auth and bank. Import validates the complete snapshot, requires an empty Sink store, writes canonical state in one cache, rebuilds indexes through the settlement writer, recomputes totals, and checks exact bank restoration. Import does not sweep, mint, refund, rotate policy, or normalize epochs. Policies, votes, governance evidence, vetoes, void orders, paid states, and empty account usage entries survive export and import.

Every imported transfer and void-request height must precede `max(1, InitChain height)`. Duplicate block-height/sequence pairs are rejected across owners as well. Operators must retain an initial height above the exported chain height. `akash export` keeps the original `genesis_time`. Operators must also set it at or after the export's last block time, and preserve the native epoch record; import rejects usage beyond its current number. A database restart resumes after the committed height with a fresh transient sequence. Prior Sink schemas require a fresh deployment domain; their keys and history are not rewritten in place.

Defaults are development policy. Deposits and refunds are disabled. Reporter membership is empty, the ratio is 0.75, and the challenge is 72 hours. Release and sinkdev challenge floors remain in SDK validation. Solana's independent mint amount caps do not couple submission timing to Akash send epochs.

Recovery must preserve the native `day` schedule and current number before Sink initialization. A new installation accepts empty zero-stamped usage at an existing nonzero epoch. Populated import preserves raw current and stale consumption and requires the first executable height to exceed every stored transfer height. Restart preserves both modules. Sink does not tick the native timer during import, export, or queries.
