# AKT Solana audit reference

This reference describes the native protobuf proof contract used with Sink schema 4. The earlier `audit-v1` artifact hashes and validator results do not certify this program. A deployment needs a matching node, SDK archive, program, IDL, services and portal, with their commits and artifact hashes recorded together.

[Bridge protocol](bridge-protocol.md) specifies the keys and proven values. [Launch and recovery](sink-migration-v2.md) specifies the service journals, budgets and recovery rules. [Production deployment](production-deployment.md) covers authorities and operational acceptance.

## Scope and trust

The program verifies membership using the pinned `tendermint-light-client-membership` library inside AKT. It reads the authenticated ICS07 client and consensus accounts. It does not call the verifier's membership instruction through CPI. ICS07 still owns header verification, consensus roots, freezing and pruning.

The program's audit scope includes `programs/akt-solana`, its membership dependencies in `Cargo.lock`, the generated ICS07 account types, and the patch series applied by `scripts/apply-light-client-patches.sh`. The upstream client, access manager, Anchor, Solana runtime, SPL Token and Metaplex remain dependencies to review. Akash, reporters and the relayer have separate code and trust boundaries.

| Actor                                        | Authority                                                                                                                             |
| -------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| AKT upgrade authority                        | Replaces the program, configures the bridge and mint policy, changes the light-client pin, sets the guardian and controls metadata.   |
| ICS07 and access-manager upgrade authorities | Control the code and trust configuration that produce the consensus roots AKT accepts.                                                |
| Pause guardian                               | Disables minting. It cannot enable minting or change policy.                                                                          |
| Relayers                                     | Submit headers and proofs and pay fees and rent. Their honest operation affects liveness; every resolution still needs a valid proof. |
| Reporters                                    | Observe finalized Solana outcomes and authorize Akash settlement under its reporter policy. They have no Solana mint authority.       |
| Akash governance                             | Orders a void in canonical settlement state and can resolve Akash disputes using asserted Solana evidence.                            |

Upgrade authorities are trusted for issuance. A malicious AKT upgrade can bypass caps, and a malicious verifier or checkpoint can authenticate false state. Production authority controls are specified in [production deployment](production-deployment.md#authorities-and-keys).

## Source map

Paths beginning with `src/` are relative to `programs/akt-solana`.

| Source                                                              | Responsibility                                                                                |
| ------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| `src/lib.rs`                                                        | Entrypoints and the SBF allocator.                                                            |
| `src/constants.rs`                                                  | PDA seeds, token parameters and proof limits.                                                 |
| `src/state.rs`                                                      | Accounts, mint-cap accounting, proof-buffer sizing and policy validation.                     |
| `src/wire.rs`                                                       | Composite IDs, canonical record encoding, status keys and full-key nonces.                    |
| `src/authorization.rs`                                              | Bounded parsing of the permanent void authorization in native settlement bytes.               |
| `src/light_client.rs`                                               | Authenticated client and consensus accounts, time checks and both ICS23 membership levels.    |
| `src/instructions/proof_buffer.rs`                                  | Creation, bounded growth, sequential upload and rent reclamation.                             |
| `src/instructions/mint_transfer.rs`                                 | Record membership, recipient checks, caps and issuance or recipient void.                     |
| `src/instructions/void_transfer.rs`                                 | Settlement membership and governance void.                                                    |
| `src/instructions/resolution.rs`                                    | Invocation restrictions and permanent outcome storage.                                        |
| `src/instructions/upgrade_authority.rs`                             | Upgrade-authority check shared by admin instructions.                                         |
| `src/instructions/bridge_admin.rs`                                  | Bridge configuration, policy, pause switch and re-pinning.                                    |
| `src/instructions/pause_guardian.rs`                                | Guardian assignment and pause.                                                                |
| `src/instructions/genesis.rs`, `src/instructions/token_metadata.rs` | Zero-supply initialization and metadata.                                                      |
| `tests/idl.test.ts`                                                 | The instruction, account, event and error interface.                                          |
| `tests/bridge.ts`                                                   | Validator scenarios, including proof failures, recipient rules, pause, replay and re-pinning. |

## Proofs and identity

A transfer ID contains a nonzero 20-byte owner, positive int64 height and positive uint64 per-block sequence. The permanent status PDA uses `transfer`, owner20, LE64 height and LE64 sequence. Mint and void initialize that same account, and no instruction closes it. Re-pinning does not create a new outcome namespace.

The mint key is `0200 || 14 || owner20 || BE64(height) || BE64(sequence)`. The program constructs the expected native protobuf `RecordData` from the recipient and amount. Its Coin denomination is `uakt`; the positive amount fits uint64. There is no custom 41-byte record or record-version argument.

The governance key is `0301 || status || 14 || owner20 || BE64(height) || BE64(sequence)`. A void proves the entire native protobuf `SettlementData` value, including its permanent `void_authorization`. Status is in the key. The program rejects ambiguous or malformed authorization fields and enforces `deposit_height <= requested_height < proof_height` and `0 < requested_round <= settlement_round`.

Both paths authenticate the store path `["sink", key]` against the root in the pinned consensus account. Header H proves state at H-1, so the proof height must be strictly greater than the deposit height. The program checks the pinned executable program, account ownership, PDA addresses, discriminators, chain ID, frozen state, latest height, trusting period and clock drift before membership verification.

The program assumes Akash never rewrites immutable transfer data or reuses an ID. Akash import validation rejects record heights at or above the restarted chain's initial height. A reset chain or unrelated fork needs a fresh program namespace, not a re-pin that reuses identities.

## Instructions and storage

The generated IDL is the complete interface, including account order, mutability and error codes. The principal lifecycle operations are listed below.

| Instruction                            | Effect                                                                                                                                            |
| -------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| `initialize`                           | Creates the mint and launch configuration once, under the upgrade authority. Supply starts at zero and bridge minting is disabled.                |
| `configure_bridge`                     | Pins an active client and chain ID, and stores the mint policy.                                                                                   |
| `set_bridge_enabled`, `guardian_pause` | Control issuance. Only the upgrade authority can enable it.                                                                                       |
| `set_mint_policy`                      | Changes the independent Solana per-transfer and daily caps and vault list without erasing consumed capacity.                                      |
| `repin_light_client`                   | Checks and selects a new active client without reading the old deployment. Permanent resolution accounts remain.                                  |
| `create_proof_buffer`                  | Creates a submitter-owned buffer for 1 to 16,384 proof bytes. The first allocation is at most 10,240 bytes.                                       |
| `grow_proof_buffer`                    | Funds and allocates the remaining capacity. The current maximum needs at most one growth.                                                         |
| `append_proof`                         | Appends at most 500 bytes at the exact next offset within the declared length.                                                                    |
| `close_proof_buffer`                   | Returns the buffer's rent to its submitter.                                                                                                       |
| `mint_transfer`                        | Proves the native record, then mints to its valid legacy ATA or records an invalid-recipient void. Cap or proof failures roll back the operation. |
| `void_transfer`                        | Proves the canonical settlement's permanent governance order and records a void, including while issuance is paused.                              |

A proof buffer uses `proof`, submitter32 and all 32 bytes of `SHA256(proven_key)` as seeds. Each status key has a different nonce. The account needs an 80-byte header plus the declared proof length, up to 16,464 bytes. Only the submitter grows, appends or closes it. Another payer may use a complete buffer, but membership and nonce checks still bind its contents to the operation.

Upload and resolution transactions request a 256 KiB heap and the configured compute limit. Full-capacity uploads also need the larger heap. Refundable proof-buffer rent is temporary liquidity. The permanent status account's rent and transaction fees are lasting costs. Service budget accounting and crash recovery are described in [launch and recovery](sink-migration-v2.md#buffer-funding-and-recovery).

## Invariants

| Invariant                                                   | Enforcement and checks                                                                                                                                                          |
| ----------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Genesis cannot mint or repeat.                              | `genesis.rs` initializes the mint and config and has no mint CPI. Genesis validator tests check supply and authority.                                                           |
| Each ID resolves at most once.                              | Mint and void initialize the same permanent status PDA. Replay and re-pin scenarios test this behavior.                                                                         |
| Only authenticated transfer amounts can mint.               | `wire.rs` builds the canonical value and `light_client.rs` checks both membership levels before `mint_transfer.rs` issues tokens. Shared SDK vectors check the encoding.        |
| A governance order cannot itself pay a refund.              | Solana records a void outcome. Akash separately requires finalized evidence, authorization, no hold, an elapsed challenge and available refund capacity.                        |
| A permanent Solana void prevents a later mint.              | Both resolution instructions require the same uninitialized status PDA.                                                                                                         |
| Mint caps are independent of Akash quota epochs.            | `BridgeConfig::next_mint_total` uses checked Solana-clock accounting. Akash uses the configured native `day` epoch. Neither delays immediate submission until a batch boundary. |
| Pause prevents minting, including recipient voids.          | `MintTransfer` requires the enabled flag. Governance void does not require it.                                                                                                  |
| Recipients receive only legacy SPL Token AKT.               | Recipient and ATA checks reject unusable destinations; an invalid recipient becomes a void without issuance.                                                                    |
| Resolution evidence remains attributable.                   | Mint and void require a top-level instruction preceded only by ComputeBudget instructions.                                                                                      |
| Uploads cannot skip, overwrite or exceed their declaration. | Buffer ownership, offset, allocation and length checks run before writing. Buffer tests cover growth and reclamation.                                                           |

## Build and verification

The pinned build uses Solana 2.3.13 with platform-tools 1.48 and SBF Rust 1.84.1. Host tests and IDL generation use Rust 1.89.0 and Anchor 0.32.1. Scripts use Node 24. Commands run from `contracts/solana` after placing the matching SDK archive at `.cache/solana/chain-sdk.tgz` relative to the node root.

```bash
npm ci
npm run setup:light-client
npm run build
npm run test:unit
npm run typecheck
npm run test:services
npm run test:portal-contract
npm test
npm run test:sink
npm run build:verifiable
```

`npm test` starts an isolated validator. `npm run test:sink` uses a real Akash proof. The verifiable build archives a commit and excludes uncommitted changes. Its recorded program hash, IDL, dependency lockfiles and toolchain must match the deployed artifact. A host unit test, a synthetic proof, a validator run and a populated fork exercise establish different facts and must be recorded separately.

Canonical implementation checks include native proof vectors, program unit tests and SBF execution of the built ELF. The measured synthetic 127-step mint used 530,878 compute units. That case is not a universal compute bound or a live-chain result. Fresh validator, signed-header, cross-host, wallet and populated fork acceptance remains required for deployment evidence.

## Limitations and operational risks

- Governance evidence and reporter quorum are Akash trust decisions. Solana cannot prove that Akash has not refunded a transfer. Honest refund processing needs a real finalized Solana void first. A false governance confirmation can violate that assumption.
- Solana mint-cap days reset at UTC midnight. Two adjacent windows can permit twice the daily cap within a rolling 24-hour interval. This clock does not define Akash's configurable quota epochs or its challenge Duration.
- Lowering a cap preserves usage and can stall pending transfers. Lowering the per-transfer cap below a deposit's amount requires a later policy change or governance void.
- Expired, frozen or invalid light clients stop both resolution paths. Akash deposits must be paused operationally when recovery cannot keep pace.
- Status accounts are permanent. Open buffers retain refundable rent until their submitter closes them.
- A System-owned on-curve address proves format, not that anyone controls its private key. The guardian address is also not a proof of key possession.
- Token-2022 accounts are not supported. Metadata and admin authority remain separate from holder balances.
- Old verified heights can authenticate records while their consensus accounts remain available and the client is active. Pruned heights fail; services must obtain a fresh proof without discarding unresolved signed transaction intents.
- The SBF dependency build emits stack diagnostics in transitive curve code. The canonical program's inspected ELF excludes the reported functions. Live client updates, misbehaviour handling and all deployed dependency artifacts still need their own runtime verification.
