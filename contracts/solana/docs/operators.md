# Running a relayer

The relayer carries Akash state to Solana. It uploads signed Akash headers to
the ICS07 light client and submits ICS23 proofs to `mint_transfer` and
`void_transfer`. It holds no trusted power: the light client verifies every
header and the AKT program verifies every proof. Anyone can run one against any
deployment with their own nodes and payer key. Several can run at once. Each
transfer resolves once on Solana. A relayer that loses the race pays only its
proof-upload fees; its resolution fails simulation and is not sent.

The relayer is `local/relay.ts`. With `--config` it serves an existing
deployment; without it, it runs the [local stack](local-stack.md).

## Requirements

**Workspace.** Use `contracts/solana` from the node repository and follow [setup](setup.md). A
relayer host needs the packed chain SDK and `npm ci`, the AKT IDL from
`npm run build`, the patched verifier from `npm run setup:light-client` and the
header encoder from `npm run build:header-encoder`. It needs no Akash or Solana
validator.

**Akash node.** A CometBFT RPC endpoint you run or trust:

- `abci_query` with `prove=true` on the `sink` store, at every height from the
  light client's latest verified height to now. Each relay proves at that
  height, which can be up to `refreshFraction` of the trusting period old
  (about 5 hours on the test network). Keep at least one full trusting period of state;
  `pruning = "nothing"` is simplest.
- `commit` and `validators` at the client's latest height plus one and at the
  newest block.
- Not catching up, with its latest block younger than `maxAgeSeconds`.
- x/sink `OpenTransfers` is optional; see [pending transfers](#pending-transfers).

**Solana RPC.** HTTP JSON-RPC only; the relayer polls and opens no WebSocket. It
uses `getAccountInfo`, `getMultipleAccounts`, `getProgramAccounts` on the ICS07
program (pruning), `simulateTransaction` through preflight, `sendTransaction`,
`getSignatureStatuses`, `getTransaction`, `getLatestBlockhash` and address
lookup tables. A dishonest RPC cannot make the programs accept a false header
or proof, but it can waste fees. Use your own node or a provider you trust.

**Payer.** A Solana CLI keypair file, mode 0600, owned by the relayer user.
Give each relayer its own key. Fund it as described under
[costs](#costs-and-funding).

**State directory.** One per deployment and payer. It holds header-upload
journals, `header-uploads/payer.lock` and the spend ledger. A restart resumes an
interrupted upload from it. Never point two running relayers at one directory.

## Configure

Copy [`relayer/config.example.json`](../relayer/config.example.json). Relative
paths resolve against the config file's directory. Unknown fields are rejected.

| Field                    | Required | Meaning                                                                        |
| ------------------------ | -------- | ------------------------------------------------------------------------------ |
| `akashRpc`               | yes      | CometBFT RPC URL                                                               |
| `akashChainId`           | yes      | Akash chain; the node and the bridge account must both match                   |
| `solanaRpc`              | yes      | Solana JSON-RPC URL; no credentials in the URL                                 |
| `solanaGenesisHash`      | yes      | Checked at startup; get it from a trusted source, not the RPC you configure    |
| `programId`              | yes      | AKT program                                                                    |
| `payerKeyFile`           | yes      | Owner-only keypair file                                                        |
| `stateDir`               | yes      | Journals and spend ledger                                                      |
| `lightClientProgram`     | no       | Pin. Unset follows the bridge account                                          |
| `maxLamportsPerTransfer` | no       | [Spend limit](#spend-limit), default 5,000,000                                 |
| `maxHeaderSignatures`    | no       | [Large commits](#large-validator-sets) above this are trimmed, default 100     |
| `refreshFraction`        | no       | Update an idle client after this share of its trusting period, default 1/5     |
| `retryMaxSeconds`        | no       | Longest backoff for a failed transfer, default 900                             |
| `pruneIntervalSeconds`   | no       | Consensus-state pruning interval, default 600                                  |
| `maxAgeSeconds`          | no       | Oldest acceptable Akash block, default 60                                      |
| `pollSeconds`            | no       | Pause between passes, default 3                                                |
| `headerEncoder`          | no       | Encoder binary, default `tools/header-encoder/target/debug/akt-header-encoder` |

Skipping trust across a long interval needs the old validator set to hold more
than a third of the new voting power. Lower `refreshFraction` if the set changes
quickly.

## Run

```bash
npm run relayer -- --config /etc/akt-relayer/config.json
```

This runs `tsx local/relay.ts --watch --config PATH`. Drop `--watch` for one
pass. Run it under a supervisor that restarts it. Each line of output is JSON;
alerts go to stderr.

## Light-client discovery

Governance can [re-pin](bridge-protocol.md#re-pinning-the-light-client) the
bridge to another light client at any time, so the relayer never uses fixed
verifier addresses. Every pass it reads:

| Account                | Seeds                                  | Holds                                          |
| ---------------------- | -------------------------------------- | ---------------------------------------------- |
| Bridge                 | `["bridge"]` under the AKT program     | `light_client_program`, `chain_id`             |
| ICS07 client state     | `["client"]` under the light client    | Chain, trust parameters, latest height, frozen |
| ICS07 consensus states | `["consensus_state", height LE u64]`   | Timestamp and app root per verified height     |
| ICS07 app state        | `["app_state"]` under the light client | `am_state.access_manager`                      |
| Access manager state   | `["access_manager"]` under that        | `RELAYER_ROLE` members                         |

- A changed program logs `light-client-changed` with both IDs and the relayer
  follows it. A header upload in flight for the old client moves to
  `header-uploads/abandoned-*.json`; its temporary accounts keep their rent
  under the old program.
- A bridge that accepts a chain other than `akashChainId` logs
  `bridge-chain-mismatch` and the relayer pays nothing.
- With `lightClientProgram` set, a different program logs
  `light-client-mismatch` and the relayer pays nothing until the bridge or the
  pin changes. It keeps polling and repeats the alert every 15 minutes.

The monitor, health check and reporters follow the same account.

## Update rights

Mints and voids are permissionless. Header updates and misbehaviour need
update rights, logged as `relayer-access` at startup and on change:

| `access` | How                                                                       | Updates headers | Prunes |
| -------- | ------------------------------------------------------------------------- | --------------- | ------ |
| `member` | The access-manager admin granted `RELAYER_ROLE` to the payer              | yes             | yes    |
| `open`   | `RELAYER_ROLE` lists the ICS07 program's own ID, the open relaying marker | yes             | no     |
| `none`   | Neither                                                                   | no              | no     |

With `none`, the relayer relays only transfers the latest verified height
already proves, and logs `relay-header-blocked` when an update is needed. It
does not resume a stored upload. [Relayers](bridge-protocol.md#relayers)
describes the role and the test network's `grant-relayer` and `open-relaying`
commands; [verifier patches](verifier-patches.md) covers the program change.

## Large validator sets

Assembly runs out of compute or heap above about 100 signatures, and nothing
above 128 fits one update. When a commit has more than `maxHeaderSignatures`
votes, the relayer keeps the highest-power signers until they hold more than
2/3 of the new set's power and, when the update skips heights, more than the
client's trust level of the trusted set's power, each with 1% to spare where
possible. The other votes become `Absent`; the verifier needs no more. It logs
`header-signatures-trimmed` with the counts.

If no subset of at most 128 signatures passes, or assembly still exhausts
compute or heap, the header is unsupported: the relayer logs
`header-unsupported` and tries again only after `retryMaxSeconds`, since the
same validator set fails the same way.

## Costs and funding

Costs at the 5,000-lamport base fee, without priority fees. A proof under 500
bytes takes four transactions: create, append, resolve and close. Measured on
a local stack on 2026-10-19:

| Action                        | Lamports                    | Notes                                                     |
| ----------------------------- | --------------------------- | --------------------------------------------------------- |
| Mint to a wallet without ATA  | 3,458,240 measured          | Status 1,398,960 and ATA 2,039,280 rent, four fees        |
| Mint to an existing ATA, void | 1,418,960 measured          | Status rent and fees                                      |
| Proof buffer                  | about 29,800,000, temporary | Returned when it closes                                   |
| Header update                 | 1,503,360 plus fees         | Consensus-state rent; chunk, batch and lookup rent return |
| Pruning                       | returns 1,503,360 per state | To the pruning payer, whoever paid                        |

Updates happen only when a pending transfer needs a newer proof height, or
when the client has aged `refreshFraction` of its trusting period. Hold a few
days of spend plus 0.05 SOL for temporary accounts. The relayer does not watch
its own balance; alert on it.

## Spend limit

The relayer records the lamports it pays per transfer: every fee, plus the
rent its resolving transaction locks. Proof-buffer rent is not counted: each
attempt closes its buffer, and the next attempt closes one a failure left.
Header updates serve all transfers and are not charged.

When a transfer's total exceeds `maxLamportsPerTransfer`, the relayer stops
paying for it and logs `relay-budget-exhausted` with the total and attempt
count, then every 15 minutes. It still closes a leftover proof buffer, which
only returns rent. The default 5,000,000 lamports is about 1.4 times a mint to a
new ATA, which leaves room for dozens of failed attempts.

The ledger is `<stateDir>/relay-budget-<genesis>-<programId>.json`, so restarts
keep it. Investigate the transfer's `relay-retry` errors first, then reset:

```bash
npx tsx local/relay.ts --config /etc/akt-relayer/config.json --reset-budget 42
npx tsx local/relay.ts --config /etc/akt-relayer/config.json --reset-budget all
```

The running relayer reads the ledger every pass. Resolved transfers leave it
automatically.

## Checks before paying

For each transfer without a Solana status, the relayer:

1. Waits without paying while the bridge is paused or the per-transfer maximum
   or daily cap applies (`relay-wait`).
2. Never relays a record Akash shows as voided or refunded without a Solana
   status (`relay-unbacked-settlement`).
3. Right before the first paid transaction, re-reads the transfer at the newest
   Akash height. It skips if the transfer became voided or refunded, or its void
   order changed (`relay-final-check`).
4. Simulates every transaction before sending it, so a failing one costs
   nothing.

`mint_transfer` stays permissionless, so these checks protect only honest
relayers. The reconciler reports unbacked records.

## Pending transfers

Nodes with x/sink `OpenTransfers` list every non-terminal transfer: pending,
voided or held. The relayer reads Solana statuses for those only. Older nodes
get a cursor scan of Solana statuses from the lowest unresolved record ID; it
restarts at 1 with each process. `relay-index` logs the mode in use. In
`OpenTransfers` mode a refunded record without a Solana status is terminal and
not listed, so only the reconciler reports it.

## Alerts

Route these to a person.

| Event                                                 | Meaning                                            | Action                                           |
| ----------------------------------------------------- | -------------------------------------------------- | ------------------------------------------------ |
| `light-client-changed`                                | Governance re-pinned the bridge                    | Confirm the proposal; check `relayer-access`     |
| `light-client-mismatch`                               | Bridge differs from the pin; paying stopped        | Verify the re-pin, then update or remove the pin |
| `bridge-chain-mismatch`                               | Bridge accepts another Akash chain; paying stopped | Check `akashChainId` and the deployment          |
| `relayer-access` with `none`, `relay-header-blocked`  | Payer cannot update the client                     | Request the role, or rely on another updater     |
| `relay-budget-exhausted`                              | Spend limit reached for one transfer               | Read its `relay-retry` errors, fix, reset        |
| `relay-final-check`                                   | Akash changed between planning and paying          | Review the transfer; usually a governance action |
| `relay-unbacked-settlement`                           | Akash void or refund without a Solana status       | Investigate the reporters or governance          |
| repeated `relay-retry`, `header-retry`                | Transfers or updates keep failing                  | Check RPCs, payer balance and client status      |
| `header-upload-abandoned`                             | Re-pin during an upload                            | None; the new client gets a fresh upload         |
| `header-unsupported`                                  | No commit subset fits one update                   | The client will expire; plan a verifier change   |
| `consensus-prune-failed`, `proof-buffer-close-failed` | Rent recovery failed                               | Retried later; check the RPC if it persists      |

Watch the light client's age as well. The relayer stops at expiry with
`light client expired`; recovery is a governance re-pin.
