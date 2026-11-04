# Sink implementation decisions

## Canonical state and recovery

Typed `RecordData` and `SettlementData` maps are authoritative; the void authorization lives inside `SettlementData`. Node's key codecs own the proven key bytes, and the SDK's protobuf messages define the value bytes. The chronological codec only reorders the same validated ID components for discovery. This avoids a second proof format and avoids synthesizing missing settlements.

The single `put` writer maintains settlement, financial totals, status membership, dispute membership, and refund membership. It changes a queue key only when membership or deadline/scope changes. Deposit separately creates the immutable record and charges send usage. Refund separately charges refund usage and pays the original owner. The public command's cache includes all of these changes.

Genesis rebuilds derived indexes with the settlement writer's index step and does not replay accounting; `GenesisState.Validate` recomputes totals and import rejects a mismatch. Export rejects orphaned canonical state and invalid aggregate accounting. Auth and bank restore accounts and balances first. Recovery never manufactures bank funds or performs lifecycle transitions.

The allocator is transient because committed height supplies restart identity. Import requires a strictly later initial height and rejects duplicate positions across owners. Persistent allocation fields would duplicate the chain's committed position and could disagree with it.

## Independent financial status and disputes

Amount and count describe the same status population, including held transfers. Minted and paid refunded transfers can still be disputed. Keeping disputes separate makes a false terminal report visible without pretending that a payment was reversed.

Each vote names its immutable reporter policy. Terminal and governance authorization keep their original evidence when reporters rotate. Unfinished quorum authorization refreshes lazily on the next report. Policy updates therefore write one policy record instead of scanning all settlements. A settlement write loads only the versions its authorization, veto, and votes name, so its cost does not grow with retained history. Genesis import and export still validate every retained policy and require the current one. Scoped queues prevent stale quorum entries from blocking current payments.

SDK `Params.ValidateBasic` and `GenesisState.Validate` check shape only: they accept a reporter set whose quorum is not above two thirds and a genesis without the current policy. The keeper enforces both. `SetParams` validates the set before storing a policy, because policy history is immutable, settlement writes revalidate the policy they embed, and export revalidates every stored policy. Genesis validation requires the policy at `policy_sequence` to exist and match params, because `SetParams` and every reporter command load it. A governance proposal with an invalid set therefore passes submission and fails on execution without changing state.

A governance confirmation names one outcome through its message type. The evidence contains only a slot and signature. `expected_round` prevents a passed proposal from applying to a different settlement revision. A refund of a genuinely invalid Solana recipient may have no governance order, so resolving a paid refund does not require one.

## Amount policy and bank authority

Sending and refunding track separate usage against the current number of the existing `x/epochs` schedule named `day`. Its duration and start come from epochs genesis. The challenge Duration remains separate from that schedule. A refund does not return send capacity. Account count quotas were deliberately removed; amount quotas do not provide equivalent protection against outstanding record counts. A refund-cap reduction walks the transfer-pending, voided, and refund-pending status buckets at governance time so it cannot strand the queue head. Terminal history is not read.

The existing bank `BlockedAddrs` protects public transfers to Sink. Bank messages dispatched through authz or wasm retain the recipient check. Sink adds no bank send restriction or deposit, refund, or sweep context capabilities. Ante still binds the exact sole top-level request. The keeper owns account eligibility, operation authorization, and cached accounting. Post-send balance checks detect redirected deposits and refunds and preserve minted backing. Installation still clears pre-upgrade balances and withdrawal addresses.

## Discovery and compatibility

Owner-first IDs make owner and status/owner queries bounded. An independent chronological index serves global discovery so a newly active owner cannot appear behind an old cursor. `Transfers` pages with opaque server cursors and only `Disputes` uses native pagination; both read the historical SDK context selected by BaseApp. Clients pin a height during a page walk and retain the last chronological identity and the key that fetched its page between walks.

Genesis carries no schema marker. Import rejects usage stamps ahead of the native epoch but cannot otherwise tell earlier fixed-day counters from native epoch numbers, so snapshots from fixed-day builds must not be imported. The module has no in-place migration from an existing Sink installation. Existing testnets, programs, mints, and their histories require fresh paired state rather than reinterpretation.
