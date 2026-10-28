# Header verification with 100 validators

The patched Tendermint client verifies equal-power 100-validator commits within 64 accounts and 1.4 million compute units. Both the live relayer and benchmark use the same uploader. The patch packs authenticated signature hashes into one account; Tendermint still checks quorum, validator hashes, trusted overlap, time and header binding. See [the patch design](signature-batches.md).

These are synthetic signed headers executed on the real Solana runtime. The [four-validator network](testnetify.md) separately exercises real Akash consensus and deposits. Neither test establishes mainnet readiness or performance for every validator transition.

## Reproduce

Complete [setup](setup.md) first. From the Solana repo:

```bash
npm run bench:headers
npm run bench:headers -- weighted
npm run test:receipts
npm run test:recovery
npm run test:misbehaviour
npm run test:expiry
```

Run these sequentially. Each starts its own validator on port 19999, preserves evidence under `.cache/scale/run-*`, and stops that validator on completion. Occupied ports fail before startup. Every run creates a separate explicit test checkpoint. It never resets a retained bridge checkpoint or sends public-chain transactions.

See [the test guide](reproduce.md#4-verifier-scale-recovery-and-consensus-failures) for expected result files and host verifier tests.

The base is `cosmos/ibc-contracts` commit `dc78ade828b766b218fb6bcb5417fdaf8e553021`, with the checked-in [verifier patches](verifier-patches.md). Setup verifies the revision, applies the patches idempotently, builds SBF and checks the generated IDLs. A changed source tree fails rather than being overwritten.

## Runtime results

Agave 2.3.13, at most 64 accounts, 1,232 serialized transaction bytes, 1.4M CU per transaction and a 256 KiB heap. The equal fixture assigns 10,000 voting power to each of 100 validators. The weighted fixture assigns 100,000 to 30 and 1,000 to 70. Weighted results do not describe actual Akash stake distribution.

| Signed commit                  | Header bytes | Chunks | Assembly CU | Result                           |
| ------------------------------ | -----------: | -----: | ----------: | -------------------------------- |
| Equal, 67/100                  |       20,910 |     24 |     927,256 | Accepted, 35 accounts            |
| Equal, 100/100                 |       24,078 |     27 |   1,052,058 | Accepted, 38 accounts            |
| Equal, 66/100                  |       20,814 |     24 |     918,380 | Quorum rejected; trust unchanged |
| Weighted, 21/100               |       16,494 |     19 |     536,992 | Accepted                         |
| Weighted, 100/100              |       24,078 |     27 |     866,516 | Accepted                         |
| Weighted, 20/100               |       16,398 |     19 |     517,738 | Quorum rejected; trust unchanged |
| Weighted, 21 with altered root |       16,494 |     19 |     417,908 | Rejected; trust unchanged        |

Evidence: `.cache/scale/run-1BcQsE/results.json` and `.cache/scale/run-3Gd1mx/results.json`. CU varies with fixture bytes and account derivation. Manifests record program and patch hashes so historical results stay attributable after changes.

Commits keep all 100 entries, using `Absent` for omitted votes. All present signatures are preverified into a submitter/session batch before assembly. Up to 100 signatures the live driver submits full commits, avoiding dependence on a favorable stake distribution; larger commits are trimmed to a quorum (see below). Actual full-set rotation, key-type assumptions and production throughput still need rehearsal.

## Sets above 100 validators

`npx tsx local/scale.ts wide N` runs the equal fixture with N validators, from
101 to 256, at quorum, full commit and one below quorum. Above 128 present
signatures the uploader uses two batches. These are measurements, not gates;
only the below-quorum case must never authenticate. Runs on 2026-10-18 and
2026-10-19 with ICS07 `c6310749…`:

| Signed commit  | Header bytes | Chunks | Accounts | Assembly CU | Result                   |
| -------------- | -----------: | -----: | -------: | ----------: | ------------------------ |
| Equal, 87/129  |       26,861 |     30 |       41 |   1,169,679 | Accepted                 |
| Equal, 129/129 |       30,893 |     35 |       47 |   1,400,000 | Compute exhausted        |
| Equal, 86/129  |       26,765 |     30 |       41 |   1,175,078 | Quorum rejected          |
| Equal, 101/150 |       31,124 |     35 |       46 |   1,341,943 | Accepted                 |
| Equal, 150/150 |       35,828 |     40 |       52 |   1,400,000 | Compute exhausted        |
| Equal, 100/150 |       31,028 |     35 |       46 |   1,360,382 | Quorum rejected          |
| Equal, 117/175 |       36,135 |     41 |       52 |   1,400,000 | Compute exhausted        |
| Equal, 175/175 |       41,703 |     47 |       59 |   1,310,360 | Heap exhausted (256 KiB) |
| Equal, 134/200 |       41,242 |     46 |       58 |   1,230,843 | Heap exhausted           |
| Equal, 200/200 |       47,578 |     53 |       65 |           0 | Refused before sending   |

Evidence: `.cache/scale/run-mdXm3r`, `run-oheZzi`, `run-gFyPPw` and
`run-JWFO0X`. No commit with more than 128 present signatures authenticated, so
the verifier's 256-signature limit is not reachable: compute or heap runs out
first. A full commit fits at 100 validators and fails at 129; the limit between
is unmeasured. Quorum-only commits, with the other votes `Absent`, fit up to at
least 150 equal-power validators. Each upload that failed at assembly still
cost its payer 0.0014 to 0.002 SOL in fees; upload rent returns on cleanup and
lookup-table rent after its cooldown. The relayer now trims commits above
`maxHeaderSignatures` (default 100) to trusted signers above the trust level
plus the highest-power new signers above two thirds, marks the rest `Absent`,
and treats compute or heap exhaustion as unsupported rather than retrying it.

## Fees and rent

For equal-power updates, excluding bootstrap and without priority fees:

| Item                           | 67 signatures, SOL | 100 signatures, SOL |
| ------------------------------ | -----------------: | ------------------: |
| Transaction fees               |           0.000830 |            0.001175 |
| Consensus-state rent           |         0.00150336 |          0.00150336 |
| Temporary upload rent returned |          0.2090088 |          0.23139216 |
| Lookup rent awaiting cooldown  |         0.00907584 |          0.00974400 |

Consensus-state rent stays locked until the state expires; [pruning](verifier-patches.md#consensus-state-pruning) then returns it. The benchmark checks exact payer deltas. Its returned temporary rent excludes lookup tables until they actually close. The live relayer journals a delayed-close queue and reclaims one table per tick after the deactivation slot leaves SlotHashes. The recovery test waits through this real cooldown and checks closure.

## Recovery and adversarial checks

The uploader journals the exact header, identity and signed transaction before submission. Restart resolves any outstanding transaction before constructing another. Lookup changes, assembly and cleanup reach finalized commitment before becoming terminal in the journal. A per-payer OS lock serializes uploads and lookup cleanup; process death releases it. Matching authenticated state from another operator overrides a rejected local assembly. An unknown outcome retains its intent and upload accounts.

Recovery tests interrupt lookup extension submission, signature/chunk writes, assembly, cleanup and deactivation. They also race two authorized operators, revoke the first operator's update role after the second authenticates the target, and retry the rejected operator after restart. Expired-client checks stop new uploads without replacing the checkpoint; already completed jobs can still clean up. Conflicting valid headers freeze the client, and frozen clients reject further updates.

The SBF writer tests reject missing or invalid precompiles, substituted digest/message bytes, external instruction offsets, foreign writers and over-capacity inserts. Duplicates are idempotent, and the owner can clean up after losing its relayer role. Host parser tests reject malformed batch accounts and forged legacy receipts.

Each run retains its manifest, fixtures, transaction logs, costs, results and validator logs. Development keys also live there; share selected result files rather than the whole directory.
