# Compact signature verification

Individual signature receipts exceeded the account and compute limits for an equal-power 100-validator update. The patched client stores authenticated signature hashes in one compact batch per relayer and upload session. Both 67/100 and 100/100 now pass on the real Solana runtime; 66/100 reaches a quorum rejection within the compute limit. See [measured results](header-scaling.md).

## Design decision

Three independent candidates and a separate review converged on direct preverification into a compact account. The selected design uses bounded, sorted hashes validated once by the signature verifier. It incorporates explicit initialization and durable upload identity from the receipt candidate and strict parser failure handling from the verifier candidate. Candidate sketches and the review are retained in `.cache/design/`; decisions are logged in `.cache/live-stack-decisions.tsv`.

The batch is an Anchor account with version, submitter, 32-byte session, bump and at most 128 SHA256 digests. A digest covers the exact Ed25519 public key, canonical vote bytes and signature. Its PDA uses `sig_batch`, submitter and session. Initialize explicitly; append only after the existing Ed25519 instruction-introspection check returns true. Duplicate insertion succeeds without mutation. A full batch rejects a new digest. Cleanup requires its submitter and returns rent only to that signer. Creating and filling a batch passes the relaying role check, which [open relaying](verifier-patches.md#open-relaying) extends to any signer; batches stay keyed by submitter.

The shared verifier authenticates owner, discriminator, supported version, exact allocation, PDA, bounded vector length and sorted uniqueness before copying at most 256 hashes across two batches. It binary-searches those hashes before trying legacy receipts or software verification. Invalid recognized batches fail verification. Legacy receipts also gain owner, discriminator and length checks.

Tendermint continues to check validator-set hashes, quorum, trusted-set overlap, times and signed-header binding. Batch membership only proves the exact signature was verified. The existing assembly and membership CPI ABIs remain unchanged. New account and instructions are additive. All source changes are captured in a reproducible patch against the pinned upstream revision.

The relayer stores the exact header and session before uploading. A per-payer OS lock serializes uploads and rent reclamation, and releases after process death. Signed transaction intents are durable before submission. Lookup modifications, assembly outcomes and temporary-account cleanup must finalize. On restart the uploader resolves pending transactions and checks on-chain receipts, target consensus and cleanup state. Matching finalized consensus from a competing operator counts as completion even if the local assembly was rejected. It never makes a new checkpoint to bypass expiry. Lookup-table rent remains a separate liability until closure is permitted.

We rejected copying legacy receipt accounts because it retains shared account collisions and extra rent/transactions. A larger header buffer alone still leaves too many signature accounts. A multi-transaction consensus verifier would change much more security-sensitive logic. No account or compute limit is raised.

## Signature limit

A header can use up to 256 preverified signatures, the verifier's two-batch
limit. The uploader puts signatures 1 to 128 in the session batch and the rest
in a second batch whose session is derived from the first. Single-batch upload
journals keep their session. A commit with more than 256 present signatures,
more than 255 chunks or more than 64 assembly accounts is rejected before any
transaction. The result carries an `unsupportedHeader` error that names the
limit. The relayer then drops that packet instead of retrying it forever; later
packets fail the same way until the validator set shrinks or the patch grows.

Akash mainnet has 100 active validators. Commits above 128 signatures exhaust
compute or heap before the batch limit matters: 129/129 runs out of compute and
134/200 out of heap, so a set above 100 needs quorum-only commits; see
[sets above 100 validators](header-scaling.md#sets-above-100-validators). The
100-signature commit uses 1.05M of 1.4M CU. `npx tsx local/scale.ts split` uploads a 100-signature commit with 64
signatures per batch, so assembly reads two batches, and checks that a
257-signature packet spends nothing. On 2026-10-18 the two-batch assembly used
1,079,492 CU and 39 accounts, against 1,050,586 CU with one batch
(`.cache/scale/run-PNJVFK`, `run-bLNX0Z`).

## Verification gates

- Equal-power 67/100 and 100/100 must authenticate under the original runtime limits; 66/100 must reach a quorum rejection rather than exhaust compute.
- The actual precompile must reject invalid signatures and substitution attempts. Missing precompiles, mismatched hashes, foreign writers, duplicate inserts and capacity overflow must leave correct state.
- Malformed account layout, owner, discriminator, PDA, version and counts must fail before any hash is trusted.
- Interrupted uploads, assembly and cleanup must resume without duplicate issuance or lost deposit refunds.
- Live Akash deposits, mint/void proofs, reporter settlement and restart must continue through the shared uploader.
- Conflicting-header and expiry tests must preserve the original consensus safety rules.
