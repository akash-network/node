# x/sink

`x/sink` locks AKT on Akash for minting on Solana. A deposit escrows `uakt` in
the sink module account and writes an immutable transfer record. A Solana
program verifies that record with an IAVL proof of this store and mints the
same amount. Reporters attest the Solana outcome back to Akash. A voided
transfer can be refunded to its original sender after a challenge window.

The protobuf API, Go types, validation and CLI live in chain-sdk under
`proto/node/akash/sink/v1`, `go/node/sink/v1` and `go/cli/sink_{tx,query}.go`.
This module holds the keeper, message and query servers, the bank send
restriction and the ante decorator.

Deposits and refunds are disabled by default and the reporter set is empty.
Governance enables them after reporters are configured. There is no launch
reserve: Solana supply starts at zero, and every SPL AKT is minted from a
proven transfer.

## State

Store key `sink`. State is kept in `collections` with two-byte prefixes, as in
x/bme. Integer keys are fixed-width big-endian, so iteration follows numeric
order.

| Prefix      | Key                                    | Value                                            |
| ----------- | -------------------------------------- | ------------------------------------------------ |
| `0x01 0x00` | none                                   | `Params`                                         |
| `0x01 0x01` | none                                   | `Totals`                                         |
| `0x02 0x00` | transfer ID, 8-byte BE                 | 77-byte transfer record, never rewritten         |
| `0x02 0x01` | transfer ID, 8-byte BE                 | 9-byte governance void order                     |
| `0x03 0x00` | transfer ID, 8-byte BE                 | `Settlement`                                     |
| `0x03 0x01` | transfer ID, 8-byte BE                 | empty; index of transfers not minted or refunded |
| `0x03 0x02` | challenge end, then ID, 8-byte BE each | empty; refund queue                              |
| `0x04 0x00` | 20-byte sender address                 | `AccountUsage`                                   |

IDs start at 1 and are never reused. Solana proves the transfer and void keys,
so their prefixes and encoding are fixed and published in the SDK as
`TransferKey` and `VoidKey`. For ID 1 the transfer key is
`02 00 00 00 00 00 00 00 00 01`.

The transfer record is what the Solana verifier checks, so its layout is fixed:

| Offset | Bytes | Value                               |
| ------ | ----- | ----------------------------------- |
| 0      | 1     | Version `1`                         |
| 1      | 8     | Transfer ID, little endian          |
| 9      | 20    | Akash sender address                |
| 29     | 32    | Solana recipient public key         |
| 61     | 8     | Amount in uakt, little endian       |
| 69     | 8     | Creation block height, little endian |

A void order is version `1` followed by the ID as a little-endian uint64.
Writing one never changes the transfer record or releases funds.

Proofs use the standard ABCI query `/store/sink/key` with the transfer key.
The merkle path is `["sink", TransferKey(id)]`, or `["sink", VoidKey(id)]` for a
void order.

The open index is derived. A deposit adds an entry; the first mint
confirmation or refund removes it. The refund queue holds voided transfers
without a hold. Both are rebuilt by InitGenesis from settlements and omitted by
ExportGenesis. InitGenesis also rejects refund or account-usage days later than
the genesis time.

## Accounting

The sink module account balance always equals
`pending_amount + minted_amount + voided_amount` in `uakt`.
`refunded_amount` is cumulative and excluded. Deposits and refunds check this
after each bank transfer, and InitGenesis checks it on import. A fresh install
holds nothing.

`Totals` field 2 and `GenesisState` field 3 held the removed launch reserve.
They stay reserved.

## Messages

| Message              | Signer                     | Effect                                                           |
| -------------------- | -------------------------- | ---------------------------------------------------------------- |
| `MsgSink`            | depositor                  | Escrow `amount` for Solana destination `dest`, return the ID     |
| `MsgUpdateParams`    | governance                 | Replace all parameters                                           |
| `MsgRequestVoid`     | governance                 | Write a void order for an unsettled transfer, or a held mint     |
| `MsgReportOutcome`   | current reporter           | Vote that a transfer was minted or voided on Solana              |
| `MsgVetoVoid`        | current reporter or gov    | Hold a void before its refund is paid                            |
| `MsgResolveTransfer` | governance                 | Restart a held transfer, or confirm a mint or a void             |
| `MsgFinalizeRefund`  | anyone                     | Pay an eligible refund to the original sender                    |
| `MsgGuardianPause`   | `params.pause_guardian`    | Turn deposits, refunds or both off                               |

`MsgSink` must be the only message in its transaction, signed by an ordinary
account. `dest_type` must be `1` (Solana). `dest` must be the canonical base58
encoding of 32 nonzero bytes: a canonical ed25519 point that is not of small
order, unless it is in `params.vault_allowlist`. Akash does not check the Solana account itself; the
Solana program does that when it mints. `MsgSink.ValidateBasic` runs the
stateless checks before fees are charged. The keeper enforces
`min_transfer <= amount <= max_transfer`, the per-account daily count, pending
count and daily amount limits, and checked uint64 arithmetic. A nonzero
`min_transfer` is required while deposits are enabled.

`MsgSinkResponse.id` is the decimal record ID. It is available from the
included transaction's message responses, not from the initial broadcast reply.
A `sink_deposit` event carries the same ID with the sender, amount, recipient
and height.

Reports carry the reporter epoch, the transfer round and an observation: the
outcome, a finalized Solana slot and the 64-byte transaction signature. They
are attestations, not proofs. A mint quorum closes the transfer. A void quorum
starts the challenge window, after which anyone can finalize the refund,
subject to `refunds_enabled` and `daily_refund_cap`. Refunds are paid oldest
first: only the head of the refund queue (earliest challenge end, then lowest
ID) can be paid, so a small refund cannot use up a day's cap ahead of an older,
larger one. A head from an earlier reporter epoch blocks the queue until a
current reporter reports on it. Conflicting votes or a veto hold the transfer
until governance resolves it.

Governance confirms a void only with a void order, so a proposal pairs
`MsgRequestVoid` with `MsgResolveTransfer`. Refunded transfers are terminal. A
Minted transfer is terminal unless it carries a conflict hold: governance can
then confirm a void, which moves it back to voided with a new challenge window
and restores its pending slot. This recovers a refund after a false mint quorum
voted before Solana resolved the transfer. The amount must fit the daily refund
cap.

`akash query sink open-transfers --held-only` reads every page and lists the
held transfers, for a proposal that removes a reporter and restarts them.

`MsgGuardianPause` can only clear `enabled` and `refunds_enabled`. Only
`MsgUpdateParams` can set them again or change the guardian. An empty
`pause_guardian` means there is none.

## Bank restrictions

The sink address is a blocked bank address, like other module accounts.
A send restriction rejects every transfer into or out of it except:

- the exact deposit authorized inside the `MsgSink` handler,
- the exact refund authorized by `FinalizeRefund`,
- the sweep of a preexisting balance by `PrepareInstall` before the store is
  initialized.

The ante decorator records the shape of the transaction so the handler can
reject a `MsgSink` that is nested in authz, contracts or a multi-message
transaction.

## Queries

| Query           | REST                                                 |
| --------------- | ---------------------------------------------------- |
| `Sink`          | `/akash/sink/v1/records/{id}`                        |
| `State`         | `/akash/sink/v1/state`                               |
| `Transfer`      | `/akash/sink/v1/transfers/{id}`                      |
| `Account`       | `/akash/sink/v1/accounts/{sender}`                   |
| `OpenTransfers` | `/akash/sink/v1/open_transfers?start_after=&limit=`  |

`Sink` takes the canonical decimal ID and returns a public status with the full
transfer state. Malformed IDs return `InvalidArgument`; unknown IDs return
`NotFound`. `transfer_pending` means unsettled, `completed` means the mint was
confirmed, and `void` means a confirmed void whether or not the refund has been
paid; `transfer.settlement` has the details. `validation_pending` and
`validated` are reserved.

`OpenTransfers` returns transfers that are neither minted nor refunded with IDs
above `start_after`, in ascending order. A limit of 0 means 100; larger limits
are capped at 500. Pass `next_start_after` to continue; 0 means no more.

## Parameters

| Parameter                      | Default         | Notes                                               |
| ------------------------------ | --------------- | --------------------------------------------------- |
| `enabled`                      | false           | Requires a reporter set and `min_transfer`          |
| `max_transfer`                 | 1,000,000 AKT   | At most `solana_daily_mint_cap`                     |
| `solana_daily_mint_cap`        | 10,000,000 AKT  | Must match the Solana program's daily cap           |
| `max_daily_per_account`        | 20              | Deposits per account per UTC day                    |
| `max_pending_per_account`      | 3               | Unsettled transfers per account                     |
| `vault_allowlist`              | empty           | Up to 16 recipients exempt from the curve check     |
| `reporters`                    | empty           | Up to 32 accounts                                   |
| `reporter_threshold`           | 0               | More than two thirds of `reporters`                 |
| `challenge_period_seconds`     | 259,200         | At least 72 hours; 1 s in builds with tag `sinkdev` |
| `daily_refund_cap`             | 1,000,000 AKT   | At least `max_transfer`; lowered, at least the largest open transfer |
| `refunds_enabled`              | false           |                                                     |
| `min_transfer`                 | 0               | Required while `enabled`; 0 only with deposits off   |
| `max_daily_amount_per_account` | 0               | 0 means no cap; otherwise at least `min_transfer`   |
| `pause_guardian`               | empty           | 20- or 32-byte account                              |

Defaults are placeholders for development, not launch values. Changing the
reporter set or threshold increments `totals.reporter_epoch`, which invalidates
unfinished void authorizations from earlier epochs.

## Installation

An upgrade that adds this module must:

1. add the `sink` store,
2. call `PrepareInstall` before migrations, which turns any account already at
   the sink address into the module account (keeping its account number),
   moves its whole balance to the community pool and deletes distribution
   withdraw addresses set to the sink address,
3. run migrations, which call InitGenesis with the default state,
4. check that the sink balance matches its totals, which are all zero.

The provisional handler does not read the plan info. It carries only the
standard cosmovisor `binaries` and `configs`.

Deposits stay disabled until governance enables them.
