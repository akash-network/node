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
Governance enables them after the launch reserve is funded and reporters are
configured.

## State

Store key `sink`. Keys are ASCII unless noted.

| Key                         | Value                                              |
| --------------------------- | -------------------------------------------------- |
| `params`                    | `Params`                                           |
| `totals`                    | `Totals`                                           |
| `transfer/{id}`             | 77-byte transfer record, never rewritten           |
| `void/{id}`                 | 9-byte governance void order                       |
| `settlement/{id}`           | `Settlement`                                       |
| `account/{20-byte address}` | `AccountUsage`                                     |
| `open/{8-byte BE id}`       | `0x01`, index of transfers not minted or refunded  |

`{id}` is the transfer ID in decimal without leading zeros. IDs start at 1 and
are never reused.

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

Proofs use the standard ABCI query `/store/sink/key` with key `transfer/{id}`.
The merkle path is `["sink", "transfer/{id}"]`.

The `open/` index is derived. A deposit adds an entry; the first mint
confirmation or refund removes it. InitGenesis rebuilds it from settlements and
ExportGenesis omits it.

## Accounting

The sink module account balance always equals
`launch_reserve + pending_amount + minted_amount + voided_amount` in `uakt`.
`refunded_amount` is cumulative and excluded. Deposits, reserve funding and
refunds check this after each bank transfer, and InitGenesis checks it on import.

The launch reserve is exactly 1,000,000 AKT. It is funded once, from genesis
(`launch_reserve_source`) or an upgrade handler, never by a transaction.
Deposits cannot be enabled until it is funded.

## Messages

| Message              | Signer                     | Effect                                                           |
| -------------------- | -------------------------- | ---------------------------------------------------------------- |
| `MsgSink`            | depositor                  | Escrow `amount` for Solana destination `dest`, return the ID     |
| `MsgUpdateParams`    | governance                 | Replace all parameters                                           |
| `MsgRequestVoid`     | governance                 | Write a void order for an unsettled transfer                     |
| `MsgReportOutcome`   | current reporter           | Vote that a transfer was minted or voided on Solana              |
| `MsgVetoVoid`        | current reporter or gov    | Hold a void before its refund is paid                            |
| `MsgResolveTransfer` | governance                 | Restart a held transfer, or confirm a mint or a void             |
| `MsgFinalizeRefund`  | anyone                     | Pay an eligible refund to the original sender                    |
| `MsgGuardianPause`   | `params.pause_guardian`    | Turn deposits, refunds or both off                               |

`MsgSink` must be the only message in its transaction, signed by an ordinary
account. `dest_type` must be `1` (Solana). `dest` must be the canonical base58
encoding of 32 nonzero bytes, on the ed25519 curve unless it is in
`params.vault_allowlist`. Akash does not check the Solana account itself; the
Solana program does that when it mints. `MsgSink.ValidateBasic` runs the
stateless checks before fees are charged. The keeper enforces
`min_transfer <= amount <= max_transfer`, the per-account daily count, pending
count and daily amount limits, and checked uint64 arithmetic.

`MsgSinkResponse.id` is the decimal record ID. It is available from the
included transaction's message responses, not from the initial broadcast reply.
A `sink_deposit` event carries the same ID.

Reports carry the reporter epoch, the transfer round and an observation: the
outcome, a finalized Solana slot and the 64-byte transaction signature. They
are attestations, not proofs. A mint quorum closes the transfer. A void quorum
starts the challenge window, after which anyone can finalize the refund,
subject to `refunds_enabled` and `daily_refund_cap`. Conflicting votes or a
veto hold the transfer until governance resolves it. Minted and refunded are
terminal and cannot be reopened.

`MsgGuardianPause` can only clear `enabled` and `refunds_enabled`. Only
`MsgUpdateParams` can set them again or change the guardian. An empty
`pause_guardian` means there is none.

## Bank restrictions

The sink address is a blocked bank address, like other module accounts.
A send restriction rejects every transfer into or out of it except:

- the exact deposit authorized inside the `MsgSink` handler,
- the exact reserve funding authorized by `FundLaunchReserve`,
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
| `enabled`                      | false           | Requires the reserve and a reporter set             |
| `max_transfer`                 | 1,000 AKT       | At most `solana_daily_mint_cap`                     |
| `solana_daily_mint_cap`        | 10,000 AKT      | Must match the Solana program's daily cap           |
| `max_daily_per_account`        | 20              | Deposits per account per UTC day                    |
| `max_pending_per_account`      | 3               | Unsettled transfers per account                     |
| `vault_allowlist`              | empty           | Up to 16 recipients exempt from the curve check     |
| `reporters`                    | empty           | Up to 32 accounts                                   |
| `reporter_threshold`           | 0               | More than two thirds of `reporters`                 |
| `challenge_period_seconds`     | 86,400          | Positive                                            |
| `daily_refund_cap`             | 1,000 AKT       | At least `max_transfer`                             |
| `refunds_enabled`              | false           |                                                     |
| `min_transfer`                 | 0               | 0 means no minimum                                  |
| `max_daily_amount_per_account` | 0               | 0 means no cap; otherwise at least `min_transfer`   |
| `pause_guardian`               | empty           | 20- or 32-byte account                              |

Defaults are placeholders for development, not launch values. Changing the
reporter set or threshold increments `totals.reporter_epoch`, which invalidates
unfinished void authorizations from earlier epochs.

## Installation

An upgrade that adds this module must:

1. add the `sink` store,
2. call `PrepareInstall` before migrations, which turns any account already at
   the sink address into the module account (keeping its account number) and
   moves its whole balance to the community pool,
3. run migrations, which call InitGenesis with the default state,
4. call `FundLaunchReserve` with the reserve source, which fails the upgrade if
   the source holds less than 1,000,000 AKT.

Deposits stay disabled until governance enables them.
