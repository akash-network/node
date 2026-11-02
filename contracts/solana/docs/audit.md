# Audit package: AKT Solana program

This package prepares the AKT issuance program for an external audit. It gives
the scope, how to build and test, the trust model, every instruction and
account, the invariants with where each is enforced and tested, the threat
model, and the accepted risks. Paths under `src/` are relative to
`programs/akt-solana`.

## Audit commit

Frozen on 2026-10-19 as `audit-v1`. Running `npm run build:verifiable` on that
snapshot reproduces:

| File                  | SHA-256                                                            |
| --------------------- | ------------------------------------------------------------------ |
| `akt_solana.so`       | `961612a9aa0ee6de4a0ccc208665bc960430fa7c5544766fcf612d7bffd640dc` |
| `ics07_tendermint.so` | `75aad2305e16e8f2d5b42cdcb32f184f4645a93140ac71e6cb8a180d3f188b6b` |
| `access_manager.so`   | `c334e3d6ff81da93e8faba8f23593efe66790f5971e30020e77dacb364d83ed6` |

Later commits change no program input unless this section names a new tag.

## Scope

In scope:

- `programs/akt-solana` at the audit commit, including the
  `ics07-tendermint-interface` crate it depends on. That crate only runs
  Anchor's `declare_program!` over `idls/ics07_tendermint.json`.
- The ICS07 patch series in `patches/`, applied in this order by
  `scripts/apply-light-client-patches.sh` to `cosmos/ibc-contracts` at
  `dc78ade828b766b218fb6bcb5417fdaf8e553021`:
  1. `ics07-signature-batches.patch`
  2. `ics07-monotonic-latest-height.patch`
  3. `ics07-consensus-pruning.patch`
  4. `ics07-open-relaying.patch`

  [Verifier patches](verifier-patches.md) and
  [signature batches](signature-batches.md) give their design and test record.

Out of scope:

- The upstream ICS07 Tendermint client and access manager at the pinned
  commit, apart from the patched lines.
- SPL Token, the Associated Token Account program and Metaplex Token Metadata.
- Anchor 0.32.1, Agave 2.3 and its syscalls.
- The Akash x/sink module, relayer, reporters, monitor and portal. This
  package states only what the program assumes of them.

## Build and test

| Tool             | Version       |
| ---------------- | ------------- |
| Rust             | 1.89.0        |
| Solana / Agave   | 2.3.13        |
| Anchor           | 0.32.1        |
| Node             | 24            |
| Go, for fixtures | 1.23 or later |

```bash
npm ci
npm run setup:light-client # fetch, patch and build the pinned verifier
npm run build              # build AKT and its IDL
npm run test:unit          # cargo test -p akt-solana --lib
npm test                   # fresh local validator and the integration suite
npm run build:verifiable   # pinned container build of a commit
```

`npm run setup:light-client` downloads the pinned upstream source into
`.deps/`, applies the patch series, builds the verifier and access manager and
checks their IDLs against `idls/`. These commands need only this repository;
`npm run setup` also builds the Akash node from this repository.

`npm run build:verifiable` runs `scripts/verifiable-build.sh [--rev <commit>]`.
It archives the commit, so uncommitted files never reach the build, and builds
`akt_solana.so`, `ics07_tendermint.so` and `access_manager.so` at a fixed path
in a pinned linux/amd64 image. It writes `SHA256SUMS` and `build-info.txt`,
and fails if a build changes a committed `Cargo.lock` or if the verifier's
generated IDL differs from `idls/`. To check a deployment, compare
`SHA256SUMS` with `solana program dump` truncated to the file's length. See
[verifiable builds](verifier-patches.md#verifiable-builds).

`npm test` runs `scripts/test-contract.sh`. It builds AKT, starts its own
validator with a fresh wallet and preloads Metaplex, the verifier, the access
manager, two relocated copies of the verifier for re-pin tests (one frozen) and
upstream's `test_cpi_proxy`. The tests then take about 70 seconds, mostly
waiting for a client to expire. Real Akash proofs are covered separately
by `npm run test:sink`.

[Production deployment](production-deployment.md) covers authorities,
parameters, the launch sequence, monitoring and incident runbooks.

## Source map

| Path                                    | Contents                                                                          |
| --------------------------------------- | --------------------------------------------------------------------------------- |
| `src/lib.rs`                            | Entrypoints in lifecycle order, each with caller, effect, event and errors        |
| `src/constants.rs`                      | PDA seeds, token parameters and limits                                            |
| `src/state.rs`                          | Accounts, mint-cap accounting and policy validation                               |
| `src/events.rs`, `src/errors.rs`        | Events and errors                                                                 |
| `src/wire.rs`                           | x/sink record encoding, proof keys and void value                                 |
| `src/light_client.rs`                   | Checks on pinned-client accounts, expiry, and the membership CPI                  |
| `src/instructions/upgrade_authority.rs` | Upgrade-authority check shared by admin instructions                              |
| `src/instructions/genesis.rs`           | `initialize`                                                                      |
| `src/instructions/token_metadata.rs`    | `set_token_metadata`                                                              |
| `src/instructions/bridge_admin.rs`      | `configure_bridge`, `set_bridge_enabled`, `set_mint_policy`, `repin_light_client` |
| `src/instructions/pause_guardian.rs`    | `set_pause_guardian`, `guardian_pause`                                            |
| `src/instructions/proof_buffer.rs`      | `create_proof_buffer`, `append_proof`, `close_proof_buffer`                       |
| `src/instructions/mint_transfer.rs`     | `mint_transfer` and the recipient rules                                           |
| `src/instructions/void_transfer.rs`     | `void_transfer`                                                                   |
| `src/instructions/resolution.rs`        | Invocation rules and terminal status shared by mint and void                      |
| `ics07-tendermint-interface/`           | Generated ICS07 CPI and account types                                             |
| `tests/genesis.test.ts`                 | Genesis; runs `metadata.ts` and `bridge.ts` on the same validator                 |
| `tests/idl.test.ts`                     | Deployed-interface and error-table checks                                         |

## Architecture

```text
Akash x/sink                ICS07 client on Solana         AKT program on Solana
MsgSink writes              relayers submit signed         anyone uploads an ICS23
transfer_key(id)            headers; each stored           proof to a proof buffer
(77-byte record)            consensus state holds a        and calls mint_transfer
                            verified app hash              or void_transfer:
governance writes                                            CPI verify_membership
void_key(id)                                                 then mint to the
                                                             recipient's ATA or void
reporters read TransferResolved and report the outcome back to Akash
```

Genesis creates the AKT mint with zero supply and mints nothing. The only mint
is `mint_transfer`: exactly the amount of an x/sink record whose membership in
Akash state the pinned light client verifies. Each record ID gets one
permanent `TransferStatus`, `Minted` or `Voided`. A record is voided when its
proven recipient cannot receive AKT, or when Akash governance writes a void
order that `void_transfer` proves. Akash refunds a voided deposit after
reporters observe the status; that path is outside this program.

[Bridge protocol](bridge-protocol.md) specifies the x/sink encoding, proof
heights and outcomes. `transfer_key(id)` and `void_key(id)` in `src/wire.rs`
build the 10-byte sink store keys: a two-byte prefix, then the ID as a
big-endian u64.

## Roles and trust

| Role                  | Power in this program                                                                                             | Trust                                                                                        |
| --------------------- | ----------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| AKT upgrade authority | Every admin instruction; replacing the code                                                                       | Full. It can mint without limit by upgrading AKT or by re-pinning a client it controls       |
| Pause guardian        | `guardian_pause` sets `bridge_enabled` to false                                                                   | None beyond pausing. It cannot enable minting or change anything else                        |
| Pinned light client   | Its `verify_membership` decides whether a record or void order is in Akash state at a stored height               | Full for issuance. Its upgrade authority and checkpoint are trust decisions of the same tier |
| Relayers              | Submit headers to the client with `RELAYER_ROLE` or open relaying; submit proofs and pay rent for mints and voids | Liveness only. They cannot forge headers or records                                          |
| Any signer            | Its own proof buffers; `mint_transfer` and `void_transfer` with valid proofs                                      | None. Mint and void are permissionless                                                       |
| Reporters             | None on Solana. They read `TransferResolved` and status accounts and report to Akash                              | Off-chain, under an Akash threshold                                                          |
| Akash governance      | Writes `void_key(id)` orders and x/sink parameters                                                                | Trusted to void only transfers it means to refund                                            |

The program assumes that x/sink writes each `transfer_key(id)` value once and
never changes it, that IDs are never reused on the pinned chain, and that only
governance writes `void_key(id)`. The ICS07 and access-manager upgrade
authorities belong to the same tier as AKT's; production holds all three in
one time-locked multisig
([authorities](production-deployment.md#authorities-and-keys)).

## Instructions

Each entrypoint's doc comment in `src/lib.rs` gives the same information.
Errors list AKT errors; Anchor constraint errors and an `init` on an existing
account ("already in use") also apply.

| Instruction           | Caller                                                             | Writable accounts                                              | Effect                                                                                                                                            | Event                 | Main errors                                                                                                                                              |
| --------------------- | ------------------------------------------------------------------ | -------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `initialize`          | Upgrade authority, once; pays                                      | `authority`, `config`, `mint`                                  | Creates `LaunchConfig` with minting off and the mint (6 decimals, PDA mint authority, no freeze authority) with zero supply; mints nothing        | `LaunchInitialized`   | `InvalidProgramData`, `UnauthorizedInitializer`                                                                                                          |
| `set_token_metadata`  | Upgrade authority; pays on creation                                | `admin.authority`, `metadata`                                  | Creates or rewrites Metaplex metadata: name, symbol, URI; mutable; update authority is the mint-authority PDA                                     | `TokenMetadataSet`    | `InvalidAuthority`, `InvalidMetadataUri`, `InvalidMetadata`                                                                                              |
| `configure_bridge`    | Upgrade authority, once; pays                                      | `admin.authority`, `bridge`                                    | Pins light client and chain ID after checking the client is active; stores the policy and zero usage                                              | `BridgeConfigured`    | `InvalidChainId`, `InvalidPolicy`, `InvalidLightClient`, `WrongChain`, `ClientFrozen`, `ClientExpired`, `InvalidConsensusTimestamp`                      |
| `set_bridge_enabled`  | Upgrade authority                                                  | `admin.authority`, `config`, `bridge`                          | Sets `bridge_enabled`; needs a configured bridge                                                                                                  | `BridgeEnabledSet`    | `InvalidAuthority`                                                                                                                                       |
| `set_mint_policy`     | Upgrade authority                                                  | `admin.authority`, `config`, `bridge`                          | Replaces the policy; the day's usage carries over                                                                                                 | `MintPolicySet`       | `InvalidAuthority`, `InvalidPolicy`                                                                                                                      |
| `set_pause_guardian`  | Upgrade authority; pays on first call                              | `admin.authority`, `pause_guardian`                            | Creates or overwrites the guardian; `None` clears it                                                                                              | `PauseGuardianSet`    | `InvalidAuthority`                                                                                                                                       |
| `guardian_pause`      | Pause guardian                                                     | `config`                                                       | Sets `bridge_enabled` to false                                                                                                                    | `GuardianPaused`      | `UnauthorizedGuardian`                                                                                                                                   |
| `repin_light_client`  | Upgrade authority, any time                                        | `admin.authority`, `bridge`                                    | Replaces the pinned program and chain ID after the checks of `configure_bridge`; never reads the old pin; policy and usage carry over             | `LightClientRepinned` | As `configure_bridge`, without `InvalidPolicy`                                                                                                           |
| `create_proof_buffer` | Anyone; pays and owns the buffer                                   | `submitter`, `proof_buffer`                                    | Creates an empty buffer for 1 to 4,096 bytes                                                                                                      | none                  | `InvalidProofLength`                                                                                                                                     |
| `append_proof`        | The buffer's submitter                                             | `proof_buffer`                                                 | Appends a nonempty chunk at the next offset, within the declared length                                                                           | none                  | `InvalidProofOffset`, `InvalidProofLength`                                                                                                               |
| `close_proof_buffer`  | The buffer's submitter                                             | `submitter`, `proof_buffer`                                    | Closes the buffer and returns its rent                                                                                                            | none                  | Anchor `ConstraintHasOne`                                                                                                                                |
| `mint_transfer`       | Anyone, top-level, after ComputeBudget only; minting enabled; pays | `payer`, `bridge`, `status`, `mint`, `recipient_token_account` | Verifies `transfer_key(id)` = record at `proof_height`; mints to the recipient's ATA (creating it) or voids `InvalidRecipient`; writes the status | `TransferResolved`    | `BridgeDisabled`, `CpiNotAllowed`, `PrecedingInstruction`, `InvalidRecord`, light-client and proof errors, `InvalidClock`, `TransferLimit`, `DailyLimit` |
| `void_transfer`       | Anyone, top-level, after ComputeBudget only; pays                  | `payer`, `status`                                              | Verifies `void_key(id)` = `[1, id]` at `proof_height`; writes `Voided / Governance`; runs while minting is paused                                 | `TransferResolved`    | `CpiNotAllowed`, `PrecedingInstruction`, `InvalidRecord`, light-client and proof errors                                                                  |

Light-client and proof errors are `InvalidLightClient`, `WrongChain`,
`ClientFrozen`, `ClientExpired`, `InvalidConsensusTimestamp`,
`InvalidProofHeight` and `IncompleteProof`, plus ICS07 errors such as
`MembershipVerificationFailed` from the CPI.

AKT's error codes run from 7000 to 7023 so they never share a number with
ICS07's, which start at 6000 and reach clients as the same custom error of the
AKT instruction. The first variant has an explicit discriminant because
Anchor 0.32's `#[error_code(offset)]` leaves the IDL at 6000.

`admin` is the `UpgradeAuthority` account group: this program, its
ProgramData, and the signer recorded there as upgrade authority. Its signer is
writable in every admin instruction because some of them pay rent with it.

A verified mint with a 620-byte proof used 133,000 to 152,000 compute units,
including ATA creation, in the local suite. Relayers request 1,400,000 units
and a 256 KB heap frame for the verifier.

## Accounts and PDAs

Sizes include the 8-byte discriminator.
`state::tests::account_sizes_match_the_deployed_layout` pins them.

| Account                 | Address                                          | Owner      |  Bytes | Created by                         | Changed by                                                                   | Closed by            |
| ----------------------- | ------------------------------------------------ | ---------- | -----: | ---------------------------------- | ---------------------------------------------------------------------------- | -------------------- |
| `LaunchConfig`          | `["config"]`                                     | AKT        |     84 | `initialize`                       | `bridge_enabled`: `set_bridge_enabled`, `guardian_pause`                     | Never                |
| AKT mint                | `["mint"]`                                       | SPL Token  |     82 | `initialize`                       | Supply: `mint_transfer`; holders burn                                        | Never                |
| Mint authority          | `["mint_authority"]`                             | No account |      0 | Never; it only signs               | None                                                                         | None                 |
| `BridgeConfig`          | `["bridge"]`                                     | AKT        |    656 | `configure_bridge`                 | Pin: `repin_light_client`; policy: `set_mint_policy`; usage: `mint_transfer` | Never                |
| `PauseGuardian`         | `["guardian"]`                                   | AKT        |     42 | First `set_pause_guardian`         | `set_pause_guardian`                                                         | Never                |
| `ProofBuffer`           | `["proof", submitter, nonce as u64 LE]`          | AKT        |  4,152 | `create_proof_buffer`              | `append_proof`                                                               | `close_proof_buffer` |
| `TransferStatus`        | `["transfer", record ID as u64 LE]`              | AKT        |     73 | `mint_transfer` or `void_transfer` | Never                                                                        | Never                |
| Recipient token account | ATA of recipient and mint, legacy SPL Token      | SPL Token  |    165 | `mint_transfer`, if absent         | Holder                                                                       | Holder               |
| Token metadata          | Metaplex `["metadata", Metaplex, mint]`          | Metaplex   | Varies | `set_token_metadata`               | `set_token_metadata`                                                         | Never                |
| ICS07 client state      | `["client"]` under the pinned program            | ICS07      |        | ICS07                              | Read only                                                                    |                      |
| ICS07 consensus state   | `["consensus_state", height as u64 LE]` under it | ICS07      |        | ICS07                              | Read only; ICS07 prunes expired ones                                         |                      |

Account names, field order and types, PDA seeds, instruction names, arguments
and account lists are the deployed interface. The zero-supply genesis build
removed the treasury from `initialize`, `LaunchConfig` (now version 2) and the
genesis event, so a network launched with the earlier 1,000,000 AKT genesis
mint is redeployed, not upgraded in place. `tests/idl.test.ts` requires every
instruction, account, event and type of this build's IDL to stay unchanged and
allows only additions.

## Invariants

| #   | Invariant                                                                                                                                                                                                                                                           | Enforced in                                                                                                                                                                                                                            | Tested by                                                                                                                                                                                                                                                                  |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | Genesis runs once, only for the upgrade authority, and mints nothing: supply starts at zero                                                                                                                                                                         | `genesis.rs`: `init` on `config` and `mint`, no `mint_to`; no instruction closes either                                                                                                                                                | `genesis.test.ts`: "creates the mint with zero supply and disables bridge issuance", "rejects repeating genesis", "rejects repeating genesis from a non-upgrade-authority signer"                                                                                          |
| 2   | Only this program can mint, and no one can freeze                                                                                                                                                                                                                   | Mint authority is the `["mint_authority"]` PDA, no freeze authority; no instruction changes token authorities                                                                                                                          | `genesis.test.ts`: genesis assertions, "rejects direct minting by another signer", "rejects taking over the mint authority", "rejects a substituted mint authority"                                                                                                        |
| 3   | Supply equals the amounts of all `Minted` statuses, minus burns                                                                                                                                                                                                     | The only `mint_to` call is in `mint_transfer::issue`; `issue` mints `record.amount` and the same instruction stores `Minted { amount: record.amount }`                                                                                 | Supply checks after every step in `bridge.ts`                                                                                                                                                                                                                              |
| 4   | Each record ID resolves at most once, across pauses and re-pins                                                                                                                                                                                                     | Mint and void `init` `["transfer", id]`; nothing closes it; the seed holds the ID alone                                                                                                                                                | `bridge.ts`: "rejects replay and a governance void after minting", "honors a proven governance void while minting is paused", "re-pins away from an expired, closed deployment without replay"                                                                             |
| 5   | AKT is issued only for a record proven in Akash state at a verified height of an active, unfrozen, pinned client                                                                                                                                                    | `mint_transfer` runs `TransferRecord::validate` and `light_client::verify_membership` (pinned executable program, owned PDAs, `check_active`, height at most the latest, complete buffer, program-built path and value) before `issue` | `bridge.ts`: "rejects modified amounts, recipients, IDs and proof bytes in the real verifier", "rejects incomplete proofs and untrusted client accounts", "rejects an expired client even though its membership proof remains valid"; `light_client` and `wire` unit tests |
| 6   | Each mint is at most `per_transfer_max`, and each UTC day's mints at most `daily_mint_cap`; a cap failure leaves the record pending                                                                                                                                 | `BridgeConfig::next_mint_total` in `issue`, with a checked add and a clock-regression check; `MintPolicy::validate`                                                                                                                    | `state` unit tests; `bridge.ts`: "retries transfer limits and daily caps without voiding or resetting usage"                                                                                                                                                               |
| 7   | While `bridge_enabled` is false no `mint_transfer` succeeds, including recipient voids; `void_transfer` still does. Only the upgrade authority enables; the guardian can only disable                                                                               | `MintTransfer::config` constraint; `VoidTransfer` does not read `config`; `ManageBridge` requires `UpgradeAuthority`; `guardian_pause` writes only `false`                                                                             | `bridge.ts`: "rejects disabled bridge issuance and unauthorized activation", "lets the pause guardian disable but never enable minting", "honors a proven governance void while minting is paused"                                                                         |
| 8   | `Voided / Governance` requires a proven void order `void_key(id)` = `[1, id as u64 LE]`                                                                                                                                                                             | `void_transfer` with `wire::void_key` and `wire::void_value`                                                                                                                                                                           | `bridge.ts` governance void; `wire::tests::proof_keys_and_void_value_use_the_documented_format`                                                                                                                                                                            |
| 9   | Tokens go only to the recipient's legacy ATA for the AKT mint. The recipient is a System-owned on-curve key or an allowlisted vault, and never an executable, the zero key, the mint authority or a token account. Otherwise the record voids and no ATA is created | `MintTransfer` address constraints; `accepts_recipient`; `ata_usable`                                                                                                                                                                  | `bridge.ts`: "voids an invalid recipient without creating its ATA or minting" (token account, executable, zero key), "voids a recipient whose ATA owner was reassigned", "allows registered vault PDAs and voids unregistered PDAs"; `ata_usable` unit test                |
| 10  | Mint and void run only as top-level instructions preceded by ComputeBudget instructions alone, so `TransferResolved` is decodable and not truncated                                                                                                                 | `resolution::require_top_level_invocation`                                                                                                                                                                                             | `bridge.ts`: "rejects mint and void called through another program", "rejects mint and void after an instruction other than compute budget"; `resolution` unit tests                                                                                                       |
| 11  | Only the upgrade authority configures; the pin changes only through `configure_bridge` and `repin_light_client`, each requiring an active client for the given chain                                                                                                | `UpgradeAuthority`; `check_chain_id`; `light_client::check_active`; `BridgeConfig` `init`                                                                                                                                              | `bridge.ts`: "rejects unauthorized configuration and a wrong chain", "pins the real client once and keeps minting disabled", "re-pins an active client only to a checked deployment"; `metadata.ts`                                                                        |
| 12  | Only a buffer's submitter writes or closes it; writes are sequential and within the declared length                                                                                                                                                                 | `has_one = submitter`; `append_proof` offset and length checks                                                                                                                                                                         | `bridge.ts`: "protects proof uploads from other writers, gaps and oversize chunks", "returns upload rent only to its owner and retains permanent transfer status"                                                                                                          |
| 13  | The record encoding and proof keys match x/sink                                                                                                                                                                                                                     | `wire.rs`                                                                                                                                                                                                                              | `wire` unit tests against a hand-written 77-byte vector; `npm run test:sink` against the real Akash app                                                                                                                                                                    |

## Threat model

| Threat                                                                      | Mitigation                                                                                                                                                                                                              |
| --------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Forged or altered record                                                    | The program builds the path `["sink", transfer_key(id)]` and the borsh value from the instruction's record. Any changed field changes the proven value and fails ICS23 verification                                     |
| Substituted light client or client accounts                                 | The program account must equal the pin and be executable. Client and consensus accounts must be owned by it, sit at its PDAs, carry ICS07 discriminators and, for consensus states, the expected height                 |
| Proof against an unverified height                                          | `proof_height` must be at most the client's latest height and have a consensus state under the pinned program; ICS07 reads the root from that account                                                                   |
| Stale or compromised client                                                 | Every resolution rejects a frozen client, one whose latest consensus state is outside the trusting period, and a consensus timestamp ahead of the clock plus drift. The guardian can pause and the authority can re-pin |
| Double issuance, or mint after void                                         | One `init`-only status PDA per record ID for both instructions; re-pinning cannot reset it                                                                                                                              |
| Resolution hidden from reporters                                            | CPI is rejected by stack height; only ComputeBudget instructions may precede, so earlier logs cannot push `TransferResolved` past the 10 KB limit                                                                       |
| Recipient that cannot hold AKT, or a recipient that blocks its own transfer | Unusable recipients void instead of minting. An ATA whose owner was reassigned would make `create_idempotent` fail forever, so it voids too                                                                             |
| Excess issuance from a verifier bug or bad checkpoint                       | Per-transfer and daily caps bound the loss; the guardian pauses without the time lock                                                                                                                                   |
| Unrecoverable client (expired, frozen, closed)                              | `repin_light_client` moves the pin without reading the old deployment                                                                                                                                                   |
| Proof upload interference                                                   | Buffers are PDAs of the submitter's key; only the submitter appends or closes. Using another submitter's complete buffer is allowed: the proof is still verified against the record                                     |
| Mint takeover                                                               | The PDA mint authority signs only in `mint_transfer` and `set_token_metadata`; no instruction changes token authorities                                                                                                 |
| Metadata hijack                                                             | Metaplex lets only the mint authority create metadata; updates check the account's owner, mint and update authority                                                                                                     |
| Admin key compromise                                                        | Not mitigated on-chain. Production uses a time-locked multisig and an out-of-band guardian ([production deployment](production-deployment.md))                                                                          |

## Accepted risks and known limitations

- **Upgrade authorities.** Whoever holds the AKT, ICS07 or access-manager
  upgrade authority can mint without limit. The test network uses one hot key
  for all of them.
- **Re-pinning trusts a new checkpoint.** Statuses are keyed by record ID
  alone. Re-pinning to a reset or forked chain that reuses IDs could mint IDs
  not yet resolved on Solana for different records. Never re-pin to such a
  chain.
- **Permissionless minting races Akash refunds.** Anyone with a valid proof
  can mint a record that Akash voided or refunded before a Solana status
  existed. The honest relayer checks Akash first and the reconciler alerts
  ([blocker 6](production-deployment.md#settlement-safety)).
- **Wallet check is a format check.** An on-curve System-owned key passes
  whether or not anyone holds it.
- **Fixed UTC days.** The daily cap resets at 00:00 UTC on the Solana clock,
  so up to twice the cap can mint within 24 hours around midnight. A cap
  lowered below the day's usage blocks mints until the next day.
- **Lowering `per_transfer_max`** below a pending record's amount stalls it
  until the policy changes or governance voids it.
- **Pause also blocks recipient voids,** which go through `mint_transfer`.
- **Expiry stops resolutions.** Mints and voids fail until a re-pin, while
  Akash keeps accepting deposits unless its guardian pauses them.
- **Rent.** Status accounts are permanent and their rent is never returned.
  Open proof buffers keep their rent until the submitter closes them.
- **Guardian key is not validated.** `set_pause_guardian(Some(key))` accepts
  a key no one can sign for, which acts like `None`.
- **Writable accounts in shared structs.** `ManageBridge` marks `config`
  writable in `set_mint_policy` and `bridge` writable in `set_bridge_enabled`,
  and `UpgradeAuthority` marks its signer writable in instructions that pay no
  rent. Changing them would change the deployed interface; a writable flag
  grants nothing by itself.
- **`init_if_needed`** creates the guardian account on the first
  `set_pause_guardian`. Only the upgrade authority reaches it, the handler
  rewrites every field and nothing closes the account.
- **Admin events inside a CPI.** A multisig upgrade authority calls admin
  instructions through CPI. Anchor's TypeScript `EventParser` ignores events
  logged inside a CPI, so monitors must attribute `Program data` lines by
  invocation frame, as the reporters do for resolutions.
- **Token program.** Only legacy SPL Token accounts receive AKT; Token-2022
  ATAs are not supported.
- **The record's `sender` and `height`** are not interpreted on Solana beyond
  `0 < height <= proof_height`; they matter only as part of the proven value.
- **Old verified heights.** Any stored consensus state up to the latest height
  can prove a record while the client is active and the state is not pruned.
- **Upstream ICS07.** The build emits stack-size diagnostics in curve and
  misbehaviour conversion code. Membership verification, updates with up to
  100 validators and misbehaviour freezing pass runtime tests; any reachable
  stack violation still needs resolution.
- **Not covered by integration tests:** a Token-2022-owned recipient, the
  `InvalidMetadata` update path, a negative or regressing Solana clock and day
  rollover on a live clock. Unit tests cover the underlying rules where they
  are pure functions.
