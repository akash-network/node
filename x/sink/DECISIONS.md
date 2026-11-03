# Sink implementation decisions

## Canonical state and recovery

Typed `RecordData`, `SettlementData`, and `VoidOrder` maps are authoritative. The SDK codecs own proven key and value bytes. The chronological codec only reorders the validated SDK ID bytes for discovery. This avoids a second proof format and avoids synthesizing missing settlements.

The single `put` writer maintains settlement, financial totals, status membership, dispute membership, and refund membership. It changes a queue key only when membership or deadline/scope changes. Deposit separately creates the immutable record and charges send usage. Refund separately charges refund usage and pays the original owner. The public command's cache includes all of these changes.

Genesis uses the same writer to rebuild derived indexes and recompute totals. Export rejects orphaned canonical state and invalid aggregate accounting. Auth and bank restore accounts and balances first. Recovery never manufactures bank funds or performs lifecycle transitions.

The allocator is transient because committed height supplies restart identity. Import requires a strictly later initial height and rejects duplicate positions across owners. Persistent allocation fields would duplicate the chain's committed position and could disagree with it.

## Independent financial status and disputes

Amount and count describe the same status population, including held transfers. Minted and paid refunded transfers can still be disputed. Keeping disputes separate makes a false terminal report visible without pretending that a payment was reversed.

Each vote names its immutable reporter policy. Terminal and governance authorization keep their original evidence when reporters rotate. Unfinished quorum authorization refreshes lazily on the next report. Policy updates therefore write one policy record instead of scanning all settlements. A settlement write loads only the versions its authorization and votes name, so its cost does not grow with retained history. Genesis import and export still validate the complete contiguous history. Scoped queues prevent stale quorum entries from blocking current payments.

A governance confirmation names one outcome through its message type. The evidence contains only a slot and signature. `expected_round` prevents a passed proposal from applying to a different settlement revision. A refund of a genuinely invalid Solana recipient may have no governance order, so resolving a paid refund does not require one.

## Amount policy and bank authority

Sending and refunding track separate usage against the current number of the existing `x/epochs` schedule named `day`. Its duration and start come from epochs genesis. The challenge Duration remains separate from that schedule. A refund does not return send capacity. Account count quotas were deliberately removed; amount quotas do not provide equivalent protection against outstanding record counts. A refund-cap reduction walks the transfer-pending and refund-pending status buckets at governance time so it cannot strand the queue head. Terminal history is not read.

The existing bank `BlockedAddrs` protects public transfers to Sink. Bank messages dispatched through authz or wasm retain the recipient check. Sink adds no bank send restriction or deposit, refund, or sweep context capabilities. Ante still binds the exact sole top-level request. The keeper owns account eligibility, operation authorization, and cached accounting. Post-send balance checks detect redirected deposits and refunds and preserve minted backing. Installation still clears pre-upgrade balances and withdrawal addresses.

## Discovery and compatibility

Owner-first IDs make owner and status/owner queries bounded. An independent chronological index serves global discovery so a newly active owner cannot appear behind an old cursor. Native pagination uses the historical SDK context selected by BaseApp. Clients pin a height during a page walk and retain the last chronological identity between walks.

Genesis schema marker 3 distinguishes native epoch counters from the earlier fixed-day counters. The module has no in-place migration from an existing Sink installation. Existing testnets, programs, mints, and their histories require fresh paired state rather than reinterpretation.
