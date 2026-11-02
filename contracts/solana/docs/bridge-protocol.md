# Solana bridge prototype and Akash handoff

This implements the light-client design. Genesis creates the mint with zero supply. All issuance requires a successful CPI to the configured Tendermint client's `verify_membership`. There is no relayer-authorized mint instruction.

## Trust and activation

1. Initialize genesis with the program's upgrade authority.
2. Deploy the pinned Tendermint client and access manager. Initialize a reviewed Akash checkpoint and grant the required relayer roles.
3. The AKT program's current upgrade authority calls `configure_bridge(chain_id, policy)`. This fixes the light-client deployment and chain ID in the bridge PDA. Only `repin_light_client` can change them. Configuration validates client ownership, addresses, chain ID, frozen status and expiry. It leaves minting off.
4. The same authority calls `set_bridge_enabled(true)` after the Akash module, backing, proof compatibility and operational checks are complete.

`set_bridge_enabled(false)` pauses mint attempts, including automatic recipient voids. A proven governance void remains available while minting is paused. `set_mint_policy` changes limits and the vault allowlist, but never resets that day's issued amount. These calls rely on the upgrade authority's signature. The production multisig and time lock are external deployment work, not implemented by this prototype.

The upgrade authority can also name a pause guardian with `set_pause_guardian(Some(key))`, rotate it, or clear it with `None`. The key lives in the `["guardian"]` PDA. `guardian_pause` lets that key disable minting without waiting out the authority's time lock. It has the same effect as `set_bridge_enabled(false)`. The guardian cannot enable minting or change any other setting.

The configured light-client program and its initial checkpoint are trust decisions. Program upgrades can change verification or minting rules. The local suite assigns upgrade authority to the development wallet; it is not a production governance setup.

### Re-pinning the light client

`repin_light_client(chain_id)` points the bridge at another compatible light-client deployment. The upgrade authority can call it at any time: to recover from an expired or frozen client, to move to a patched verifier, or to hand verification to a deployment run by other operators. The ICS07 program has a single `client` PDA and no recovery instruction, so an expired or frozen client always needs a new deployment initialized with a reviewed checkpoint.

- The call passes only the new program, client state and latest consensus. The pinned client is never read, so a broken, closed or incompatible deployment cannot block a re-pin.
- The new deployment gets the same checks as `configure_bridge`: executable program, account owners, PDAs, chain ID, not frozen, active, and a trusting period shorter than the unbonding period.
- Policy and the day's mint usage carry over. Only the program and chain ID change.
- It emits `LightClientRepinned { previous_program, program, previous_chain_id, chain_id }`. Monitors should alert on every one.

Re-pinning has the same trust as a program upgrade. The same authority can replace the AKT program, so in production both go through the time-locked multisig. It cannot mint a record twice: transfer statuses are keyed only by record ID, so a record that already has a status cannot be minted or voided through any client. The new checkpoint is a fresh trust decision. It must come from the same chain, with the same x/sink state and record IDs. Never re-pin to a reset or forked chain. A different chain ID is valid only for an Akash chain-ID upgrade that keeps x/sink state.

### Relayers

Each ICS07 deployment names its access manager in its `app_state` account. That access manager's `RELAYER_ROLE` (role 1) decides who may upload headers and submit updates and misbehaviour. Its admin grants and revokes the role for individual keys. Relayers cannot forge headers: every update needs a commit signed by the trusted validator set.

With the [open relaying patch](verifier-patches.md), granting `RELAYER_ROLE` to the ICS07 program's own ID lets any signer submit header uploads, updates and misbehaviour. Pruning consensus states still needs a real member. Revoking that grant closes relaying again.

On the test network, `local/bridge-admin.ts` runs these calls. It reads every address from chain: the light client from the bridge PDA and the access manager from the client's `app_state`. Each command requires `--genesis` with the Solana genesis hash and refuses any other cluster.

| Command                                               | Effect                                                           |
| ----------------------------------------------------- | ---------------------------------------------------------------- |
| `show`                                                | Pin, policy, guardian, client status, open relaying and relayers |
| `set-guardian --guardian KEY`, `clear-guardian`       | Name, rotate or clear the pause guardian                         |
| `pause [--guardian-keypair PATH]`                     | Pause minting as the guardian                                    |
| `repin --program ID [--chain-id ID]`                  | Re-pin to another active deployment and print the event          |
| `relayers`                                            | List `RELAYER_ROLE` members                                      |
| `grant-relayer --key KEY`, `revoke-relayer --key KEY` | Add or remove one relayer                                        |
| `open-relaying`, `close-relaying`                     | Grant or revoke `RELAYER_ROLE` for the light-client program ID   |

The relayer commands act on the pinned client unless `--program` names another deployment, for example one being prepared for a re-pin. A key already in the requested state is reported without a transaction.

## x/sink encoding

The node's `x/sink` module writes this schema, and `npm run test:sink` verifies a real committed module record through the Solana programs. It is not deployed on Akash mainnet. The core transfer value stays immutable when bookkeeping changes. See [the x/sink README](../../../x/sink/README.md) for deposit handling.

The proof path has exactly two segments. The first is the UTF-8 store name `sink`. The second is the key in that store. The Solana program constructs both segments itself.

| Item           | Bytes                                                                                     |
| -------------- | ----------------------------------------------------------------------------------------- |
| Transfer key   | 10 bytes: prefix `0x02 0x00`, then the ID as a big-endian u64                             |
| Void-order key | 10 bytes: prefix `0x02 0x01`, then the ID as a big-endian u64                             |
| ID             | Nonzero unsigned 64-bit integer, globally unique within the pinned chain and never reused |
| Proof          | Protobuf `ibc.core.commitment.v1.MerkleProof`, IAVL proof followed by multistore proof    |

For ID 1 the transfer key is `02 00 00 00 00 00 00 00 00 01` and the void-order key is `02 01 00 00 00 00 00 00 00 01`. Keys hold the ID big-endian; the values below hold it little-endian.

A transfer value is exactly 77 bytes, serialized in this order. Integers use little-endian encoding; arrays have no length prefix.

| Offset | Length | Field                                                          |
| ------ | ------ | -------------------------------------------------------------- |
| 0      | 1      | Version, `1`                                                   |
| 1      | 8      | Transfer ID                                                    |
| 9      | 20     | Original Akash sender's address bytes, without bech32 encoding |
| 29     | 32     | Solana recipient public key                                    |
| 61     | 8      | Positive `uakt` amount                                         |
| 69     | 8      | Positive Akash creation height                                 |

A void-order value is nine bytes: version `1`, then the transfer ID as little-endian u64. Only Akash governance should write that key. Proving this value authorizes a permanent Solana void, not an Akash refund by itself.

The program's `wire` unit tests pin this layout and the key formats with hand-written byte vectors.

The instruction's proof height selects the client's verified consensus state. The record's creation height must not exceed it. A relayer must match ABCI state height H to the verified header H+1 whose app hash commits that state. The real-app test queries committed state at height 2 and uses a deliberately trusted checkpoint at height 3. The SDK rejects proof queries at heights <= 1. The persistent local driver authenticates later signed headers before submitting proofs against them.

## Submission and outcomes

`create_proof_buffer(nonce, expected_len)` creates a submitter-owned buffer, up to 4,096 bytes. `append_proof(offset, bytes)` accepts only sequential writes by that submitter. Complete buffers can be used by anyone to submit a proof. `close_proof_buffer` refunds rent only to the submitting owner. Closing a buffer never closes a transfer status.

`mint_transfer(record, proof_height)` checks the pinned deployment, client and consensus PDAs, account owners, chain ID, frozen status, and expiry based on the **latest verified height**. It then proves the exact transfer key and encoded value against the requested verified height. Older verified roots remain usable while the client is active. A future latest timestamp beyond the client's clock-drift allowance is rejected.

After verification:

- A nonzero on-curve, System Program-owned address passes as a wallet. It need not exist yet. This format check does not prove that a user controls the key.
- An allowlisted vault may be off-curve or owned by another program. Executables, token-program-owned accounts and the bridge's mint-authority PDA are rejected even when allowlisted.
- An existing recipient ATA must be an initialized AKT account owned by the recipient. Legacy SPL Token lets a holder reassign its ATA's owner, after which ATA creation would fail on every retry. Such a recipient is invalid.
- Invalid recipients produce a permanent `Voided / InvalidRecipient` status and mint nothing. No ATA is created.
- Valid recipients receive tokens in their derived legacy SPL ATA. The transaction payer covers ATA and status rent.
- Pause, per-transfer limits, daily-cap exhaustion, invalid proofs and CPI failures leave no transfer status or issuance. They can be retried.

`void_transfer(id, proof_height)` verifies the governance void-order key and records `Voided / Governance`. Mint and void both initialize the same PDA, derived from `["transfer", id_le_u64]`. Neither can run after the other succeeds. There is no status close/reset instruction and no timeout refund path.

Both instructions must be top-level transaction instructions. A call through another program fails with `CpiNotAllowed`. Anchor's event parser skips events emitted inside a CPI, so reporters would find no `TransferResolved` evidence and the transfer would stay pending on Akash. Only compute budget instructions may precede them in the transaction; anything else fails with `PrecedingInstruction`. Earlier instructions' logs count toward the 10 KB log limit and could truncate the event. Instructions after them are allowed because the event is already logged.

## Events and errors

Every state change an operator monitors emits an Anchor event, logged as a `Program data:` line:

| Event                 | Emitted by                       | Fields                                         |
| --------------------- | -------------------------------- | ---------------------------------------------- |
| `LaunchInitialized`   | `initialize`                     | mint, initializer                              |
| `TokenMetadataSet`    | `set_token_metadata`             | mint, metadata account, URI                    |
| `BridgeConfigured`    | `configure_bridge`               | light-client program, chain ID, policy         |
| `BridgeEnabledSet`    | `set_bridge_enabled`             | enabled, signing authority                     |
| `MintPolicySet`       | `set_mint_policy`                | previous and new policy                        |
| `PauseGuardianSet`    | `set_pause_guardian`             | previous and new guardian, `null` when cleared |
| `GuardianPaused`      | `guardian_pause`                 | guardian                                       |
| `LightClientRepinned` | `repin_light_client`             | previous and new program and chain ID          |
| `TransferResolved`    | `mint_transfer`, `void_transfer` | record ID, proof height, outcome               |

In production a multisig calls the admin instructions through CPI. Anchor's TypeScript `EventParser` ignores events logged inside a CPI, so monitors must attribute `Program data:` lines to the AKT invocation frame. Resolutions are always top-level.

AKT errors use codes 7000 to 7023 (`programs/akt-solana/src/errors.rs`). A failed membership check fails the AKT instruction with the ICS07 error code, from 6000, and the program logs name the ICS07 program as the source. The two ranges never overlap.

## Caps

The mint cap counts base units in fixed UTC days using the Solana clock. It is not a rolling 24-hour limit. Vaults are subject to the same caps. Lowering the per-transfer maximum can stall pending deposits until governance changes policy or proves a void.

## What local tests establish

The suite builds the actual upstream programs and deploys them to a local validator. A Go fixture generator uses IAVL v1.2.2 to create a tree of transfer and void records, with an ICS23 multistore proof. It independently verifies each generated proof with the Go ICS23 library. The test explicitly initializes the client with that generated root as a trusted checkpoint.

This exercises real proof verification, SPL issuance, recipient checks, status exclusivity, caps, buffer ownership and expired-client rejection. It also covers the reassigned-ATA void, the pause guardian, and mint and void rejected through upstream's CPI proxy or after another instruction. It decodes every event, and `tests/idl.test.ts` checks that the IDL keeps the interface the test network runs. It re-pins an active client to a second copy of the pinned verifier, whose embedded program ID the test wrapper rewrites, and back. It rejects a frozen third copy, other invalid replacements and a non-authority signer. After expiry it closes the original deployment, re-pins away from it, mints the transfer that expiry blocked and rejects replay of resolved records. The separate `npm run test:sink` suite executes a signed deposit in the real Akash app with x/sink and verifies its committed multistore proof on Solana. Both suites use explicitly trusted checkpoints. They do **not** exercise Akash consensus signature verification, relayed header updates, misbehaviour submission, reporter refunds, or mainnet funding.

The upstream build emits stack-size diagnostics in curve and misbehaviour conversion functions. Membership verification and initialization execute in this suite. Separate runtime tests now cover 100-validator updates and conflicting-header freezing. Any reachable stack violations still need resolution before production use.

The [persistent local stack](local-stack.md) separately runs actual CometBFT and Solana validators. After its initial trusted checkpoint, it verifies signed header updates through the upstream client, rejects an altered app root, mints against real state proofs and broadcasts reporter confirmations. The live demo also executes an invalid-recipient void and the native challenge/refund path. The four-validator profile also verifies live validator transitions, quorum outages and governance voids. Synthetic equal-power 100-validator headers pass through the shared patched uploader, and conflicting signed headers freeze the client in a separate runtime test.

## Remaining integration work

- Deploy independent reporter services and implement production upgrade activation. Native x/sink confirmations, guarded refunds, deposits and real module proof compatibility are implemented locally.
- Rehearse actual full-size validator rotations, sustained relay throughput and production misbehaviour monitoring. Synthetic 100-validator commits, four-validator transitions and durable lookup-table transport already pass.
- Define and rehearse the production checkpoint and re-pin ceremonies under the time lock. `repin_light_client` works at any time; actual conflicting signed headers already freeze the local client.
- Choose production trust periods, caps, allowlists, authorities and upgrade time locks.
- Harden production relayer and reporter operation, including indexed monitoring, protected keys, independent nodes and archival accounting. Local durable header uploads and supply/burn reconciliation are implemented. Metadata and launch tooling remain.
- Audit this implementation and the pinned client deployment before a public release.
