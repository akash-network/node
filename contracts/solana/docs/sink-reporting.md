# Sink reporting and refunds

Implementation checklist:

- [x] Trace the existing bank restriction, immutable record store, genesis and query boundaries.
- [x] Define settlement states, reporter epochs and the sole refund authorization path.
- [x] Implement native messages, accounting, queries and genesis validation.
- [x] Exercise signed reports, refunds, disputes, rotations and failed bank sends.
- [x] Recheck the real Akash-proof-to-Solana mint test and update the plan.

## State and ownership

Settlement leaves the transfer records and void orders that Solana proves unchanged; [the bridge protocol](bridge-protocol.md#xsink-encoding) gives their keys and values. New settlement state, keyed by transfer ID, contains a status, hold reason, report round, reporter-set snapshot, votes, optional governance observation and challenge deadline. An open-transfer index indexes transfers that are pending or voided; the first mint or refund removes the entry.

Statuses are pending, minted, voided and refunded. Pending and voided both consume the sender's pending slot. Minted and refunded are terminal. A hold records a veto or conflicting report without changing which accounting bucket owns the escrow. Conflicting reports after a terminal outcome create an alertable hold but cannot reopen settlement or reverse a payment.

This deliberately gives terminal outcomes priority over later contradictory reports. A falsely confirmed mint cannot be refunded through these messages, even by governance; recovery would require a separately reviewed upgrade. Independent observation and reconciliation remain necessary.

Each report includes transfer ID, current reporter epoch, transfer round, outcome, finalized Solana slot and a 64-byte transaction signature. Cosmos authenticates the reporter through its normal signed transaction. These observations are attestations, not Solana consensus proofs. The reporter service must read its own replaying Solana node at finalized commitment and verify the configured program, status, amount and recipient.

Params contain at most 32 unique reporter accounts and a threshold strictly greater than two thirds, a challenge duration of at least 72 hours (1 second in `sinkdev` builds), daily refund cap, separate refund enable switch and an optional pause guardian. Empty reporter configuration is allowed only while deposits are disabled. Local tests choose their own keys and timings; there are no production identities or policy choices in the defaults.

Local defaults are a 72-hour challenge and a 1,000,000 AKT daily refund cap, with deposits and refunds off and no reporters. The signed-transaction tests use 3-of-4 local accounts, short challenge windows and small amounts. Neither configuration is a launch recommendation.

Changing reporter membership or threshold increments the global epoch. Unfinished void authorizations from earlier epochs cannot refund. On a new report the old votes are replaced and any old void authorization returns to pending, while veto/conflict holds survive until governance resolves them. A governance restart increments the transfer round, so reports from the discarded round cannot be reused. Changing the challenge duration cannot shorten a window already started; changing caps never resets daily spending.

## Native operations

- `ReportOutcome`: accepts one vote per reporter per outcome. Identical repeats have no effect. Opposite votes, including equivocation by one reporter, freeze progress. A mint quorum closes bookkeeping; a void quorum starts the challenge window.
- `VetoVoid`: one current reporter can stop a refund during its challenge window. Governance can also veto a void before payment. A veto stays until governance resolves it.
- `ResolveTransfer`: governance can reject the reports and restart, confirm a mint, or confirm an observed Solana void. Confirmations require slot/signature evidence. A governance-confirmed void starts a fresh challenge window and remains subject to pause, veto, reporter rotation and the daily cap. Neither a timeout nor an Akash void order authorizes this operation as a refund.
- `GuardianPause`: `params.pause_guardian` can turn refunds, deposits or both off at once, without a vote. It cannot enable either; only governance can. Pending voids keep their windows and pay after governance re-enables refunds, subject to the cap.
- `FinalizeRefund`: any signed caller can execute an eligible refund after the deadline. Payment always goes to the original sender, for exactly that transfer's amount. The bank restriction permits outgoing sink funds only with a private context authorization installed by this function. A cache makes the bank transfer and settlement/counter changes atomic. Repeating a completed refund pays nothing. Refunds are paid oldest first: voided transfers without a hold form a queue ordered by challenge end, then ID, and only the head can be paid, so a small refund cannot use up a day's cap ahead of an older one. The payment must reach the original sender.

There is no unbounded end-block scan. Reporters can find unsettled work with the paged `OpenTransfers` query, which lists pending and voided transfers, held or not, in ID order, instead of reading every transfer. Reporters running with broadcast enabled submit `FinalizeRefund` for the head of the refund queue once it is eligible, and pay that transaction's gas; the next refund waits for the following pass. The Solana relayer takes no part. Any signed account, including the original sender, can also call it. At cap exhaustion reporters retry on a later UTC day. This does not affect eligibility for a single-reporter veto during the original window.

Governance may change future limits, but a previously accepted transfer can become temporarily unrefundable if its amount exceeds a later lowered cap. Restoration of sufficient cap is required; no caller or governance resolution can bypass it. Lowering a challenge duration never changes an existing deadline.

## Verification

Use the [native test command](reproduce.md#2-component-and-proof-checks) after setup so the node tests use the matching local SDK. The tests execute signed Cosmos transactions and the real bank keeper, inject a bank rejection to verify refund rollback, and export/import mixed settlement state. `npm run test:sink` in the Solana repo confirms the immutable proof bytes still mint successfully through the actual Solana programs.

Most native state-machine tests use fixture observations. The [local reporter](reporter.md) now reads finalized Solana status, checks deployment identity and transfer contents, and prepares or signs reports, vetoes and eligible refunds. `npm run test:sink` observes an actual finalized mint and returns three signed TypeScript transactions for the native Akash app to verify and execute. The regular Solana suite also observes actual automatic and governance voids. Live multi-node operation and authenticated header updates remain untested; the cross-chain test still uses a trusted checkpoint.

## Accounting

The exact local bank invariant is `pending + minted + voided = sink uakt balance`. Refunded is a separate cumulative total. Pending counts include unresolved voids and holds, and decrease only once on mint confirmation or successful refund. Deposit totals and cumulative refund accounting use checked uint64 arithmetic.

Cross-chain reconciliation still needs the observation service: `escrow behind actual Solana mints = Solana supply + cumulative SPL burns`, with report lag and pending transfers shown separately. A false reporter threshold can lie about a void; the challenge window, independent veto, cap and monitoring bound that trust assumption but do not remove it.
