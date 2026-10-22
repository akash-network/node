# AKT on Solana: Implementation Status

> Companion to [AEP-92](./README.md). Status as of 2026-10-20.

## Where the code is

All private until approved for publication.

| Part | Repository and branch |
| --- | --- |
| AKT program, ICS07 patches, relayer, reporters, monitor, portal, deployment tooling | `ovrclk/mintdock`, `solana`. Audit tag `audit-v1` on `039525e` |
| `x/sink` | `ovrclk/node`, `solana`. Split for the mainnet repository on `feat/sink` |
| Protobuf and generated clients | `ovrclk/chain-sdk`, `solana`. Split for the mainnet repository on `feat/sink` |

Detailed docs in `ovrclk/mintdock`: `docs/audit.md` (audit package), `docs/production-deployment.md`, `docs/deploy-program.md`, `docs/operators.md`, `docs/verifier-patches.md` and `docs/decisions.md` (open decisions with owners).

## Akash: `x/sink`

| Item | Status | Notes |
| --- | --- | --- |
| Deposits | Done | `MsgSink { sender, dest_type, dest, amount }`. The memo has no meaning |
| Sink address | Done | A blocked bank address. Plain sends to it fail |
| Deposit checks | Done | Enabled, single top-level message, ordinary signing account, canonical 32-byte destination that is on-curve or a listed vault, `uakt` amount between `min_transfer` and `max_transfer`, daily count, pending count and daily amount per sender |
| Transfer records | Done | 77 bytes at `transfer/{id}` in store `sink`, version byte first. Never changed |
| Void orders | Done | 9 bytes at `void/{id}`, written by `MsgRequestVoid` |
| Reporter voting | Done | Votes carry outcome, finalized slot and transaction signature. Reporter epochs and rounds, conflict hold, veto hold, governance `MsgResolveTransfer` |
| Refunds | Done | `MsgFinalizeRefund`, anyone can send. Needs a confirmed void in the current epoch, an ended challenge window, no hold, refunds enabled and room under the daily refund cap. Paid from the transfer's own locked AKT, never from the reserve |
| Launch reserve | Done | `InitGenesis` moves exactly 1,000,000 AKT from `launch_reserve_source`. A short balance fails the upgrade, which halts the chain at that height, so the proposal must check the balance first |
| Existing account at the sink address | Done | `PrepareInstall` adopts it as the module account and sends any balance to the community pool |
| Supply queries | Done | `State` returns totals. The module checks `balance = reserve + pending + minted + voided` |
| Parameters | Done | All parameters in the README, including `min_transfer`, `max_daily_amount_per_account` and `pause_guardian` |
| Pause guardian | Done | `MsgGuardianPause` turns deposits and/or refunds off, never on |
| Open-transfer index and query | Done | `OpenTransfers` pages through transfers that are not Minted or Refunded |
| Production upgrade handler | Open | Name, height and reserve source not chosen. A rehearsal handler exists for test chains |
| Amino names | Open | `sink/MsgSink` and `sink/MsgGuardianPause`. Decide the naming style before wallet integration |
| Wallet and Ledger signing | Open | Amino signing tested in code. Keplr and Ledger acceptance testing to do |

## Solana

### Light client

| Item | Status | Notes |
| --- | --- | --- |
| Own ICS07 deployment from `cosmos/ibc-contracts` | Done | Pinned at `dc78ade828b766b218fb6bcb5417fdaf8e553021` with four patches: signature batches, monotonic latest height, consensus pruning, open relaying |
| Permissionless relaying | Done | Granting `RELAYER_ROLE` to the ICS07 program's own ID admits any signer. Pruning needs a real member |
| Trusting period | Set per deployment | At most 14 days on mainnet. The team testnet uses 1 day |
| Access-manager admin as a time-locked multisig | Tooling done | The multisig itself is a launch decision |
| Checkpoint | Open | Ceremony: three people, three independent nodes, a post-upgrade height |

### AKT program

| Item | Status | Notes |
| --- | --- | --- |
| Genesis | Done | Legacy SPL mint, 6 decimals, no freeze authority, PDA mint authority, Metaplex metadata, 1,000,000 AKT to the treasury once, minting off |
| Proof upload | Done | Chunked proof buffers up to 4,096 bytes, owned by the submitter, rent returned on close |
| `mint_transfer` | Done | Checks the pinned client (not frozen, within the trusting period, clock drift), proves `transfer/{id}`, checks the recipient, then mints or voids. Writes one permanent status per record ID |
| Recipient check | Done | Wallet keys and listed vaults pass. Voids executables, token accounts, the zero key, the mint-authority PDA, other program-owned or off-curve keys, and a recipient whose AKT token account was reassigned to another owner |
| `void_transfer` | Done | Proves `void/{id}`. Works while minting is paused |
| Caps and pause | Done | Per-transfer maximum and daily cap on fixed UTC days. `set_bridge_enabled` |
| Rent | Done | Whoever submits a mint pays the token account and status rent, about 0.0035 SOL per mint |
| Pause guardian | Done | `guardian_pause` turns minting off without the time lock |
| Re-pin | Done | `repin_light_client` moves to another active ICS07 deployment at any time without reading the old one |
| Top-level only | Done | Mint and void reject CPI and allow only ComputeBudget instructions before them, so reporters always see the event |
| Events and errors | Done | Events for every admin and resolution action. Errors 7000 to 7024 |
| Audit | Frozen | Tag `audit-v1`. Reproducible builds give the same hashes on any machine |

## Relayers and operations

The relayer encodes headers with the pinned upstream Tendermint and Borsh types, pre-verifies signatures, uploads chunks, assembles the update and reclaims temporary accounts. It needs no IBC router (ICS26) on Solana.

| Component | Status | Notes |
| --- | --- | --- |
| Relayer | Done | Config-driven. Follows the light client pinned in the AKT program. Updates on demand and after one fifth of the trusting period when idle. Trims commits above 100 signatures to a quorum. Prunes expired consensus states. Checks Akash before paying, and keeps a spend ledger and per-transfer budget |
| Reporters | Done | Config-driven, each with its own nodes. Read status at finalized commitment, attribute events by invocation frame, vote, veto mismatched voids and send refunds when due. External signer supported |
| Monitor | Done | Supply reconciliation, bank equality, admin events, client age |
| Portal | Done | Connects Keplr and a Solana wallet and fills in the destination |
| Production hosting | Open | Separate hosts, each reporter on its own nodes, alerts delivered to a person |

## Test results

| Check | Result |
| --- | --- |
| ICS23 proofs of `x/sink` keys pass `verify_membership` | Yes, with a real `MsgSink` in the Akash app |
| Commits with unneeded signatures marked absent are accepted | Yes. The relayer uses this above 100 signatures |
| Header update cost | About 1.05M compute units to assemble a full 100-validator commit. About 0.0015 SOL of rent per verified height until pruned. Upload rent returns on cleanup |
| Other ways for funds to reach `x/sink` are rejected | Yes. The sink is a blocked address and only the module's own transfers pass |

## Open

- Production upgrade name, height and reserve source.
- Who holds the multisig, the guardians and the treasury.
- Reporter operators, threshold, challenge period and caps for mainnet.
- Checkpoint ceremony and maximum clock drift.
- Audits of the AKT program and ICS07 patches, and of `x/sink`.
- Token metadata hosting.
- Production hosting and on-call alerting.

`docs/decisions.md` in `ovrclk/mintdock` tracks each of these with an owner.
